package http

import (
	"fmt"
	"reflect"
)

// ValidateQueryStruct checks at startup that T is usable as a query/path
// request struct for DecodeQueryRequest and DecodePathRequest. It reports
// unsupported field types instead of letting the first request discover them
// at runtime.
//
// Call it during assembly, for example in main or in an init of the package
// that defines the request types:
//
//	if err := transporthttp.ValidateQueryStruct[ListOrdersRequest](); err != nil {
//	    return err
//	}
//
// Supported fields: strings, booleans, integers, unsigned integers, floats,
// time.Time, time.Duration, slices of those, pointers, embedded structs, and
// types implementing encoding.TextMarshaler/TextUnmarshaler. Fields with
// `form:"-"` or `json:"-"` are skipped.
func ValidateQueryStruct[T any]() error {
	typ := reflect.TypeOf((*T)(nil)).Elem()
	if typ.Kind() != reflect.Struct {
		return fmt.Errorf("query struct validation: %s is not a struct", typ)
	}
	return validateQueryStructType(typ, typ.Name())
}

func validateQueryStructType(typ reflect.Type, path string) error {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}
		spec := queryFieldSpecFor(field)
		if spec.skip {
			continue
		}
		fieldPath := path + "." + field.Name
		if err := validateQueryField(field.Type, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

func validateQueryField(typ reflect.Type, path string) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == queryTimeType || typ == queryDurationType {
		return nil
	}
	if typ.Implements(textUnmarshalerType) || reflect.PointerTo(typ).Implements(textUnmarshalerType) ||
		typ.Implements(textMarshalerType) || reflect.PointerTo(typ).Implements(textMarshalerType) {
		return nil
	}

	switch typ.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return nil
	case reflect.Slice:
		return validateQueryField(typ.Elem(), path)
	case reflect.Struct:
		return validateQueryStructType(typ, path)
	default:
		return fmt.Errorf("query struct validation: field %s has unsupported type %s", path, typ)
	}
}
