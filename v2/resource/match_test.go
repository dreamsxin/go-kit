package resource_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/resource"
)

func matchSchema(t *testing.T) resource.Schema {
	t.Helper()
	return mustSchema(t,
		resource.NewField("id", resource.TypeInt),
		resource.NewField("name", resource.TypeString),
		resource.NewField("active", resource.TypeBool),
		resource.NewField("score", resource.TypeFloat),
		resource.NewField("created", resource.TypeTime),
		resource.NewField("doc", resource.TypeJSON),
	)
}

// mustMatch runs one filter against one record.
func mustMatch(t *testing.T, schema resource.Schema, filter resource.Filter, record resource.Record) bool {
	t.Helper()
	matched, err := resource.Match(filter, record, schema)
	if err != nil {
		t.Fatalf("Match(%v): %v", filter, err)
	}
	return matched
}

// TestMatcherMatchesEveryOperator is the semantics table: one row per operator,
// with the answer the filter documentation promises. It is the definition every
// storage adapter has to reproduce, written as an executable example.
func TestMatcherMatchesEveryOperator(t *testing.T) {
	schema := matchSchema(t)
	january := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	february := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		record resource.Record
		filter resource.Filter
		want   bool
	}{
		{
			name:   "eq compares a stored int64 with a decoded number",
			record: resource.Record{"id": int64(7)},
			filter: resource.Condition("id", resource.OpEq, json.Number("7")),
			want:   true,
		},
		{
			name:   "eq rejects a different integer",
			record: resource.Record{"id": int64(7)},
			filter: resource.Condition("id", resource.OpEq, 8),
			want:   false,
		},
		{
			name:   "ne accepts a different string",
			record: resource.Record{"name": "ling"},
			filter: resource.Condition("name", resource.OpNe, "wei"),
			want:   true,
		},
		{
			name:   "gt orders integers",
			record: resource.Record{"id": 9},
			filter: resource.Condition("id", resource.OpGt, 8),
			want:   true,
		},
		{
			name:   "gte includes the boundary",
			record: resource.Record{"id": 8},
			filter: resource.Condition("id", resource.OpGte, 8),
			want:   true,
		},
		{
			name:   "lt orders floats",
			record: resource.Record{"score": 1.5},
			filter: resource.Condition("score", resource.OpLt, 1.25),
			want:   false,
		},
		{
			name:   "lte includes the boundary",
			record: resource.Record{"score": 1.25},
			filter: resource.Condition("score", resource.OpLte, 1.25),
			want:   true,
		},
		{
			name:   "in matches one of a list",
			record: resource.Record{"name": "ling"},
			filter: resource.Condition("name", resource.OpIn, []string{"wei", "ling"}),
			want:   true,
		},
		{
			name:   "nin rejects a listed value",
			record: resource.Record{"name": "ling"},
			filter: resource.Condition("name", resource.OpNotIn, []string{"wei", "ling"}),
			want:   false,
		},
		{
			name:   "like is a literal substring",
			record: resource.Record{"name": "ling"},
			filter: resource.Condition("name", resource.OpLike, "in"),
			want:   true,
		},
		{
			name:   "prefix matches the start only",
			record: resource.Record{"name": "ling"},
			filter: resource.Condition("name", resource.OpPrefix, "ing"),
			want:   false,
		},
		{
			name:   "empty matches an absent field",
			record: resource.Record{"id": 1},
			filter: resource.Condition("name", resource.OpEmpty, nil),
			want:   true,
		},
		{
			name:   "empty matches the empty string",
			record: resource.Record{"name": ""},
			filter: resource.Condition("name", resource.OpEmpty, nil),
			want:   true,
		},
		{
			name:   "notEmpty rejects an absent field",
			record: resource.Record{"id": 1},
			filter: resource.Condition("name", resource.OpNotEmpty, nil),
			want:   false,
		},
		{
			name:   "dateRange includes the start",
			record: resource.Record{"created": february.Add(-time.Second)},
			filter: resource.Condition("created", resource.OpDateRange,
				[]time.Time{january, february}),
			want: true,
		},
		{
			name:   "dateRange excludes the end",
			record: resource.Record{"created": february},
			filter: resource.Condition("created", resource.OpDateRange,
				[]time.Time{january, february}),
			want: false,
		},
		{
			name:   "and needs every child",
			record: resource.Record{"name": "ling", "active": true},
			filter: resource.And(
				resource.Condition("name", resource.OpEq, "ling"),
				resource.Condition("active", resource.OpEq, false),
			),
			want: false,
		},
		{
			name:   "or needs one child",
			record: resource.Record{"name": "ling", "active": true},
			filter: resource.Or(
				resource.Condition("name", resource.OpEq, "wei"),
				resource.Condition("active", resource.OpEq, true),
			),
			want: true,
		},
		{
			name:   "not inverts a decided child",
			record: resource.Record{"name": "ling"},
			filter: resource.Not(resource.Condition("name", resource.OpEq, "ling")),
			want:   false,
		},
		{
			name:   "an empty filter matches everything",
			record: resource.Record{},
			filter: resource.Filter{},
			want:   true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := mustMatch(t, schema, testCase.filter, testCase.record); got != testCase.want {
				t.Fatalf("matched = %v, want %v (record %v, filter %v)", got, testCase.want, testCase.record, testCase.filter)
			}
		})
	}
}

