package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// Op names a comparison or a connective in a Filter.
//
// The set is closed on purpose. An operator is a promise every repository
// adapter has to keep, so an application that could register its own would be
// registering a query fragment the policy layer cannot reason about and the
// audit log cannot replay — which is the injection surface this package exists
// to remove. A condition the closed set cannot express is a schema problem, not
// an operator problem.
type Op string

// The comparison and logical operators a Filter may carry. Comparisons apply to
// one field; the logical operators apply to Children.
const (
	// OpEq matches records whose field equals Value.
	OpEq Op = "eq"
	// OpNe matches records whose field differs from Value.
	OpNe Op = "ne"
	// OpIn matches records whose field is one of Value, a non-empty list.
	OpIn Op = "in"
	// OpNotIn matches records whose field is none of Value, a non-empty list.
	OpNotIn Op = "nin"
	// OpGt, OpGte, OpLt and OpLte order a field against Value. They are only
	// valid on an ordered field type: string, int, float and time.
	OpGt  Op = "gt"
	OpGte Op = "gte"
	OpLt  Op = "lt"
	OpLte Op = "lte"
	// OpLike matches a string field that contains Value. Value is a literal:
	// "%" and "_" are the characters they look like, not wildcards. Wildcard
	// syntax belongs to SQL, and a filter that means something different
	// depending on which adapter runs it is worse than one that says less.
	OpLike Op = "like"
	// OpPrefix matches a string field that starts with Value.
	OpPrefix Op = "prefix"
	// OpDateRange matches a time field inside the half-open interval
	// [Value[0], Value[1]): the start is included, the end is not, so two
	// adjacent ranges never both match the record on their boundary.
	OpDateRange Op = "dateRange"
	// OpEmpty matches a field that is null or the empty string. No Value.
	OpEmpty Op = "empty"
	// OpNotEmpty is the negation of OpEmpty. No Value.
	OpNotEmpty Op = "notEmpty"

	// OpAnd matches a record that satisfies every child.
	OpAnd Op = "and"
	// OpOr matches a record that satisfies at least one child.
	OpOr Op = "or"
	// OpNot matches a record that does not satisfy its single child.
	OpNot Op = "not"
)

// Filter is one node of a query condition tree: either a comparison against one
// field, or a logical connective over Children.
//
// The zero Filter matches every record. It is not a condition that was written
// and then lost; it is the absence of one, which is why And drops it and Or
// refuses it.
type Filter struct {
	// Op is the operator. The zero value means "no condition".
	Op Op
	// Field names the schema field a comparison applies to. Empty on a
	// logical node.
	Field string
	// Value is the right-hand side of a comparison. Its Go type follows the
	// field: a string for OpLike, a slice for OpIn and OpDateRange, and the
	// field's own value — string, json.Number, bool — everywhere else.
	Value any
	// Children holds the operands of OpAnd, OpOr and OpNot, and is empty on a
	// comparison.
	Children []Filter
}

// IsZero reports whether the filter states no condition at all.
//
// Only the zero value is zero. A node that names a field without naming an
// operator is not "empty" — it is a filter somebody built wrongly, and treating
// it as absent would drop a condition instead of rejecting it.
func (f Filter) IsZero() bool {
	return f.Op == "" && f.Field == "" && f.Value == nil && len(f.Children) == 0
}

// Condition builds a comparison against one field.
func Condition(field string, op Op, value any) Filter {
	return Filter{Op: op, Field: field, Value: value}
}

// And combines filters into one condition that every operand must satisfy.
//
// Zero operands are dropped, because a condition that matches everything adds
// nothing to a conjunction — and this is the only simplification And performs.
// Narrowing the result further, deduplicating operands, or merging nested And
// nodes into one would all be ways for a policy constraint to disappear while
// the code still reads as though it had been applied.
//
// And with no non-zero operand — and no operand at all — returns the zero
// Filter, which matches everything: an empty constraint is the absence of a
// constraint, and callers that must not allow that (a policy that denied the
// request) deny it before this point.
func And(filters ...Filter) Filter {
	operands := make([]Filter, 0, len(filters))
	for _, filter := range filters {
		if filter.IsZero() {
			continue
		}
		operands = append(operands, filter)
	}
	switch len(operands) {
	case 0:
		return Filter{}
	case 1:
		return operands[0]
	}
	return Filter{Op: OpAnd, Children: operands}
}

