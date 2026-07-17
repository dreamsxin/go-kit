package http

import (
	"encoding"
	"fmt"
	nethttp "net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	queryTimeType       = reflect.TypeOf(time.Time{})
	queryDurationType   = reflect.TypeOf(time.Duration(0))
	textMarshalerType   = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// QueryError reports an invalid query or path parameter.
type QueryError struct {
	Field string
	Err   error
}

func (e *QueryError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Field == "" {
		return fmt.Sprintf("invalid query request: %v", e.Err)
	}
	return fmt.Sprintf("invalid query field %s: %v", e.Field, e.Err)
}

func (e *QueryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *QueryError) StatusCode() int   { return nethttp.StatusBadRequest }
func (e *QueryError) ErrorCode() string { return "bad_request.invalid_query" }

// EncodePathAndQuery replaces path placeholders and encodes remaining request
// fields as query parameters. Struct fields use form tags first, then json tags,
// then their lower-cased Go name.
func EncodePathAndQuery(path string, request any) (string, error) {
	if request == nil {
		return path, nil
	}
	u, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("parse request path: %w", err)
	}
	value, ok := indirectQueryValue(reflect.ValueOf(request))
	if !ok {
		return path, nil
	}
	if value.Kind() != reflect.Struct {
		return "", fmt.Errorf("encode query request: expected struct, got %s", value.Type())
	}

	rawPath := u.Path
	query := u.Query()
	if err := encodeQueryStruct(value, &rawPath, query); err != nil {
		return "", err
	}
	if strings.ContainsAny(rawPath, "{}") {
		return "", fmt.Errorf("encode query request: unresolved path parameter in %q", rawPath)
	}
	escapedPath := escapedQueryPath(rawPath)
	decodedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return "", fmt.Errorf("decode encoded request path: %w", err)
	}
	u.Path = decodedPath
	u.RawPath = escapedPath
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// EncodePath replaces path placeholders from request fields without adding
// the remaining fields to the query string.
func EncodePath(path string, request any) (string, error) {
	if request == nil {
		return path, nil
	}
	u, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("parse request path: %w", err)
	}
	value, ok := indirectQueryValue(reflect.ValueOf(request))
	if !ok {
		return path, nil
	}
	if value.Kind() != reflect.Struct {
		return "", fmt.Errorf("encode request path: expected struct, got %s", value.Type())
	}

	rawPath := u.Path
	if err := encodePathStruct(value, &rawPath); err != nil {
		return "", err
	}
	if strings.ContainsAny(rawPath, "{}") {
		return "", fmt.Errorf("encode request path: unresolved path parameter in %q", rawPath)
	}
	escapedPath := escapedQueryPath(rawPath)
	decodedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return "", fmt.Errorf("decode encoded request path: %w", err)
	}
	u.Path = decodedPath
	u.RawPath = escapedPath
	return u.String(), nil
}

// DecodeQueryRequest decodes path and query parameters into a struct pointer.
// Path parameters take precedence over query parameters with the same name.
func DecodeQueryRequest(r *nethttp.Request, target any) error {
	if r == nil {
		return &QueryError{Err: fmt.Errorf("nil HTTP request")}
	}
	return decodeRequestParameters(r, target, r.URL.Query())
}

// DecodePathRequest decodes path parameters into a struct pointer. It is
// intended to run after body decoding so URL values take precedence.
func DecodePathRequest(r *nethttp.Request, target any) error {
	return decodeRequestParameters(r, target, nil)
}

func decodeRequestParameters(r *nethttp.Request, target any, query url.Values) error {
	if r == nil {
		return &QueryError{Err: fmt.Errorf("nil HTTP request")}
	}
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return &QueryError{Err: fmt.Errorf("target must be a non-nil pointer")}
	}
	elem := value.Elem()
	if elem.Kind() != reflect.Struct {
		return &QueryError{Err: fmt.Errorf("target must point to a struct")}
	}
	return decodeQueryStruct(r, elem, query)
}

