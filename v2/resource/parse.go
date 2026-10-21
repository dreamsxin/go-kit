package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// ParseFilter reads the filter dialect clients send and returns the condition
// tree it spells.
//
// The dialect is one JSON object whose keys are either field names or the
// logical operators $and, $or and $not:
//
//	{"status":{"$eq":"active"},"age":{"$gte":18,"$lt":65}}
//	{"$or":[{"ownerId":7},{"$and":[{"shared":true},{"status":"active"}]}]}
//	{"id":{"$in":[3,4]},"publishedAt":{"$dateRange":["2026-01-01T00:00:00Z","2026-02-01T00:00:00Z"]}}
//
// raw may be any of the shapes a caller actually holds: a json.RawMessage from a
// request body, a string from a query parameter (which may itself carry the
// document as text), a map[string]any from a decoded tool call, or nil for "no
// filter". One parser for all of them, because a second entry point per shape is
// a second dialect in everything but name.
//
// Several comparisons on one field are a conjunction, which is why
// {"$gte":18,"$lt":65} is one condition on age and not one of them silently
// winning. Keys are visited in sorted order so the same document always produces
// the same tree: two audit records for the same query have to be comparable, and
// Go map iteration would make them differ at random.
//
// The result is structurally checked but not validated against a schema; call
// Validate for that. Parsing answers "is this a condition", validation answers
// "is this condition about this resource".
//
// Stable: resource.filter-dialect — a filter the schema cannot describe is rejected with a filter.* code and never narrowed silently, and one document always parses to one tree, with operands in sorted order, so two audit records of the same query are comparable.
// Covered by: TestParseFilterSpeaksTheDocumentedDialect, TestParseFilterRejectsWhatItCannotMean, TestParseFilterIsDeterministic, TestValidateRejectsFieldsTheResourceDoesNotExpose
func ParseFilter(raw any) (Filter, error) {
	nodes := 0
	filter, err := parseFilterValue(raw, "", 0, &nodes)
	if err != nil {
		return Filter{}, err
	}
	// The dialect builds only well-shaped nodes, but its operands are whatever
	// the client wrote. Checking here is what turns an empty $in list into a 400
	// at the edge instead of a repository error nobody attributes to the query.
	if filter.IsZero() {
		return Filter{}, nil
	}
	if err := filter.checkShape(); err != nil {
		return Filter{}, err
	}
	return filter, nil
}

func parseFilterValue(raw any, path string, depth int, nodes *int) (Filter, error) {
	switch typed := raw.(type) {
	case nil:
		return Filter{}, nil
	case json.RawMessage:
		document, err := decodeFilterDocument(typed, path)
		if err != nil {
			return Filter{}, err
		}
		if document == nil {
			return Filter{}, nil
		}
		return parseFilterValue(document, path, depth, nodes)
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return Filter{}, nil
		}
		document, err := decodeFilterDocument([]byte(trimmed), path)
		if err != nil {
			return Filter{}, err
		}
		return parseFilterValue(document, path, depth, nodes)
	case map[string]any:
		return parseFilterObject(typed, path, depth, nodes)
	default:
		return Filter{}, apperror.InvalidArgument("filter.not_an_object",
			fmt.Sprintf("%sa filter is a JSON object, got %s", pathPrefix(path), describeValue(raw)))
	}
}

// decodeFilterDocument turns JSON text into the generic value the dialect walk
// expects. Numbers stay json.Number so an identifier wider than 2^53 is not
// rounded on the way in.
func decodeFilterDocument(data []byte, path string) (any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, apperror.InvalidArgument("filter.malformed",
			fmt.Sprintf("%sis not readable as JSON: %v", pathPrefix(path), err))
	}
	// A document followed by a second one is two requests wearing one string;
	// taking the first would answer a question nobody asked in full.
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, apperror.InvalidArgument("filter.malformed",
			fmt.Sprintf("%scarries more than one JSON document", pathPrefix(path)))
	}
	return document, nil
}

// parseFilterObject walks one dialect object: a set of conditions to be
// satisfied together.
func parseFilterObject(object map[string]any, path string, depth int, nodes *int) (Filter, error) {
	*nodes++
	if *nodes > maxFilterNodes {
		return Filter{}, errTooManyConditions()
	}
	if depth > maxFilterDepth {
		return Filter{}, errTooDeep()
	}
	if len(object) == 0 {
		// {} is how a client says "no conditions", the same as omitting the
		// parameter. It is not a condition that was written and lost.
		return Filter{}, nil
	}

	keys := sortedKeys(object)
	conditions := make([]Filter, 0, len(keys))
	for _, key := range keys {
		value := object[key]
		nodePath := joinPath(path, key)
		switch {
		case key == "$and" || key == "$or":
			children, err := parseLogicalOperands(Op(strings.TrimPrefix(key, "$")), value, nodePath, depth, nodes)
			if err != nil {
				return Filter{}, err
			}
			conditions = append(conditions, children...)
		case key == "$not":
			child, err := parseNegation(value, nodePath, depth, nodes)
			if err != nil {
				return Filter{}, err
			}
			conditions = append(conditions, child)
		case strings.HasPrefix(key, "$"):
			return Filter{}, apperror.InvalidArgument("filter.unknown_operator",
				fmt.Sprintf("%s is not an operator; a filter's keys are field names, $and, $or or $not", nodePath))
		default:
			fieldConditions, err := parseFieldConditions(key, value, nodePath)
			if err != nil {
				return Filter{}, err
			}
			conditions = append(conditions, fieldConditions...)
		}
	}
	return And(conditions...), nil
}

