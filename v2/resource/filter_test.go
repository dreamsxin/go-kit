package resource_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/resource"
)

// codeOf reports the stable error code a rejection carries, or "" when the error
// is not an application error at all. Tests assert the code rather than the
// message so the message stays free to improve.
func codeOf(err error) string {
	var invalid *apperror.Error
	if !errors.As(err, &invalid) {
		return ""
	}
	return invalid.ErrorCode()
}

func mustCanonical(t *testing.T, filter resource.Filter) string {
	t.Helper()
	encoded, err := json.Marshal(filter)
	if err != nil {
		t.Fatalf("Marshal(%v): %v", filter, err)
	}
	return string(encoded)
}

// TestFilterCanonicalJSONRoundTrips proves the stored form is the tree itself:
// what Marshals out Unmarshals back to the same tree, including a nested
// connective whose value is a number rather than a string.
func TestFilterCanonicalJSONRoundTrips(t *testing.T) {
	original := resource.And(
		resource.Condition("status", resource.OpEq, "active"),
		resource.Condition("age", resource.OpGte, json.Number("18")),
		resource.Or(
			resource.Condition("ownerId", resource.OpIn, []any{json.Number("7"), json.Number("9")}),
			resource.Not(resource.Condition("archived", resource.OpEq, true)),
		),
	)

	canonical := mustCanonical(t, original)
	var decoded resource.Filter
	if err := json.Unmarshal([]byte(canonical), &decoded); err != nil {
		t.Fatalf("Unmarshal(%s): %v", canonical, err)
	}
	if roundTripped := mustCanonical(t, decoded); roundTripped != canonical {
		t.Fatalf("round trip changed the tree:\n first: %s\nsecond: %s", canonical, roundTripped)
	}
	// The canonical form says which operator, on which field, with which value,
	// in one spelling — the property an audit log needs to be reviewable.
	want := `{"op":"and","children":[` +
		`{"op":"eq","field":"status","value":"active"},` +
		`{"op":"gte","field":"age","value":18},` +
		`{"op":"or","children":[` +
		`{"op":"in","field":"ownerId","value":[7,9]},` +
		`{"op":"not","children":[{"op":"eq","field":"archived","value":true}]}]}]}`
	if canonical != want {
		t.Fatalf("canonical form =\n %s\nwant\n %s", canonical, want)
	}
}

// TestZeroFilterIsAbsentNotAnEmptyObject proves "no condition" has one spelling.
// An empty object would be indistinguishable from a node whose fields were
// dropped on the way through, which is the shape a lost condition would take.
func TestZeroFilterIsAbsentNotAnEmptyObject(t *testing.T) {
	if got := mustCanonical(t, resource.Filter{}); got != "null" {
		t.Fatalf("zero filter marshals as %s, want null", got)
	}
	var filter resource.Filter
	if err := json.Unmarshal([]byte("null"), &filter); err != nil {
		t.Fatalf("Unmarshal(null): %v", err)
	}
	if !filter.IsZero() {
		t.Fatalf("null decoded to %+v, want the zero filter", filter)
	}
}

// TestOnlyTheZeroValueIsZero proves a node that names a field without an
// operator is reported, not treated as absent: dropping it would drop a
// condition, and a query that answers more records than it was asked for looks
// like an authorization failure from the outside.
func TestOnlyTheZeroValueIsZero(t *testing.T) {
	cases := map[string]resource.Filter{
		"field without an operator": {Field: "status"},
		"operator without a field":  {Op: resource.OpEq, Value: 1},
		"children without an op":    {Children: []resource.Filter{resource.Condition("a", resource.OpEq, 1)}},
		"a value alone":             {Value: json.Number("0")},
	}
	for name, filter := range cases {
		if filter.IsZero() {
			t.Errorf("%s is reported as no condition", name)
		}
	}
}