// Or combines filters into one condition that at least one operand must
// satisfy.
//
// A zero operand is kept, not dropped, and Validate rejects it. Dropping it
// would say "this alternative adds nothing", but a condition that matches
// everything is not nothing: Or(everything, x) matches every record, while
// Or(x) matches some of them. A filter that turns into "allow everything"
// because one alternative was empty is the exact failure a permission layer
// must not have, so it is an error here instead.
func Or(filters ...Filter) Filter {
	return Filter{Op: OpOr, Children: append([]Filter(nil), filters...)}
}

// Not negates one filter.
func Not(filter Filter) Filter {
	return Filter{Op: OpNot, Children: []Filter{filter}}
}

// MarshalJSON writes the canonical form, which is the tree itself:
//
//	{"op":"and","children":[{"op":"eq","field":"status","value":"active"}]}
//
// This is the form an audit log stores and a bug report replays — not the
// dialect clients send. The dialect is an input grammar with several spellings
// for one condition; this form has exactly one, so a reviewer reading two log
// lines can compare them.
func (f Filter) MarshalJSON() ([]byte, error) {
	type node struct {
		Op       Op       `json:"op"`
		Field    string   `json:"field,omitempty"`
		Value    any      `json:"value,omitempty"`
		Children []Filter `json:"children,omitempty"`
	}
	if f.IsZero() {
		// null rather than {}: a stored condition that turns out to be absent
		// must be readable as absent, not as an object whose fields happen to
		// be missing.
		return []byte("null"), nil
	}
	return json.Marshal(node{Op: f.Op, Field: f.Field, Value: f.Value, Children: f.Children})
}

// UnmarshalJSON reads the canonical form, strictly.
//
// Unknown keys are an error rather than something to skip. A silently ignored
// key in a stored condition is a condition that quietly stopped applying, and
// the record set then answers a broader question than the one that was asked —
// which is indistinguishable from a permission bug at the call site.
func (f *Filter) UnmarshalJSON(data []byte) error {
	if trimmed := bytes.TrimSpace(data); bytes.Equal(trimmed, []byte("null")) {
		*f = Filter{}
		return nil
	}
	nodes := 0
	decoded, err := decodeFilterNode(data, 0, &nodes)
	if err != nil {
		return err
	}
	if err := decoded.checkShape(); err != nil {
		return err
	}
	*f = decoded
	return nil
}