// parseLogicalOperands reads $and and $or, whose value is the list of conditions
// being combined.
func parseLogicalOperands(op Op, value any, path string, depth int, nodes *int) ([]Filter, error) {
	list, ok := value.([]any)
	if !ok {
		return nil, apperror.InvalidArgument("filter.logical_requires_list",
			fmt.Sprintf("%s takes a list of conditions, got %s", path, describeValue(value)))
	}
	if len(list) == 0 {
		return nil, apperror.InvalidArgument("filter.shape",
			fmt.Sprintf("%s takes at least one condition", path))
	}
	children := make([]Filter, 0, len(list))
	for i, item := range list {
		child, err := parseFilterValue(item, fmt.Sprintf("%s[%d]", path, i), depth+1, nodes)
		if err != nil {
			return nil, err
		}
		if child.IsZero() {
			return nil, apperror.InvalidArgument("filter.shape",
				fmt.Sprintf("%s[%d] states no condition", path, i))
		}
		children = append(children, child)
	}
	if op == OpOr {
		return []Filter{Or(children...)}, nil
	}
	return children, nil
}

// parseNegation reads $not, whose value is the single condition being negated.
func parseNegation(value any, path string, depth int, nodes *int) (Filter, error) {
	if list, ok := value.([]any); ok {
		if len(list) != 1 {
			return Filter{}, apperror.InvalidArgument("filter.shape",
				fmt.Sprintf("%s takes exactly one condition, got %d", path, len(list)))
		}
		value = list[0]
	}
	child, err := parseFilterValue(value, path, depth+1, nodes)
	if err != nil {
		return Filter{}, err
	}
	if child.IsZero() {
		return Filter{}, apperror.InvalidArgument("filter.shape", fmt.Sprintf("%s states no condition", path))
	}
	return Not(child), nil
}

// parseFieldConditions reads one field's entry: either a literal value, which
// means equality, or an object of operators.
func parseFieldConditions(field string, value any, path string) ([]Filter, error) {
	operators, ok := value.(map[string]any)
	if !ok {
		// A literal is the short form of equality, so {"status":"active"} and
		// {"status":{"$eq":"active"}} are the same condition.
		return []Filter{Condition(field, OpEq, value)}, nil
	}
	keys := sortedKeys(operators)
	conditions := make([]Filter, 0, len(keys))
	for _, key := range keys {
		op, isOperator := trimDollar(key)
		if !isOperator {
			// {"age":{"value":18}} is somebody guessing at the syntax. Reading
			// it as equality would answer a different question than the one they
			// asked, and reading it as "no condition" would answer a broader one.
			return nil, apperror.InvalidArgument("filter.expected_operator",
				fmt.Sprintf("%s: %q is not an operator; write {\"$eq\": ...} or a literal value", path, key))
		}
		if op.isLogical() {
			return nil, apperror.InvalidArgument("filter.logical_not_allowed_here",
				fmt.Sprintf("%s: %s combines conditions and belongs at the top level", path, key))
		}
		if !op.isComparison() {
			return nil, apperror.InvalidArgument("filter.unknown_operator",
				fmt.Sprintf("%s: %s is not an operator", path, key))
		}
		operand := operators[key]
		if op == OpEmpty || op == OpNotEmpty {
			// The dialect spells these with true, or with nothing at all,
			// because JSON has no other way to say "no operand". false is a
			// different condition someone may want; it has its own operator
			// rather than a flag on this one.
			if operand != nil && operand != true {
				return nil, apperror.InvalidArgument("filter.shape",
					fmt.Sprintf("%s takes true or no value; write %s instead of %s: false", path, oppositeEmpty(op), key))
			}
			operand = nil
		}
		conditions = append(conditions, Condition(field, op, operand))
	}
	return conditions, nil
}

// oppositeEmpty names the operator a client meant when it wrote false for the
// other one, so the error tells them the spelling rather than only refusing.
func oppositeEmpty(op Op) string {
	if op == OpEmpty {
		return "$notEmpty"
	}
	return "$empty"
}

func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func pathPrefix(path string) string {
	if path == "" {
		return ""
	}
	return path + ": "
}