// TestAndDropsOnlyZeroOperands is the narrowing property in its structural form.
//
// And may only lose operands that state nothing. Every non-zero operand survives,
// in the order it was given, because the alternative — merging, deduplicating, or
// collapsing a nested And — is how a policy constraint disappears while the code
// still reads as though it had been applied.
func TestAndDropsOnlyZeroOperands(t *testing.T) {
	status := resource.Condition("status", resource.OpEq, "active")
	owner := resource.Condition("ownerId", resource.OpEq, json.Number("7"))

	combined := resource.And(resource.Filter{}, status, owner, resource.Filter{})
	if combined.Op != resource.OpAnd {
		t.Fatalf("And of two operands = %s, want an and node", combined.Op)
	}
	if len(combined.Children) != 2 {
		t.Fatalf("And kept %d operands, want 2: %s", len(combined.Children), mustCanonical(t, combined))
	}
	if combined.Children[0].Field != "status" || combined.Children[1].Field != "ownerId" {
		t.Fatalf("And reordered its operands: %s", mustCanonical(t, combined))
	}
	if got := mustCanonical(t, resource.And(status)); got != mustCanonical(t, status) {
		t.Fatalf("And of one operand = %s, want the operand itself", got)
	}
	if !resource.And().IsZero() {
		t.Fatal("And of nothing is not the zero filter")
	}
	if got := mustCanonical(t, resource.And(status, resource.Filter{})); got != mustCanonical(t, status) {
		t.Fatalf("And(condition, zero) = %s, want the condition", got)
	}
}

// TestAndKeepsBothConditionsOnOneField is the case a merge would break, and the
// case a permission layer actually produces: the client asked for one range and
// the policy adds another on the same column. Keeping only one of them — whichever
// it is — either widens the result to what the client asked for or silently
// changes the policy. Both conditions have to survive as separate operands.
func TestAndKeepsBothConditionsOnOneField(t *testing.T) {
	client := resource.Condition("age", resource.OpGte, json.Number("18"))
	policy := resource.Condition("age", resource.OpLte, json.Number("65"))
	combined := resource.And(client, policy)
	if len(combined.Children) != 2 {
		t.Fatalf("And of two conditions on one field kept %d: %s", len(combined.Children), mustCanonical(t, combined))
	}
	if got := mustCanonical(t, combined); got != `{"op":"and","children":[{"op":"gte","field":"age","value":18},{"op":"lte","field":"age","value":65}]}` {
		t.Fatalf("And of two conditions on one field = %s", got)
	}
}

// TestAndNestsRatherThanMerges proves a nested And stays a child of the new node.
// Flattening two Ands would preserve the meaning today and lose the provenance of
// which layer contributed which condition — the only thing that makes a denial
// explainable later.
func TestAndNestsRatherThanMerges(t *testing.T) {
	inner := resource.And(
		resource.Condition("tenantId", resource.OpEq, "acme"),
		resource.Condition("status", resource.OpEq, "active"),
	)
	combined := resource.And(inner, resource.Condition("ownerId", resource.OpEq, json.Number("7")))
	if len(combined.Children) != 2 {
		t.Fatalf("combined children = %d, want the inner and as one operand", len(combined.Children))
	}
	if combined.Children[0].Op != resource.OpAnd {
		t.Fatalf("the inner and was flattened: %s", mustCanonical(t, combined))
	}
}

// TestOrKeepsAnEmptyOperandSoValidationRejectsIt proves the asymmetry between And
// and Or.
//
// Or's identity element is "match nothing", which no filter expresses, so an
// empty operand cannot be dropped without turning the disjunction into "match
// everything". It is kept, and validation refuses it.
func TestOrKeepsAnEmptyOperandSoValidationRejectsIt(t *testing.T) {
	status := resource.Condition("status", resource.OpEq, "active")
	disjunction := resource.Or(status, resource.Filter{})
	if len(disjunction.Children) != 2 {
		t.Fatalf("Or dropped an operand: %s", mustCanonical(t, disjunction))
	}
	if err := disjunction.Validate(resource.Schema{}); codeOf(err) != "filter.shape" {
		t.Fatalf("Or with an empty operand validated: %v", err)
	}
}

