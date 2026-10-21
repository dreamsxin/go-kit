package resource_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/resource"
)

// mustParse parses the client dialect and returns the canonical form, which is
// what a test can compare without depending on the tree's Go shape.
func mustParse(t *testing.T, raw any) string {
	t.Helper()
	filter, err := resource.ParseFilter(raw)
	if err != nil {
		t.Fatalf("ParseFilter(%v): %v", raw, err)
	}
	return mustCanonical(t, filter)
}

// TestParseFilterSpeaksTheDocumentedDialect pins one spelling per condition.
// The table is the dialect's specification: a client writes the left column and a
// reviewer reads the right one in an audit log.
func TestParseFilterSpeaksTheDocumentedDialect(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "a literal means equality",
			input: `{"status":"active"}`,
			want:  `{"op":"eq","field":"status","value":"active"}`,
		},
		{
			name:  "the explicit spelling of the same condition",
			input: `{"status":{"$eq":"active"}}`,
			want:  `{"op":"eq","field":"status","value":"active"}`,
		},
		{
			name:  "two operators on one field are a conjunction",
			input: `{"age":{"$gte":18,"$lt":65}}`,
			want:  `{"op":"and","children":[{"op":"gte","field":"age","value":18},{"op":"lt","field":"age","value":65}]}`,
		},
		{
			name:  "or",
			input: `{"$or":[{"status":"draft"},{"published":true}]}`,
			want:  `{"op":"or","children":[{"op":"eq","field":"status","value":"draft"},{"op":"eq","field":"published","value":true}]}`,
		},
		{
			name:  "and beside a bare condition",
			input: `{"published":true,"$and":[{"ownerId":7},{"status":"active"}]}`,
			want:  `{"op":"and","children":[{"op":"eq","field":"ownerId","value":7},{"op":"eq","field":"status","value":"active"},{"op":"eq","field":"published","value":true}]}`,
		},
		{
			name:  "not",
			input: `{"$not":{"status":"archived"}}`,
			want:  `{"op":"not","children":[{"op":"eq","field":"status","value":"archived"}]}`,
		},
		{
			name:  "in",
			input: `{"id":{"$in":[3,4]}}`,
			want:  `{"op":"in","field":"id","value":[3,4]}`,
		},
		{
			name:  "date range",
			input: `{"publishedAt":{"$dateRange":["2026-01-01T00:00:00Z","2026-02-01T00:00:00Z"]}}`,
			want:  `{"op":"dateRange","field":"publishedAt","value":["2026-01-01T00:00:00Z","2026-02-01T00:00:00Z"]}`,
		},
		{
			name:  "empty takes no operand in the tree",
			input: `{"deletedAt":{"$empty":true}}`,
			want:  `{"op":"empty","field":"deletedAt"}`,
		},
		{
			name:  "not empty",
			input: `{"deletedAt":{"$notEmpty":true}}`,
			want:  `{"op":"notEmpty","field":"deletedAt"}`,
		},
		{
			name:  "an empty object is no condition",
			input: `{}`,
			want:  `null`,
		},
		{
			name:  "a string that carries the document, as a query parameter does",
			input: `"{\"status\":\"active\"}"`,
			want:  `{"op":"eq","field":"status","value":"active"}`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := mustParse(t, testCase.input); got != testCase.want {
				t.Fatalf("ParseFilter(%s)\n got: %s\nwant: %s", testCase.input, got, testCase.want)
			}
		})
	}
}

// TestParseFilterAcceptsEveryShapeACallerHolds proves the one entry point covers
// the several forms the same filter arrives in: a request body's RawMessage, a
// query parameter's string, a decoded tool call's map, and nothing at all.
func TestParseFilterAcceptsEveryShapeACallerHolds(t *testing.T) {
	want := `{"op":"eq","field":"status","value":"active"}`
	cases := map[string]any{
		"raw message":  json.RawMessage(`{"status":"active"}`),
		"quoted text":  `{"status":"active"}`,
		"decoded map":  map[string]any{"status": "active"},
		"double coded": json.RawMessage(`"{\"status\":\"active\"}"`),
		"empty string": "",
	}
	for name, raw := range cases {
		got := mustParse(t, raw)
		if name == "empty string" {
			if got != "null" {
				t.Errorf("%s parsed to %s, want no condition", name, got)
			}
			continue
		}
		if got != want {
			t.Errorf("%s parsed to %s, want %s", name, got, want)
		}
	}
	if got := mustParse(t, nil); got != "null" {
		t.Fatalf("nil parsed to %s, want no condition", got)
	}
}

