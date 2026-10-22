package resource_test

// Fuzz targets for the two parsers that read text a client wrote: the filter
// dialect, and the canonical form a stored condition is replayed from.
//
// Both are reachable from an HTTP query parameter and from a persisted audit
// record, so both are fed bytes nobody vouched for. The seeds are the documented
// examples, which means plain `go test` replays the dialect this package
// promises before a fuzzer ever starts — and every crash a `-fuzz` run finds is
// written to testdata/fuzz/<Target>/, where `go test` replays it from then on.
//
// The assertions are contracts rather than implementation details: a refusal is
// classified and named filter.*, an accepted tree is inside the limits the
// refusal messages state, one document parses to one tree, and the canonical
// form is a fixed point. A parser can be rewritten under all four.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/resource"
)

// The limits the parser's own refusals promise: "a filter nests at most 32
// levels" and "carries at most 256 conditions". They are written out here rather
// than read from the package because they are what a client can rely on; a
// change to either has to be a change to the promise, and this fails until it is.
const (
	documentedMaxDepth = 32
	documentedMaxNodes = 256
)

func FuzzParseFilter(f *testing.F) {
	seeds := []string{
		``,
		`{}`,
		`null`,
		`{"status":"active"}`,
		`{"status":{"$eq":"active"},"age":{"$gte":18,"$lt":65}}`,
		`{"$or":[{"ownerId":7},{"$and":[{"shared":true},{"status":"active"}]}]}`,
		`{"id":{"$in":[3,4]},"publishedAt":{"$dateRange":["2026-01-01T00:00:00Z","2026-02-01T00:00:00Z"]}}`,
		`{"name":{"$like":"a%"}}`,
		`{"deletedAt":{"$empty":true}}`,
		`{"$not":{"status":"archived"}}`,
		`{"$not":[{"status":"archived"}]}`,
		`{"id":{"$in":[]}}`,
		`{"age":{"$value":18}}`,
		`{"$in":[1]}`,
		`{"$and":[]}`,
		`"{\"status\":\"active\"}"`,
		`{"a":1}{"b":2}`,
		// On either side of each budget, so a limit that stops being enforced
		// fails under plain `go test` rather than only under a fuzzer that
		// happens to grow a document wide enough.
		wideFilter(documentedMaxNodes - 1),
		wideFilter(documentedMaxNodes + 1),
		strings.Repeat(`{"$not":`, documentedMaxDepth) + `{"a":1}` + strings.Repeat(`}`, documentedMaxDepth),
		strings.Repeat(`{"$not":`, documentedMaxDepth+2) + `{"a":1}` + strings.Repeat(`}`, documentedMaxDepth+2),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		filter, err := resource.ParseFilter(raw)
		if err != nil {
			requireFilterRefusal(t, err)
			return
		}
		// An accepted tree is inside the budget the refusals advertise. Measure
		// it independently: the parser counting itself is what the budget is.
		depth, nodes := measureFilter(filter)
		if nodes > documentedMaxNodes {
			t.Fatalf("ParseFilter(%q) returned %d conditions, over the %d it promises", raw, nodes, documentedMaxNodes)
		}
		if depth > documentedMaxDepth {
			t.Fatalf("ParseFilter(%q) returned a tree %d levels deep, over the %d it promises", raw, depth, documentedMaxDepth)
		}

		canonical := mustMarshalFilter(t, filter)
		// One document, one tree: two audits of the same request have to be
		// comparable, and map iteration is the thing that would break that.
		again, err := resource.ParseFilter(raw)
		if err != nil {
			t.Fatalf("ParseFilter(%q) succeeded and then failed on the same input: %v", raw, err)
		}
		if got := mustMarshalFilter(t, again); !bytes.Equal(canonical, got) {
			t.Fatalf("ParseFilter(%q) is not deterministic: %s then %s", raw, canonical, got)
		}

		// The canonical form is what a log stores and a bug report replays, so
		// it has to read back into the same tree.
		var decoded resource.Filter
		if err := decoded.UnmarshalJSON(canonical); err != nil {
			t.Fatalf("the canonical form of ParseFilter(%q) does not read back: %v\n%s", raw, err, canonical)
		}
		if got := mustMarshalFilter(t, decoded); !bytes.Equal(canonical, got) {
			t.Fatalf("the canonical form of ParseFilter(%q) is not a fixed point: %s then %s", raw, canonical, got)
		}
	})
}