// TestMatcherTreatsAMissingFieldAsUnknown proves the reference semantics are
// SQL's. A record that does not carry the field cannot answer "is it equal" or
// "is it different", and both answers exclude it from the result.
func TestMatcherTreatsAMissingFieldAsUnknown(t *testing.T) {
	schema := matchSchema(t)
	record := resource.Record{"id": 1}
	for _, op := range []resource.Op{resource.OpEq, resource.OpNe, resource.OpGt, resource.OpLike} {
		filter := resource.Condition("name", op, "ling")
		if mustMatch(t, schema, filter, record) {
			t.Fatalf("%s on a missing field matched; an undecidable condition must not", op)
		}
	}
}

// TestMatcherKeepsUnknownThroughNegation is the reason three-valued logic is
// implemented at all. With two values, `not(name = 'ling')` on a record without
// a name would be true, and a query for "everything except ling" would return
// every incomplete record — a difference no SQL adapter would reproduce and no
// reader would expect.
func TestMatcherKeepsUnknownThroughNegation(t *testing.T) {
	schema := matchSchema(t)
	record := resource.Record{"id": 1}
	filter := resource.Not(resource.Condition("name", resource.OpEq, "ling"))
	if mustMatch(t, schema, filter, record) {
		t.Fatal("not(unknown) matched; the negated condition was undecidable")
	}
	if !mustMatch(t, schema, filter, resource.Record{"name": "wei"}) {
		t.Fatal("not(eq) did not match a record that answers the question")
	}
}

// TestMatcherCombinesUnknownWithDecidedAnswers proves the connectives keep the
// distinction: a false operand decides an and whatever the rest say, a true one
// decides an or, and an unknown operand leaves the result undecided rather than
// guessing.
func TestMatcherCombinesUnknownWithDecidedAnswers(t *testing.T) {
	schema := matchSchema(t)
	unknown := resource.Condition("name", resource.OpEq, "ling")
	yes := resource.Condition("id", resource.OpEq, 1)
	no := resource.Condition("id", resource.OpEq, 2)
	record := resource.Record{"id": 1}

	cases := map[string]struct {
		filter resource.Filter
		want   bool
	}{
		"and with a false operand is false":        {resource.And(unknown, no), false},
		"and with an unknown operand is undecided": {resource.And(unknown, yes), false},
		"or with a true operand is true":           {resource.Or(unknown, yes), true},
		"or with an unknown operand is undecided":  {resource.Or(unknown, no), false},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if got := mustMatch(t, schema, testCase.filter, record); got != testCase.want {
				t.Fatalf("matched = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestMatcherLikeDoesNotTreatPercentAsAWildcard proves the operator means the
// characters it shows. A filter whose meaning depends on which adapter runs it
// is worse than one that says less.
func TestMatcherLikeDoesNotTreatPercentAsAWildcard(t *testing.T) {
	schema := matchSchema(t)
	if mustMatch(t, schema, resource.Condition("name", resource.OpLike, "l%g"), resource.Record{"name": "ling"}) {
		t.Fatal("like treated % as a wildcard")
	}
	if !mustMatch(t, schema, resource.Condition("name", resource.OpLike, "100%"), resource.Record{"name": "100% pure"}) {
		t.Fatal("like did not match a literal percent sign")
	}
}

// TestMatcherRejectsAFilterTheSchemaCannotDescribe proves an unknown field is
// reported rather than treated as "no match". An empty result and an ignored
// condition look identical to a caller, and only one of them is true.
func TestMatcherRejectsAFilterTheSchemaCannotDescribe(t *testing.T) {
	schema := matchSchema(t)
	if _, err := resource.NewMatcher(resource.Condition("nmae", resource.OpEq, "x"), schema); codeOf(err) != "filter.unknown_field" {
		t.Fatalf("NewMatcher = %v, want filter.unknown_field", err)
	}
}

// TestMatcherReportsAValueTheFieldCannotHold proves a storage layer returning
// the wrong Go type is loud rather than silently unmatched.
func TestMatcherReportsAValueTheFieldCannotHold(t *testing.T) {
	schema := matchSchema(t)
	_, err := resource.Match(
		resource.Condition("id", resource.OpEq, 7),
		resource.Record{"id": "seven"},
		schema,
	)
	if code := codeOf(err); code != "resource.value_type_mismatch" {
		t.Fatalf("Match = %v, want resource.value_type_mismatch", err)
	}
}
