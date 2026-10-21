package resource

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// NormalizeValue converts a value into the Go form its field type is compared
// in: int64, float64, string, bool, time.Time, or — for TypeJSON — the value
// itself.
//
// It exists because a filter operand, a stored column and a JSON request body
// arrive as three different Go types for the same number: json.Number from a
// decoded request, int64 from a database driver, int from application code. Two
// of those compare unequal with ==, so an adapter that skips this step answers
// a query about 3 with "no rows".
//
// The conversion is idempotent: normalizing an int64 yields an int64. Adapters
// may call it as often as they like.
func NormalizeValue(kind FieldType, value any) (any, error) {
	if value == nil {
		return nil, errValueType(kind, value)
	}
	switch kind {
	case TypeString:
		switch typed := value.(type) {
		case string:
			return typed, nil
		case []byte:
			// A driver may hand text columns back as bytes.
			return string(typed), nil
		}
	case TypeInt:
		if number, ok := numberOf(value); ok {
			if integer, err := strconv.ParseInt(number.String(), 10, 64); err == nil {
				return integer, nil
			}
			// "3.0" from a JSON body is the integer 3; "3.5" is not an
			// integer and must not become one.
			if fraction, err := strconv.ParseFloat(number.String(), 64); err == nil &&
				!math.IsInf(fraction, 0) && fraction == math.Trunc(fraction) &&
				fraction >= math.MinInt64 && fraction <= math.MaxInt64 {
				return int64(fraction), nil
			}
		}
	case TypeFloat:
		if number, ok := numberOf(value); ok {
			if fraction, err := strconv.ParseFloat(number.String(), 64); err == nil {
				return fraction, nil
			}
		}
	case TypeBool:
		if typed, ok := value.(bool); ok {
			return typed, nil
		}
	case TypeTime:
		switch typed := value.(type) {
		case time.Time:
			return typed, nil
		case string:
			if parsed, err := time.Parse(time.RFC3339, typed); err == nil {
				return parsed, nil
			}
		}
	case TypeJSON:
		// An opaque document has no normalized form: it is compared only for
		// presence, which needs no conversion.
		return value, nil
	}
	return nil, errValueType(kind, value)
}

// CompareValues orders two values of one field type: -1, 0 or 1.
//
// Both operands are normalized first, so a caller may pass whatever it holds.
// A JSON document has no order and is rejected: sorting opaque values by their
// serialization would put {"a":1} before {"b":0} for reasons no reader could
// predict.
func CompareValues(kind FieldType, left, right any) (int, error) {
	first, err := NormalizeValue(kind, left)
	if err != nil {
		return 0, err
	}
	second, err := NormalizeValue(kind, right)
	if err != nil {
		return 0, err
	}
	switch kind {
	case TypeInt:
		return compareOrdered(first.(int64), second.(int64)), nil
	case TypeFloat:
		return compareOrdered(first.(float64), second.(float64)), nil
	case TypeString:
		return compareOrdered(first.(string), second.(string)), nil
	case TypeBool:
		return compareOrdered(boolRank(first.(bool)), boolRank(second.(bool))), nil
	case TypeTime:
		return first.(time.Time).Compare(second.(time.Time)), nil
	default:
		return 0, apperror.InvalidArgument("resource.value_not_orderable",
			fmt.Sprintf("a %s field has no order", kind))
	}
}

// boolRank gives false a lower rank than true, so a boolean column sorts in one
// predictable direction rather than by whichever storage encoded it.
func boolRank(value bool) int {
	if value {
		return 1
	}
	return 0
}

// compareOrdered is the ordering helper for types whose Go comparison is
// already total.
func compareOrdered[T int64 | float64 | string | int](left, right T) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

// valueFits reports whether one operand belongs to a field's value domain.
func valueFits(kind FieldType, value any) bool {
	_, err := NormalizeValue(kind, value)
	return err == nil
}

// errValueType names both sides of a mismatch. "expects int, got a string" is
// actionable; "invalid value" sends the reader back to the request body.
func errValueType(kind FieldType, value any) error {
	if value == nil {
		return apperror.InvalidArgument("resource.value_type_mismatch",
			fmt.Sprintf("a %s field has no value", kind))
	}
	return apperror.InvalidArgument("resource.value_type_mismatch",
		fmt.Sprintf("a %s field cannot hold %s (%v)", kind, describeValue(value), value))
}

// numberOf accepts every numeric Go type a caller may hold and returns it as a
// json.Number, which is how a parsed filter carries one.
func numberOf(value any) (json.Number, bool) {
	switch typed := value.(type) {
	case json.Number:
		return typed, true
	case int:
		return json.Number(strconv.FormatInt(int64(typed), 10)), true
	case int8:
		return json.Number(strconv.FormatInt(int64(typed), 10)), true
	case int16:
		return json.Number(strconv.FormatInt(int64(typed), 10)), true
	case int32:
		return json.Number(strconv.FormatInt(int64(typed), 10)), true
	case int64:
		return json.Number(strconv.FormatInt(typed, 10)), true
	case uint:
		return json.Number(strconv.FormatUint(uint64(typed), 10)), true
	case uint8:
		return json.Number(strconv.FormatUint(uint64(typed), 10)), true
	case uint16:
		return json.Number(strconv.FormatUint(uint64(typed), 10)), true
	case uint32:
		return json.Number(strconv.FormatUint(uint64(typed), 10)), true
	case uint64:
		return json.Number(strconv.FormatUint(typed, 10)), true
	case float32:
		return json.Number(strconv.FormatFloat(float64(typed), 'g', -1, 32)), true
	case float64:
		return json.Number(strconv.FormatFloat(typed, 'g', -1, 64)), true
	default:
		return "", false
	}
}

// describeValue names the Go type an operand arrived as, so the error tells a
// client what it sent rather than only what it should have sent.
func describeValue(value any) string {
	switch value.(type) {
	case nil:
		return "nothing"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case json.Number:
		return "a number"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func knownFieldType(kind FieldType) bool {
	switch kind {
	case TypeString, TypeInt, TypeFloat, TypeBool, TypeTime, TypeJSON:
		return true
	default:
		return false
	}
}

// orderedFieldType reports whether a field type has an order, which is what
// gt/gte/lt/lte require.
func orderedFieldType(kind FieldType) bool {
	switch kind {
	case TypeString, TypeInt, TypeFloat, TypeTime:
		return true
	default:
		return false
	}
}

// numericFieldType reports whether a field type supports sum and average.
func numericFieldType(kind FieldType) bool {
	switch kind {
	case TypeInt, TypeFloat:
		return true
	default:
		return false
	}
}