func encodeQueryStruct(value reflect.Value, path *string, query url.Values) error {
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		fieldInfo := valueType.Field(i)
		field := value.Field(i)
		if fieldInfo.PkgPath != "" {
			continue
		}
		spec := queryFieldSpecFor(fieldInfo)
		if spec.skip {
			continue
		}
		if fieldInfo.Anonymous {
			if embedded, ok := indirectQueryValue(field); ok && embedded.Kind() == reflect.Struct {
				if err := encodeQueryStruct(embedded, path, query); err != nil {
					return err
				}
				continue
			}
		}
		if spec.omitEmpty && field.IsZero() {
			continue
		}
		values, err := encodeQueryValues(field)
		if err != nil {
			return fmt.Errorf("encode query field %s: %w", fieldInfo.Name, err)
		}
		if len(values) == 0 {
			continue
		}
		if replacePathValue(path, spec.names, values) {
			continue
		}
		query.Del(spec.name)
		for _, item := range values {
			query.Add(spec.name, item)
		}
	}
	return nil
}

func encodePathStruct(value reflect.Value, path *string) error {
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		fieldInfo := valueType.Field(i)
		field := value.Field(i)
		if fieldInfo.PkgPath != "" {
			continue
		}
		spec := queryFieldSpecFor(fieldInfo)
		if spec.skip {
			continue
		}
		if fieldInfo.Anonymous {
			if embedded, ok := indirectQueryValue(field); ok && embedded.Kind() == reflect.Struct {
				if err := encodePathStruct(embedded, path); err != nil {
					return err
				}
				continue
			}
		}
		if !pathContainsQueryName(*path, spec.names) {
			continue
		}
		values, err := encodeQueryValues(field)
		if err != nil {
			return fmt.Errorf("encode path field %s: %w", fieldInfo.Name, err)
		}
		if len(values) == 0 || values[len(values)-1] == "" {
			return fmt.Errorf("encode request path: empty path parameter %s", spec.name)
		}
		replacePathValue(path, spec.names, values)
	}
	return nil
}

func pathContainsQueryName(path string, names []string) bool {
	for _, name := range names {
		if strings.Contains(path, "{"+name+"}") {
			return true
		}
	}
	return false
}

func decodeQueryStruct(r *nethttp.Request, target reflect.Value, query url.Values) error {
	targetType := target.Type()
	for i := 0; i < target.NumField(); i++ {
		fieldInfo := targetType.Field(i)
		field := target.Field(i)
		if fieldInfo.PkgPath != "" || !field.CanSet() {
			continue
		}
		spec := queryFieldSpecFor(fieldInfo)
		if spec.skip {
			continue
		}
		if fieldInfo.Anonymous && queryStructField(field) {
			embedded := ensureQueryValue(field)
			if err := decodeQueryStruct(r, embedded, query); err != nil {
				return err
			}
			continue
		}

		values, ok := queryRequestValues(r, query, spec.names)
		if !ok {
			continue
		}
		if err := setQueryValues(field, values); err != nil {
			return &QueryError{Field: fieldInfo.Name, Err: err}
		}
	}
	return nil
}

type queryFieldSpec struct {
	name      string
	names     []string
	omitEmpty bool
	skip      bool
}

func queryFieldSpecFor(field reflect.StructField) queryFieldSpec {
	spec := queryFieldSpec{}
	seen := map[string]struct{}{}
	add := func(name string) {
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		spec.names = append(spec.names, name)
		if spec.name == "" {
			spec.name = name
		}
	}

	for _, tagName := range []string{"form", "json"} {
		tag := field.Tag.Get(tagName)
		parts := strings.Split(tag, ",")
		if len(parts) > 0 && parts[0] == "-" {
			spec.skip = true
			return spec
		}
		if len(parts) > 0 {
			add(strings.TrimSpace(parts[0]))
		}
		for _, option := range parts[1:] {
			if strings.TrimSpace(option) == "omitempty" {
				spec.omitEmpty = true
			}
		}
	}
	add(strings.ToLower(field.Name))
	add(field.Name)
	return spec
}

