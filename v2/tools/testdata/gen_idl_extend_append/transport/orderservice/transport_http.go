package orderservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
	idl "example.com/gen_idl_extend_append"
	genendpoint "example.com/gen_idl_extend_append/endpoint/orderservice"
)

// NewHTTPHandler returns the generated HTTP handler set.
func NewHTTPHandler(endpoints genendpoint.OrderServiceEndpoints) http.Handler {
	m := http.NewServeMux()
	registerHTTPServeMuxRoutes(m, endpoints)
	return m
}

// RegisterHTTPRoutes binds the generated HTTP routes onto a gorilla/mux router.
func RegisterHTTPRoutes(router *mux.Router, endpoints genendpoint.OrderServiceEndpoints, prefix string) {

	// POST /placeorder
	router.Handle(routePath(prefix, "/placeorder"), server.NewServer(
		endpoints.PlaceOrderEndpoint,
		decodePlaceOrderRequest,
		encodePlaceOrderResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("POST")

}

func registerHTTPServeMuxRoutes(m *http.ServeMux, endpoints genendpoint.OrderServiceEndpoints) {

	m.Handle("POST /placeorder", server.NewServer(
		endpoints.PlaceOrderEndpoint,
		decodePlaceOrderRequest,
		encodePlaceOrderResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func routePath(prefix, route string) string {
	if prefix == "" {
		return route
	}
	return prefix + route
}


var (
	queryTimeType     = reflect.TypeOf(time.Time{})
	queryDurationType = reflect.TypeOf(time.Duration(0))
)

type queryDecodeError struct {
	err error
}

func (e queryDecodeError) Error() string {
	if e.err == nil {
		return "invalid query request"
	}
	return e.err.Error()
}

func (e queryDecodeError) Unwrap() error {
	return e.err
}

func (e queryDecodeError) StatusCode() int {
	return http.StatusBadRequest
}

func (e queryDecodeError) ErrorCode() string {
	return "bad_request.invalid_query"
}

func decodeQueryRequest(r *http.Request, target any) error {
	if r == nil {
		return fmt.Errorf("nil HTTP request")
	}
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return fmt.Errorf("query target must be a non-nil pointer")
	}
	elem := value.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("query target must point to a struct")
	}
	return decodeQueryStruct(r, elem, r.URL.Query(), mux.Vars(r))
}

func decodeQueryStruct(r *http.Request, target reflect.Value, values url.Values, pathVars map[string]string) error {
	targetType := target.Type()
	for i := 0; i < target.NumField(); i++ {
		fieldInfo := targetType.Field(i)
		field := target.Field(i)
		if fieldInfo.PkgPath != "" || !field.CanSet() {
			continue
		}
		if fieldInfo.Anonymous && indirectKind(field) == reflect.Struct {
			if err := decodeQueryStruct(r, indirectValue(field), values, pathVars); err != nil {
				return err
			}
			continue
		}
		names := queryFieldNames(fieldInfo)
		if len(names) == 0 {
			continue
		}
		raw, ok := queryFieldValue(r, values, pathVars, names)
		if !ok {
			continue
		}
		if err := setQueryField(field, raw); err != nil {
			return fmt.Errorf("%s: %w", fieldInfo.Name, err)
		}
	}
	return nil
}

func queryFieldNames(field reflect.StructField) []string {
	var names []string
	for _, tagName := range []string{"form", "json"} {
		tag := field.Tag.Get(tagName)
		if tag == "-" {
			return nil
		}
		if name := strings.TrimSpace(strings.Split(tag, ",")[0]); name != "" {
			names = append(names, name)
		}
	}
	names = append(names, field.Name, strings.ToLower(field.Name))
	return names
}

func queryFieldValue(r *http.Request, values url.Values, pathVars map[string]string, names []string) (string, bool) {
	for _, name := range names {
		if vals, ok := values[name]; ok && len(vals) > 0 {
			return vals[len(vals)-1], true
		}
	}
	for _, name := range names {
		if value, ok := pathVars[name]; ok {
			return value, true
		}
	}
	for _, name := range names {
		if value := r.PathValue(name); value != "" {
			return value, true
		}
	}
	return "", false
}

func setQueryField(field reflect.Value, raw string) error {
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setQueryField(field.Elem(), raw)
	}
	if field.Type() == queryTimeType {
		parsed, err := parseQueryTime(raw)
		if err != nil {
			return err
		}
		field.Set(reflect.ValueOf(parsed))
		return nil
	}
	if field.Type() == queryDurationType {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		field.SetInt(int64(parsed))
		return nil
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
		return fmt.Errorf("unsupported query field type %s", field.Type())
	}
	return nil
}

func parseQueryTime(raw string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02", raw)
}

func indirectKind(v reflect.Value) reflect.Kind {
	return indirectValue(v).Kind()
}

func indirectValue(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return v.Elem()
	}
	return v
}



// decodePlaceOrderRequest uses the generated method-aware decode path.
//
// @Summary      PlaceOrder
// @Description  PlaceOrder microservice endpoint
// @Tags         OrderService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.PlaceOrderRequest  true  "PlaceOrder request"
// @Success      200      {object}  idl.PlaceOrderResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /placeorder [post]
func decodePlaceOrderRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.PlaceOrderRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodePlaceOrderResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}
