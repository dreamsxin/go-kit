// Package validation turns `validate` struct tags into runtime checks.
//
// The tag subset is deliberately small: every rule maps one-to-one onto a
// JSON Schema keyword, so one tag drives the generated contract, the form a
// frontend builds from it, and the check the service runs — a single source
// of truth instead of three diverging ones:
//
//	Status string `json:"status" validate:"enum=active|suspended,default=active"`
//	Age    int    `json:"age" validate:"min=0,max=150"`
//	Name   string `json:"name" validate:"minLen=1,maxLen=64"`
//
// enum lists the allowed values, separated by |. min and max bound numeric
// fields. minLen and maxLen bound the length of strings, slices, and maps.
// default is a form-filling hint carried by the contract; validation does not
// enforce it, because a default is what a form pre-fills, not what a server
// assumes. A pointer field that is nil is absent — constraints do not run on
// it, because required-ness is the field's own business: a non-pointer field
// is already required in the generated contract.
//
// Failures are classified with apperror as invalid_argument, with a
// machine-readable code naming the field and the rule
// ("validation.username.minLen"), so every transport maps them uniformly and
// a form can point at the field that failed.
package validation

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// Stable: validation.tag-semantics — the validate tag carries the rules
// enum, min, max, minLen, maxLen and default; a value violating enum, min,
// max, minLen or maxLen fails validation as an invalid_argument error whose
// code names the field and the rule; an absent pointer field and a struct
// with no validate tags pass; default is never enforced.
// Covered by: TestParseTag, TestValidateEnum, TestValidateRange,
// TestValidateLength, TestValidateSkipsAbsentPointer, TestValidateIgnoresDefault

// Constraints is one field's parsed validate tag.
type Constraints struct {
	Enum       []string
	Min        *float64
	Max        *float64
	MinLen     *int
	MaxLen     *int
	Default    string
	HasDefault bool
}

// Empty reports whether the constraints carry no rule at all.
func (c Constraints) Empty() bool {
	return c.Enum == nil && c.Min == nil && c.Max == nil && c.MinLen == nil && c.MaxLen == nil
}

// ParseTag parses one validate tag. An unknown rule, a missing value, a
// malformed number, or min > max / minLen > maxLen is an error, so a broken
// tag is refused at generation time instead of being silently ignored at
// request time.
func ParseTag(tag string) (Constraints, error) {
	var c Constraints
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return c, nil
	}
	for _, option := range strings.Split(tag, ",") {
		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}
		key, value, ok := strings.Cut(option, "=")
		if !ok {
			return c, fmt.Errorf("validate tag %q: rule %q has no value", tag, option)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "enum":
			if value == "" {
				return c, fmt.Errorf("validate tag %q: enum lists no values", tag)
			}
			c.Enum = strings.Split(value, "|")
		case "min":
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return c, fmt.Errorf("validate tag %q: min %q is not a number", tag, value)
			}
			c.Min = &f
		case "max":
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return c, fmt.Errorf("validate tag %q: max %q is not a number", tag, value)
			}
			c.Max = &f
		case "minLen":
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return c, fmt.Errorf("validate tag %q: minLen %q is not a non-negative integer", tag, value)
			}
			c.MinLen = &n
		case "maxLen":
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return c, fmt.Errorf("validate tag %q: maxLen %q is not a non-negative integer", tag, value)
			}
			c.MaxLen = &n
		case "default":
			c.Default = value
			c.HasDefault = true
		default:
			return c, fmt.Errorf("validate tag %q: unknown rule %q (want enum, min, max, minLen, maxLen, or default)", tag, key)
		}
	}
	if c.Min != nil && c.Max != nil && *c.Min > *c.Max {
		return c, fmt.Errorf("validate tag %q: min %v is greater than max %v", tag, *c.Min, *c.Max)
	}
	if c.MinLen != nil && c.MaxLen != nil && *c.MinLen > *c.MaxLen {
		return c, fmt.Errorf("validate tag %q: minLen %d is greater than maxLen %d", tag, *c.MinLen, *c.MaxLen)
	}
	return c, nil
}