// TestNotTakesExactlyOneChild proves the negation's arity is enforced where the
// filter is built, before any adapter sees it.
func TestNotTakesExactlyOneChild(t *testing.T) {
	numbers := mustSchema(t, resource.NewField("a", resource.TypeInt), resource.NewField("b", resource.TypeInt))
	if err := resource.Not(resource.Condition("a", resource.OpEq, 1)).Validate(numbers); err != nil {
		t.Fatalf("Not of one condition: %v", err)
	}
	broken := resource.Filter{Op: resource.OpNot, Children: []resource.Filter{
		resource.Condition("a", resource.OpEq, 1),
		resource.Condition("b", resource.OpEq, 2),
	}}
	if err := broken.Validate(numbers); codeOf(err) != "filter.shape" {
		t.Fatalf("Not of two conditions validated: %v", err)
	}
}

// mustSchema builds a schema for tests that get far enough to name a field.
func mustSchema(t *testing.T, fields ...resource.Field) resource.Schema {
	t.Helper()
	schema, err := resource.NewSchema(fields...)
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return schema
}

// TestFilterRejectsAnUnreadableCanonicalForm proves the stored form is read
// strictly. An unknown key that is skipped is a condition that silently stopped
// applying.
func TestFilterRejectsAnUnreadableCanonicalForm(t *testing.T) {
	cases := map[string]string{
		"unknown key":            `{"op":"eq","field":"a","value":1,"extra":true}`,
		"not an object":          `[{"op":"eq","field":"a","value":1}]`,
		"logical with a field":   `{"op":"and","field":"a","children":[{"op":"eq","field":"a","value":1}]}`,
		"leaf without a field":   `{"op":"eq","value":1}`,
		"leaf without a value":   `{"op":"eq","field":"a"}`,
		"leaf with children":     `{"op":"eq","field":"a","value":1,"children":[{"op":"eq","field":"b","value":2}]}`,
		"and without children":   `{"op":"and"}`,
		"empty operand":          `{"op":"and","children":[{"op":"eq","field":"a","value":1},null]}`,
		"empty list for in":      `{"op":"in","field":"a","value":[]}`,
		"value on empty":         `{"op":"empty","field":"a","value":true}`,
		"unknown operator":       `{"op":"between","field":"a","value":1}`,
		"missing operator":       `{"field":"a","value":1}`,
		"date range of one":      `{"op":"dateRange","field":"a","value":["2026-01-01T00:00:00Z"]}`,
		"date range not a list":  `{"op":"dateRange","field":"a","value":"2026-01-01"}`,
		"empty value not a list": `{"op":"in","field":"a","value":"x"}`,
	}
	for name, document := range cases {
		var filter resource.Filter
		err := json.Unmarshal([]byte(document), &filter)
		if err == nil {
			t.Errorf("%s was accepted as %s", name, mustCanonical(t, filter))
			continue
		}
		if code := codeOf(err); code == "" {
			t.Errorf("%s failed with %v, which carries no application code", name, err)
		}
	}
}

// TestFilterLimitsApplyToBuiltTrees proves the size limits are a property of the
// value, not of the parser. A tree assembled in Go is walked by the same adapter,
// so it gets the same budget; otherwise the request-facing limit is a formality a
// caller can walk around.
func TestFilterLimitsApplyToBuiltTrees(t *testing.T) {
	deep := resource.Condition("a", resource.OpEq, 1)
	for i := 0; i < 40; i++ {
		deep = resource.Not(deep)
	}
	if err := deep.Validate(resource.Schema{}); codeOf(err) != "filter.too_deep" {
		t.Fatalf("a 40-level filter validated: %v", err)
	}

	wide := make([]resource.Filter, 0, 300)
	for i := 0; i < 300; i++ {
		wide = append(wide, resource.Condition("a", resource.OpEq, i))
	}
	if err := resource.And(wide...).Validate(resource.Schema{}); codeOf(err) != "filter.too_many_conditions" {
		t.Fatalf("a 300-condition filter validated: %v", err)
	}
}