func encodeQueryValues(value reflect.Value) ([]string, error) {
	value, ok := indirectQueryValue(value)
	if !ok {
		return nil, nil
	}
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		values := make([]string, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			encoded, present, err := encodeQueryScalar(value.Index(i))
			if err != nil {
				return nil, err
			}
			if present {
				values = append(values, encoded)
			}
		}
		return values, nil
	}
	encoded, present, err := encodeQueryScalar(value)
	if err != nil || !present {
		return nil, err
	}
	return []string{encoded}, nil
}

func encodeQueryScalar(value reflect.Value) (string, bool, error) {
	value, ok := indirectQueryValue(value)
	if !ok {
		return "", false, nil
	}
	if value.Type() == queryDurationType {
		return time.Duration(value.Int()).String(), true, nil
	}
	if value.Type() == queryTimeType {
		return value.Interface().(time.Time).Format(time.RFC3339Nano), true, nil
	}
	if value.CanInterface() && value.Type().Implements(textMarshalerType) {
		raw, err := value.Interface().(encoding.TextMarshaler).MarshalText()
		return string(raw), true, err
	}
	if value.CanAddr() && value.Addr().CanInterface() && value.Addr().Type().Implements(textMarshalerType) {
		raw, err := value.Addr().Interface().(encoding.TextMarshaler).MarshalText()
		return string(raw), true, err
	}

	switch value.Kind() {
	case reflect.String:
		return value.String(), true, nil
	case reflect.Bool:
		return strconv.FormatBool(value.Bool()), true, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10), true, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(value.Uint(), 10), true, nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(value.Float(), 'g', -1, value.Type().Bits()), true, nil
	default:
		return "", false, fmt.Errorf("unsupported type %s", value.Type())
	}
}

func setQueryValues(field reflect.Value, values []string) error {
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setQueryValues(field.Elem(), values)
	}
	if field.Kind() == reflect.Slice {
		decoded := reflect.MakeSlice(field.Type(), 0, len(values))
		for _, raw := range values {
			item := reflect.New(field.Type().Elem()).Elem()
			if err := setQueryScalar(item, raw); err != nil {
				return err
			}
			decoded = reflect.Append(decoded, item)
		}
		field.Set(decoded)
		return nil
	}
	if len(values) == 0 {
		return nil
	}
	return setQueryScalar(field, values[len(values)-1])
}

func setQueryScalar(field reflect.Value, raw string) error {
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setQueryScalar(field.Elem(), raw)
	}
	if field.Type() == queryDurationType {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		field.SetInt(int64(parsed))
		return nil
	}
	if field.Type() == queryTimeType {
		parsed, err := parseQueryTime(raw)
		if err != nil {
			return err
		}
		field.Set(reflect.ValueOf(parsed))
		return nil
	}
	if field.CanAddr() && field.Addr().CanInterface() && field.Addr().Type().Implements(textUnmarshalerType) {
		return field.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(raw))
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		field.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		parsed, err := strconv.ParseUint(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(raw, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(parsed)
	default:
		return fmt.Errorf("unsupported type %s", field.Type())
	}
	return nil
}

func queryRequestValues(r *nethttp.Request, query url.Values, names []string) ([]string, bool) {
	for _, name := range names {
		if value := r.PathValue(name); value != "" {
			return []string{value}, true
		}
	}
	for _, name := range names {
		if values, exists := query[name]; exists && len(values) > 0 {
			return values, true
		}
	}
	return nil, false
}

func replacePathValue(path *string, names, values []string) bool {
	for _, name := range names {
		token := "{" + name + "}"
		if !strings.Contains(*path, token) {
			continue
		}
		*path = strings.ReplaceAll(*path, token, url.PathEscape(values[len(values)-1]))
		return true
	}
	return false
}

func escapedQueryPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
		parts[i] = strings.ReplaceAll(parts[i], "%25", "%")
	}
	return strings.Join(parts, "/")
}

func parseQueryTime(raw string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02", raw)
}

func queryStructField(value reflect.Value) bool {
	valueType := value.Type()
	for valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	return valueType.Kind() == reflect.Struct
}

func ensureQueryValue(value reflect.Value) reflect.Value {
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		value = value.Elem()
	}
	return value
}

func indirectQueryValue(value reflect.Value) (reflect.Value, bool) {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}
	return value, value.IsValid()
}
