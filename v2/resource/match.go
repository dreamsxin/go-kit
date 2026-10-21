package resource

import (
	"fmt"
	"strings"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// Matcher is a filter bound to a schema, ready to be applied to records.
//
// It exists as a type rather than a free function because a filter is usually
// applied to many records: validation happens once, when the matcher is built,
// and every record after that is evaluated without re-checking the shape of the
// tree.
type Matcher struct {
	filter Filter
	schema Schema
}

// NewMatcher validates a filter against a schema and returns the matcher that
// applies it. An unusable filter is reported here, once, rather than on the
// first record that happens to reach the offending operand.
func NewMatcher(filter Filter, schema Schema) (*Matcher, error) {
	if err := filter.Validate(schema); err != nil {
		return nil, err
	}
	return &Matcher{filter: filter, schema: schema}, nil
}

// Match reports whether one record satisfies the filter.
//
// The semantics are SQL's three-valued logic, and that is deliberate: the
// reference implementation has to agree with the databases the adapters are
// built on. A comparison against a field the record does not carry is unknown,
// not false; `not` of unknown is unknown; a record is returned only when the
// whole condition is true. So `{"deletedAt":{"$empty":true}}` matches a record
// without the field, and `not` of a condition on a missing field matches
// nothing — which is what `NOT (deleted_at = 'x')` does in SQL, and the reason a
// two-valued implementation would let rows through that a database would not.
//
// Stable: resource.filter-semantics — a comparison on a missing field is unknown and stays unknown through not, so a negated condition matches nothing rather than everything.
// Covered by: TestMatcherTreatsAMissingFieldAsUnknown, TestMatcherKeepsUnknownThroughNegation, TestMatcherMatchesEveryOperator
func (m *Matcher) Match(record Record) (bool, error) {
	if m.filter.IsZero() {
		return true, nil
	}
	value, err := m.evaluate(m.filter, record)
	if err != nil {
		return false, err
	}
	return value == triTrue, nil
}

// Match reports whether one record satisfies a filter.
//
// It compiles a Matcher per call, so a caller applying one condition to a set of
// records should build the Matcher instead.
func Match(filter Filter, record Record, schema Schema) (bool, error) {
	matcher, err := NewMatcher(filter, schema)
	if err != nil {
		return false, err
	}
	return matcher.Match(record)
}

// truth is a condition's outcome under three-valued logic.
type truth uint8

const (
	// triFalse is a condition the record definitively fails.
	triFalse truth = iota
	// triTrue is a condition the record definitively satisfies.
	triTrue
	// triUnknown is a condition the record cannot answer — a comparison
	// against a field it does not carry.
	triUnknown
)

// boolTruth lifts a decided answer into the three-valued domain.
func boolTruth(value bool) truth {
	if value {
		return triTrue
	}
	return triFalse
}

// negate flips a decided answer and leaves an undecided one alone.
func negate(value truth) truth {
	switch value {
	case triTrue:
		return triFalse
	case triFalse:
		return triTrue
	default:
		return triUnknown
	}
}

// evaluate walks the tree. A connective short-circuits on the answer that
// decides it, which is also what makes `and` a real narrowing: one false child
// is enough to reject the record whatever the others say.
func (m *Matcher) evaluate(f Filter, record Record) (truth, error) {
	switch f.Op {
	case OpAnd:
		result := triTrue
		for _, child := range f.Children {
			value, err := m.evaluate(child, record)
			if err != nil {
				return triUnknown, err
			}
			if value == triFalse {
				return triFalse, nil
			}
			if value == triUnknown {
				result = triUnknown
			}
		}
		return result, nil
	case OpOr:
		result := triFalse
		for _, child := range f.Children {
			value, err := m.evaluate(child, record)
			if err != nil {
				return triUnknown, err
			}
			if value == triTrue {
				return triTrue, nil
			}
			if value == triUnknown {
				result = triUnknown
			}
		}
		return result, nil
	case OpNot:
		value, err := m.evaluate(f.Children[0], record)
		if err != nil {
			return triUnknown, err
		}
		return negate(value), nil
	default:
		return m.compare(f, record)
	}
}

// compare evaluates one comparison against one record.
func (m *Matcher) compare(f Filter, record Record) (truth, error) {
	field, ok := m.schema.Field(f.Field)
	if !ok {
		// NewMatcher validated the tree, so this is a schema that changed under
		// a matcher that was already built. Reporting it beats answering
		// "no match", which would look like an empty result set.
		return triUnknown, apperror.InvalidArgument("resource.unknown_field",
			fmt.Sprintf("field %q is not part of this resource", f.Field))
	}
	raw, present := record[f.Field]

	// Presence operators are the only ones that can decide a missing field.
	if f.Op == OpEmpty || f.Op == OpNotEmpty {
		empty := !present || isEmptyValue(raw)
		if f.Op == OpNotEmpty {
			return boolTruth(!empty), nil
		}
		return boolTruth(empty), nil
	}
	if !present || raw == nil {
		return triUnknown, nil
	}

	switch f.Op {
	case OpEq, OpNe:
		order, err := CompareValues(field.Type, raw, f.Value)
		if err != nil {
			return triUnknown, err
		}
		if f.Op == OpNe {
			return boolTruth(order != 0), nil
		}
		return boolTruth(order == 0), nil
	case OpGt, OpGte, OpLt, OpLte:
		order, err := CompareValues(field.Type, raw, f.Value)
		if err != nil {
			return triUnknown, err
		}
		return orderTruth(f.Op, order), nil
	case OpIn, OpNotIn:
		matched := false
		for _, operand := range elements(f.Value) {
			order, err := CompareValues(field.Type, raw, operand)
			if err != nil {
				return triUnknown, err
			}
			if order == 0 {
				matched = true
				break
			}
		}
		if f.Op == OpNotIn {
			return boolTruth(!matched), nil
		}
		return boolTruth(matched), nil
	case OpLike, OpPrefix:
		text, err := NormalizeValue(TypeString, raw)
		if err != nil {
			return triUnknown, err
		}
		needle, ok := f.Value.(string)
		if !ok {
			return triUnknown, apperror.InvalidArgument("resource.value_type_mismatch",
				fmt.Sprintf("%s takes a string, got %s", f.Op, describeValue(f.Value)))
		}
		if f.Op == OpPrefix {
			return boolTruth(strings.HasPrefix(text.(string), needle)), nil
		}
		return boolTruth(strings.Contains(text.(string), needle)), nil
	case OpDateRange:
		return m.inDateRange(field, raw, f.Value)
	default:
		return triUnknown, apperror.InvalidArgument("filter.unknown_operator",
			fmt.Sprintf("%q is not an operator", string(f.Op)))
	}
}

// inDateRange evaluates the half-open interval [from, to).
func (m *Matcher) inDateRange(field Field, raw, operand any) (truth, error) {
	interval := elements(operand)
	if len(interval) != 2 {
		return triUnknown, apperror.InvalidArgument("filter.shape",
			fmt.Sprintf("dateRange takes [from, to], got %v", operand))
	}
	afterStart, err := CompareValues(field.Type, raw, interval[0])
	if err != nil {
		return triUnknown, err
	}
	beforeEnd, err := CompareValues(field.Type, raw, interval[1])
	if err != nil {
		return triUnknown, err
	}
	// Half-open: the start is included and the end is not, so two adjacent
	// ranges never both claim the record on their shared boundary.
	return boolTruth(afterStart >= 0 && beforeEnd < 0), nil
}

// orderTruth turns a comparison result into the answer for an ordering operator.
func orderTruth(op Op, order int) truth {
	switch op {
	case OpGt:
		return boolTruth(order > 0)
	case OpGte:
		return boolTruth(order >= 0)
	case OpLt:
		return boolTruth(order < 0)
	default:
		return boolTruth(order <= 0)
	}
}

// isEmptyValue reports whether a present value counts as empty for the presence
// operators: null, the empty string, or empty bytes. A zero time is a value, not
// an absence, and stays non-empty.
func isEmptyValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []byte:
		return len(typed) == 0
	default:
		return false
	}
}