func FuzzFilterCanonicalForm(f *testing.F) {
	seeds := []string{
		`null`,
		`{}`,
		`{"op":"eq","field":"status","value":"active"}`,
		`{"op":"in","field":"id","value":[3,4]}`,
		`{"op":"empty","field":"deletedAt"}`,
		`{"op":"and","children":[{"op":"eq","field":"a","value":1},{"op":"not","children":[{"op":"eq","field":"b","value":true}]}]}`,
		`{"op":"and","children":[]}`,
		`{"op":"not","children":[]}`,
		`{"op":"eq","field":"a","value":1,"extra":2}`,
		`{"op":"nope","field":"a"}`,
		`{"op":"eq"}`,
		`{"op":"dateRange","field":"t","value":["2026-01-01T00:00:00Z"]}`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var filter resource.Filter
		if err := filter.UnmarshalJSON(data); err != nil {
			requireFilterRefusal(t, err)
			return
		}
		canonical := mustMarshalFilter(t, filter)
		var again resource.Filter
		if err := again.UnmarshalJSON(canonical); err != nil {
			t.Fatalf("a filter that decoded does not decode its own canonical form: %v\n%s", err, canonical)
		}
		if got := mustMarshalFilter(t, again); !bytes.Equal(canonical, got) {
			t.Fatalf("canonical form is not a fixed point: %s then %s", canonical, got)
		}
	})
}

// wideFilter builds one object with count distinct conditions, which is the shape
// a budget counted in objects has to still refuse: the parser counts objects, and
// one object can carry a thousand conditions.
func wideFilter(count int) string {
	conditions := make([]string, 0, count)
	for i := 0; i < count; i++ {
		conditions = append(conditions, fmt.Sprintf(`"f%d":%d`, i, i))
	}
	return "{" + strings.Join(conditions, ",") + "}"
}

// requireFilterRefusal pins the classification both parsers promise: a document
// they cannot mean is an invalid argument with a code in the filter namespace,
// and not a bare error a transport would answer 500 to.
func requireFilterRefusal(t *testing.T, err error) {
	t.Helper()
	var classified *apperror.Error
	if !errors.As(err, &classified) {
		t.Fatalf("refusal %v (%T) is not a classified application error", err, err)
	}
	if kind := classified.ErrorKind(); kind != apperror.KindInvalidArgument {
		t.Fatalf("refusal %v has kind %q, want %q", err, kind, apperror.KindInvalidArgument)
	}
	if code := classified.ErrorCode(); !strings.HasPrefix(code, "filter.") {
		t.Fatalf("refusal %v has code %q, want a filter.* code", err, code)
	}
}

// measureFilter counts a tree the way the budget is stated: nodes are conditions
// and logical operators alike, and depth is how many of them sit one inside
// another.
func measureFilter(filter resource.Filter) (depth, nodes int) {
	if filter.IsZero() {
		return 0, 0
	}
	nodes = 1
	for _, child := range filter.Children {
		childDepth, childNodes := measureFilter(child)
		nodes += childNodes
		if childDepth+1 > depth {
			depth = childDepth + 1
		}
	}
	return depth, nodes
}

func mustMarshalFilter(t *testing.T, filter resource.Filter) []byte {
	t.Helper()
	encoded, err := json.Marshal(filter)
	if err != nil {
		t.Fatalf("a filter that parsed does not marshal: %v", err)
	}
	return encoded
}