// TestParseFilterIsDeterministic proves one document has one tree.
//
// Go randomizes map iteration, so a parser that walked the map directly would
// produce the same condition with its operands in a different order on each call.
// Two audit records for the same query would then differ, and a diff of the log
// could not answer whether the query changed.
func TestParseFilterIsDeterministic(t *testing.T) {
	document := `{"z":1,"y":2,"x":{"$gte":3,"$lt":9},"w":"four","v":true,"u":{"$in":[5,6]},"t":7,"s":8}`
	first := mustParse(t, document)
	for i := 0; i < 100; i++ {
		if got := mustParse(t, document); got != first {
			t.Fatalf("parse %d produced a different tree:\n first: %s\ngot: %s", i, first, got)
		}
	}
}

// TestParseFilterKeepsWideIdentifiersExact proves a value is not rounded on the
// way in. 9007199254740993 is the smallest integer a float64 cannot hold, so a
// decoder that used float64 would silently return the neighbouring record.
func TestParseFilterKeepsWideIdentifiersExact(t *testing.T) {
	got := mustParse(t, json.RawMessage(`{"id":9007199254740993}`))
	want := `{"op":"eq","field":"id","value":9007199254740993}`
	if got != want {
		t.Fatalf("wide identifier parsed to %s, want %s", got, want)
	}
}

// TestParseFilterRejectsWhatItCannotMean covers the shapes a client gets wrong.
// Each one is refused with a code: reading a mistyped operator as equality or as
// "no condition" would answer a different question than the one asked.
func TestParseFilterRejectsWhatItCannotMean(t *testing.T) {
	cases := map[string]struct {
		input string
		code  string
	}{
		"a key inside a field that is not an operator": {`{"age":{"value":18}}`, "filter.expected_operator"},
		"a connective inside a field":                  {`{"age":{"$and":[{"$gt":1}]}}`, "filter.logical_not_allowed_here"},
		"an unknown operator":                          {`{"age":{"$between":[1,2]}}`, "filter.unknown_operator"},
		"an unknown top-level operator":                {`{"$nor":[{"a":1}]}`, "filter.unknown_operator"},
		"a connective whose operand is not a list":     {`{"$and":{"a":1}}`, "filter.logical_requires_list"},
		"an empty connective":                          {`{"$and":[]}`, "filter.shape"},
		"a connective over nothing":                    {`{"$and":[{}]}`, "filter.shape"},
		"not over two conditions":                      {`{"$not":[{"a":1},{"b":2}]}`, "filter.shape"},
		"not over nothing":                             {`{"$not":{}}`, "filter.shape"},
		"empty takes no false":                         {`{"a":{"$empty":false}}`, "filter.shape"},
		"an empty in list":                             {`{"a":{"$in":[]}}`, "filter.shape"},
		"text that is not JSON":                        {`status=active`, "filter.malformed"},
		"two documents in one string":                  {`{"a":1}{"b":2}`, "filter.malformed"},
		"a number at the top":                          {`3`, "filter.not_an_object"},
		"a list at the top":                            {`[{"a":1}]`, "filter.not_an_object"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			filter, err := resource.ParseFilter(testCase.input)
			if err == nil {
				t.Fatalf("%s was accepted as %s", testCase.input, mustCanonical(t, filter))
			}
			if code := codeOf(err); code != testCase.code {
				t.Fatalf("%s failed with %q (%v), want %q", testCase.input, code, err, testCase.code)
			}
		})
	}
}

// TestParseFilterLimits prove the two bounds a hostile request meets: nesting and
// size. Both are refused before a repository walks the tree.
func TestParseFilterLimits(t *testing.T) {
	deep := `{"a":1}`
	for i := 0; i < 40; i++ {
		deep = fmt.Sprintf(`{"$not":%s}`, deep)
	}
	if _, err := resource.ParseFilter(deep); codeOf(err) != "filter.too_deep" {
		t.Fatalf("40 levels parsed: %v", err)
	}

	wide := make([]string, 0, 300)
	for i := 0; i < 300; i++ {
		wide = append(wide, fmt.Sprintf(`"f%d":%d`, i, i))
	}
	if _, err := resource.ParseFilter("{" + strings.Join(wide, ",") + "}"); codeOf(err) != "filter.too_many_conditions" {
		t.Fatalf("300 conditions parsed: %v", err)
	}
}
