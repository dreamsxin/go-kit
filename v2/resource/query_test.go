package resource_test

import (
	"encoding/json"
	"math"
	"sort"
	"testing"

	"github.com/dreamsxin/go-kit/v2/resource"
)

// TestWindowDefaultsAndBounds proves an unset window becomes the default, and a
// stated one outside the bounds is rejected rather than clamped. A client that
// asked for a thousand rows has a bug, and answering with a hundred teaches it
// nothing.
func TestWindowDefaultsAndBounds(t *testing.T) {
	if got := (resource.Window{}).Normalized(); got != (resource.Window{Number: 1, Size: 20}) {
		t.Fatalf("zero window normalized to %+v", got)
	}
	if got := (resource.Window{Number: 3}).Normalized(); got != (resource.Window{Number: 3, Size: 20}) {
		t.Fatalf("window without a size normalized to %+v", got)
	}
	if got := (resource.Window{Number: 3, Size: 20}).Offset(); got != 40 {
		t.Fatalf("offset = %d, want 40", got)
	}
	if code := codeOf((resource.Window{Number: -1}).Validate()); code != "resource.page_invalid" {
		t.Fatalf("negative page = %v, want resource.page_invalid", code)
	}
	oversized := resource.Window{Size: resource.MaxPageSize + 1}
	if code := codeOf(oversized.Validate()); code != "resource.page_size_invalid" {
		t.Fatalf("oversized window = %v, want resource.page_size_invalid", code)
	}
}

// TestPageSerializesTheSharedWireShape pins the JSON a page becomes. It is the
// same shape the HTTP transport's PageResult writes, and a second pagination
// contract would be one more thing for a client to learn and one more place for
// the two to drift apart.
func TestPageSerializesTheSharedWireShape(t *testing.T) {
	page := resource.NewPage(resource.Window{Number: 2, Size: 10}, 25, nil)

	if page.Number != 2 || page.Size != 10 || page.Total != 25 {
		t.Fatalf("page = %+v, want page 2 size 10 total 25", page)
	}
	if !page.HasNext {
		t.Fatal("HasNext is false although record 21 follows the window")
	}

	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	keys := make([]string, 0, len(decoded))
	for key := range decoded {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"has_next", "items", "page", "size", "total"}
	assertFields(t, "page JSON keys", keys, want)

	items, isArray := decoded["items"].([]any)
	if !isArray {
		t.Fatalf("items decoded as %T, want an array: an empty page marshals as [] rather than null", decoded["items"])
	}
	if len(items) != 0 {
		t.Fatalf("items = %v, want an empty array", items)
	}
}

// TestPageHasNoNextOnTheLastRecord proves HasNext is derived from the total, not
// from whether the window happens to be full.
func TestPageHasNoNextOnTheLastRecord(t *testing.T) {
	page := resource.NewPage(resource.Window{Number: 2, Size: 10}, 20, nil)
	if page.HasNext {
		t.Fatal("HasNext is true although the window ends on the last record")
	}
}

// TestWindowArithmeticDoesNotOverflow proves a page number near the integer limit
// is answered with "nothing there". An overflowing offset becomes a negative
// slice index, which is a panic any client could request with one parameter.
func TestWindowArithmeticDoesNotOverflow(t *testing.T) {
	huge := resource.Window{Number: math.MaxInt, Size: resource.MaxPageSize}
	if got := huge.Offset(); got != math.MaxInt64 {
		t.Fatalf("offset = %d, want it saturated at the maximum", got)
	}
	page := resource.NewPage(huge, 10, nil)
	if page.HasNext {
		t.Fatal("HasNext is true for a window past the end of the data")
	}
}

// TestQueryValidateRejectsWhatTheResourceCannotAnswer proves a query naming a
// column that does not exist, a field that is not sortable, or no column at all
// fails before storage is touched.
func TestQueryValidateRejectsWhatTheResourceCannotAnswer(t *testing.T) {
	schema := mustSchema(t,
		resource.NewField("id", resource.TypeInt),
		resource.NewField("name", resource.TypeString),
		// Sortable is false here: a column that may be read but not ordered.
		resource.Field{Name: "note", Type: resource.TypeString, Readable: true},
	)
	cases := map[string]struct {
		query resource.Query
		want  string
	}{
		"an unknown sort field": {
			query: resource.Query{Sort: []resource.Sort{resource.Asc("nmae")}},
			want:  "resource.unknown_field",
		},
		"a field that cannot be sorted": {
			query: resource.Query{Sort: []resource.Sort{resource.Desc("note")}},
			want:  "resource.field_not_sortable",
		},
		"a projection of nothing": {
			query: resource.Query{Fields: []string{}},
			want:  "resource.fields_empty",
		},
		"an unknown projection field": {
			query: resource.Query{Fields: []string{"email"}},
			want:  "resource.unknown_field",
		},
		"a window past the limit": {
			query: resource.Query{Window: resource.Window{Size: resource.MaxPageSize + 1}},
			want:  "resource.page_size_invalid",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if code := codeOf(testCase.query.Validate(schema)); code != testCase.want {
				t.Fatalf("Validate = %v, want %s", code, testCase.want)
			}
		})
	}

	valid := resource.Query{
		Filter:  resource.Condition("name", resource.OpEq, "ling"),
		Sort:    []resource.Sort{resource.Asc("id")},
		Fields:  []string{"id", "name"},
		Window:  resource.Window{Number: 1, Size: 10},
		Appends: []string{"comments"},
	}
	if err := valid.Validate(schema); err != nil {
		t.Fatalf("a valid query was rejected: %v", err)
	}
}