// decodeFilterNode decodes one canonical node and its children.
//
// Children are repassed through RawMessage so the depth and node budgets are
// counted by this loop. Letting encoding/json recurse through Filter itself
// would hand the recursion limit to a decoder that has none of these bounds.
func decodeFilterNode(data []byte, depth int, nodes *int) (Filter, error) {
	*nodes++
	if *nodes > maxFilterNodes {
		return Filter{}, errTooManyConditions()
	}
	if depth > maxFilterDepth {
		return Filter{}, errTooDeep()
	}
	var raw struct {
		Op       Op                `json:"op"`
		Field    string            `json:"field"`
		Value    json.RawMessage   `json:"value"`
		Children []json.RawMessage `json:"children"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return Filter{}, apperror.InvalidArgument("filter.shape",
			fmt.Sprintf("a filter node must be an object of op, field, value and children: %v", err))
	}
	node := Filter{Op: raw.Op, Field: raw.Field}
	if len(raw.Value) > 0 {
		value, err := decodeFilterValue(raw.Value)
		if err != nil {
			return Filter{}, err
		}
		node.Value = value
	}
	for _, childData := range raw.Children {
		child, err := decodeFilterNode(childData, depth+1, nodes)
		if err != nil {
			return Filter{}, err
		}
		node.Children = append(node.Children, child)
	}
	return node, nil
}

// decodeFilterValue keeps integers as json.Number. A record identifier larger
// than 2^53 is exact in a JSON document and wrong in a float64, so the value is
// carried as the text it arrived as and converted by the adapter that finally
// compares it.
func decodeFilterValue(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, apperror.InvalidArgument("filter.shape",
			fmt.Sprintf("a filter value must be JSON: %v", err))
	}
	return value, nil
}

// reject builds the error for a node at path, so a nested condition reports
// which operand is wrong — children[1].children[0] is the difference between a
// message that can be acted on and one that sends the reader back to the request
// body.
func reject(path, code, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	if path == "" {
		return apperror.InvalidArgument(code, message)
	}
	return apperror.InvalidArgument(code, path+": "+message)
}

// checkShape enforces the structure every Filter must have before a schema is
// consulted: a known operator, one field on a comparison, children on a
// connective, the right arity for both, and the size limits a request-facing
// tree needs. It applies to a filter built in Go as much as to one parsed from a
// request: an adapter walks this tree either way.
func (f Filter) checkShape() error {
	nodes := 0
	return f.checkShapeAt("", 0, &nodes)
}

func (f Filter) checkShapeAt(path string, depth int, nodes *int) error {
	*nodes++
	if *nodes > maxFilterNodes {
		return errTooManyConditions()
	}
	if depth > maxFilterDepth {
		return errTooDeep()
	}
	switch f.Op {
	case OpAnd, OpOr:
		if f.Field != "" || f.Value != nil {
			return reject(path, "filter.shape",
				"%s is a logical operator and takes children, not field %q or a value", f.Op, f.Field)
		}
		if len(f.Children) == 0 {
			return reject(path, "filter.shape", "%s needs at least one child", f.Op)
		}
	case OpNot:
		if f.Field != "" || f.Value != nil {
			return reject(path, "filter.shape",
				"not is a logical operator and takes one child, not field %q or a value", f.Field)
		}
		if len(f.Children) != 1 {
			return reject(path, "filter.shape", "not takes exactly one child, got %d", len(f.Children))
		}
	case OpEq, OpNe, OpIn, OpNotIn, OpGt, OpGte, OpLt, OpLte, OpLike, OpPrefix, OpDateRange, OpEmpty, OpNotEmpty:
		if f.Field == "" {
			return reject(path, "filter.field_required", "%s needs a field", f.Op)
		}
		if len(f.Children) != 0 {
			return reject(path, "filter.shape", "%s compares one field and takes no children", f.Op)
		}
		if err := checkOperand(f, path); err != nil {
			return err
		}
	default:
		return reject(path, "filter.unknown_operator", "%q is not an operator", string(f.Op))
	}
	for i, child := range f.Children {
		childPath := joinChildPath(path, i)
		if child.IsZero() {
			return reject(childPath, "filter.shape",
				"states no condition; an operand that matches everything is an error, not a no-op")
		}
		if err := child.checkShapeAt(childPath, depth+1, nodes); err != nil {
			return err
		}
	}
	return nil
}

// checkOperand enforces the operand shape of a comparison: how many values the
// operator takes and whether it takes any at all.
func checkOperand(f Filter, path string) error {
	switch f.Op {
	case OpEmpty, OpNotEmpty:
		if f.Value != nil {
			return reject(path, "filter.shape", "%s takes no value, got %v", f.Op, f.Value)
		}
	case OpIn, OpNotIn:
		count, ok := listLen(f.Value)
		if !ok {
			return reject(path, "filter.shape", "%s takes a list, got %v", f.Op, f.Value)
		}
		if count == 0 {
			return reject(path, "filter.shape", "%s takes a non-empty list", f.Op)
		}
	case OpDateRange:
		count, ok := listLen(f.Value)
		if !ok || count != 2 {
			return reject(path, "filter.shape", "dateRange takes [from, to], got %v", f.Value)
		}
	default:
		if f.Value == nil {
			return reject(path, "filter.value_required", "%s needs a value", f.Op)
		}
	}
	return nil
}

// joinChildPath extends a path to one operand.
func joinChildPath(path string, index int) string {
	child := fmt.Sprintf("children[%d]", index)
	if path == "" {
		return child
	}
	return path + "." + child
}

// listLen reports the length of a slice or array value, which is how OpIn and
// OpDateRange receive their operands. A caller building a filter in Go naturally
// writes []string or []int; the parsed form is []any.
func listLen(value any) (int, bool) {
	if value == nil {
		return 0, false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Slice, reflect.Array:
		return reflected.Len(), true
	default:
		return 0, false
	}
}

// Limits on a condition tree that arrives from a client. Neither is a
// performance guard: a query API that will happily build a thousand-node
// condition from one request is a denial-of-service primitive, and an unbounded
// depth is a stack overflow behind an authentication check.
const (
	maxFilterDepth = 32
	maxFilterNodes = 256
)

func errTooDeep() error {
	return apperror.InvalidArgument("filter.too_deep",
		fmt.Sprintf("a filter nests at most %d levels", maxFilterDepth))
}

func errTooManyConditions() error {
	return apperror.InvalidArgument("filter.too_many_conditions",
		fmt.Sprintf("a filter carries at most %d conditions", maxFilterNodes))
}

// trimDollar strips the dialect prefix from an operator key, or reports that the
// key was not an operator at all.
func trimDollar(key string) (Op, bool) {
	name, found := strings.CutPrefix(key, "$")
	if !found || name == "" {
		return "", false
	}
	return Op(name), true
}