// Validate checks value against its validate tags and returns the first
// violation, classified as an invalid_argument apperror. A value that is not
// a struct passes: rules live on struct fields, and a decoded map or slice
// has none. The rules per type are discovered once and cached.
func Validate(value any) error {
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	for _, rule := range rulesFor(rv.Type()) {
		if err := rule.check(rv); err != nil {
			return err
		}
	}
	return nil
}

// Middleware returns endpoint middleware that validates each request against
// its validate tags. Wire it through kit.WithEndpointMiddleware or a
// per-route builder for a service that assembles its requests outside the
// typed JSON family; the typed routes and the generated transports validate
// on their own.
func Middleware() endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if err := Validate(request); err != nil {
				return nil, err
			}
			return next(ctx, request)
		}
	}
}

// fieldRule is one validated field of one type.
type fieldRule struct {
	index int
	name  string
	cons  Constraints
}

var ruleCache sync.Map // reflect.Type -> []fieldRule

func rulesFor(t reflect.Type) []fieldRule {
	if cached, ok := ruleCache.Load(t); ok {
		return cached.([]fieldRule)
	}
	var rules []fieldRule
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("validate")
		if tag == "" {
			continue
		}
		cons, err := ParseTag(tag)
		// A broken tag is a programming error, the same class as a
		// conflicting ServeMux pattern: it fails loudly at first use, where
		// any test that touches the route meets it.
		if err != nil {
			panic(fmt.Sprintf("validation: %s.%s: %v", t, f.Name, err))
		}
		rules = append(rules, fieldRule{index: i, name: fieldName(f), cons: cons})
	}
	actual, _ := ruleCache.LoadOrStore(t, rules)
	return actual.([]fieldRule)
}

func fieldName(f reflect.StructField) string {
	if json := f.Tag.Get("json"); json != "" && json != "-" {
		if name, _, ok := strings.Cut(json, ","); ok || name != "" {
			return name
		}
	}
	return f.Name
}

func (r fieldRule) check(rv reflect.Value) error {
	fv := rv.Field(r.index)
	if fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			return nil
		}
		fv = fv.Elem()
	}
	if len(r.cons.Enum) > 0 && !r.inEnum(fv) {
		return r.fail("enum", fmt.Sprintf("%s must be one of: %s", r.name, strings.Join(r.cons.Enum, ", ")))
	}
	if number, ok := asNumber(fv); ok {
		if r.cons.Min != nil && number < *r.cons.Min {
			return r.fail("min", fmt.Sprintf("%s must be at least %v", r.name, trimFloat(*r.cons.Min)))
		}
		if r.cons.Max != nil && number > *r.cons.Max {
			return r.fail("max", fmt.Sprintf("%s must be at most %v", r.name, trimFloat(*r.cons.Max)))
		}
	}
	if r.cons.MinLen != nil && hasLength(fv) && fv.Len() < *r.cons.MinLen {
		return r.fail("minLen", fmt.Sprintf("%s must be at least %d character(s)", r.name, *r.cons.MinLen))
	}
	if r.cons.MaxLen != nil && hasLength(fv) && fv.Len() > *r.cons.MaxLen {
		return r.fail("maxLen", fmt.Sprintf("%s must be at most %d character(s)", r.name, *r.cons.MaxLen))
	}
	return nil
}

func (r fieldRule) inEnum(fv reflect.Value) bool {
	if number, ok := asNumber(fv); ok {
		for _, entry := range r.cons.Enum {
			if parsed, err := strconv.ParseFloat(entry, 64); err == nil && parsed == number {
				return true
			}
		}
		return false
	}
	text := fmt.Sprint(fv.Interface())
	for _, entry := range r.cons.Enum {
		if entry == text {
			return true
		}
	}
	return false
}

// asNumber reports the numeric value of an int, uint, or float field.
func asNumber(v reflect.Value) (float64, bool) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	}
	return 0, false
}

func (r fieldRule) fail(rule, message string) error {
	return apperror.New(apperror.KindInvalidArgument, "validation."+r.name+"."+rule, message)
}

func hasLength(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return true
	}
	return false
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
