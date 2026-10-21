package resource_test

import (
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/resource"
)

func mergeSchema(t *testing.T) resource.Schema {
	t.Helper()
	return mustSchema(t,
		resource.NewField("id", resource.TypeInt),
		resource.NewField("owner", resource.TypeString),
		resource.NewField("status", resource.TypeString),
		resource.NewField("email", resource.TypeString),
		resource.NewField("shared", resource.TypeBool),
	)
}

// matchesAll reports which records a filter selects, which is the only proof
// that a merge narrowed rather than replaced.
func matchesAll(t *testing.T, filter resource.Filter, schema resource.Schema, records []resource.Record) []bool {
	t.Helper()
	matcher, err := resource.NewMatcher(filter, schema)
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}
	results := make([]bool, len(records))
	for i, record := range records {
		matched, err := matcher.Match(record)
		if err != nil {
			t.Fatalf("Match(%v): %v", record, err)
		}
		results[i] = matched
	}
	return results
}

// TestMergeParamsKeepsEveryFilterLayer is the central security property: three
// layers intersect, so a client condition and a policy condition on the same
// question both survive. The failure it rules out is "the later layer replaces
// the earlier one", which reads as a successful query and returns records the
// policy meant to hide.
func TestMergeParamsKeepsEveryFilterLayer(t *testing.T) {
	schema := mergeSchema(t)
	policy := resource.Params{Filter: resource.Condition("owner", resource.OpEq, "ling")}
	client := resource.Params{Filter: resource.Condition("status", resource.OpEq, "active")}

	merged, err := resource.MergeParams(resource.Params{}, policy, client)
	if err != nil {
		t.Fatalf("MergeParams: %v", err)
	}

	records := []resource.Record{
		{"owner": "ling", "status": "active"},
		{"owner": "ling", "status": "archived"},
		{"owner": "wei", "status": "active"},
	}
	got := matchesAll(t, merged.Filter, schema, records)
	want := []bool{true, false, false}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("record %d matched = %v, want %v (records: %v)", i, got[i], want[i], records)
		}
	}
}

// TestMergeParamsCannotBeWidenedByADisjunction proves a client cannot escape a
// policy by wrapping its own condition in $or: the disjunction stays one operand
// of the conjunction the policy also constrains.
func TestMergeParamsCannotBeWidenedByADisjunction(t *testing.T) {
	schema := mergeSchema(t)
	policy := resource.Params{Filter: resource.Condition("owner", resource.OpEq, "ling")}
	clientFilter, err := resource.ParseFilter(map[string]any{
		"$or": []any{
			map[string]any{"owner": "wei"},
			map[string]any{"shared": true},
		},
	})
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}

	merged, err := resource.MergeParams(policy, resource.Params{Filter: clientFilter})
	if err != nil {
		t.Fatalf("MergeParams: %v", err)
	}

	records := []resource.Record{
		{"owner": "ling", "shared": true},
		{"owner": "wei", "shared": false},
		{"owner": "wei", "shared": true},
	}
	if got, want := matchesAll(t, merged.Filter, schema, records), []bool{true, false, false}; !equalBools(got, want) {
		t.Fatalf("matched = %v, want %v", got, want)
	}
}

// TestMergeParamsIntersectsProjections proves a client can ask for fewer columns
// than the policy allows and never for more.
func TestMergeParamsIntersectsProjections(t *testing.T) {
	policy := resource.Params{Fields: []string{"id", "status"}}
	client := resource.Params{Fields: []string{"id", "email"}}

	merged, err := resource.MergeParams(policy, client)
	if err != nil {
		t.Fatalf("MergeParams: %v", err)
	}
	assertFields(t, "merged projection", merged.Fields, []string{"id"})

	// A layer with no opinion leaves the other's whitelist intact.
	merged, err = resource.MergeParams(policy, resource.Params{})
	if err != nil {
		t.Fatalf("MergeParams: %v", err)
	}
	assertFields(t, "policy only", merged.Fields, []string{"id", "status"})
}

// TestMergeParamsRejectsDisjointProjections proves the empty intersection is a
// denial. The failure it rules out is treating "no common column" as "no
// restriction", which would answer the request with every column instead of
// none.
func TestMergeParamsRejectsDisjointProjections(t *testing.T) {
	policy := resource.Params{Fields: []string{"status"}}
	client := resource.Params{Fields: []string{"email"}}

	merged, err := resource.MergeParams(policy, client)
	if code := codeOf(err); code != "resource.fields_disjoint" {
		t.Fatalf("MergeParams = %v, want resource.fields_disjoint", err)
	}
	if merged.Fields != nil {
		t.Fatalf("a rejected merge returned fields %v; a caller ignoring the error must not get an unrestricted query", merged.Fields)
	}
}

// TestMergeParamsLowersThePageSizeOnly proves the window merge treats size as a
// cap and the page number as a position: a client cannot raise the cap, and a
// policy does not reset the page a client asked for.
func TestMergeParamsLowersThePageSizeOnly(t *testing.T) {
	cases := map[string]struct {
		base  resource.Params
		layer resource.Params
		want  resource.Window
	}{
		"a client cannot raise the policy cap": {
			base:  resource.Params{Window: resource.Window{Number: 3, Size: resource.MaxPageSize}},
			layer: resource.Params{Window: resource.Window{Size: 20}},
			want:  resource.Window{Number: 3, Size: 20},
		},
		"a smaller client window stands": {
			base:  resource.Params{Window: resource.Window{Number: 2, Size: 5}},
			layer: resource.Params{Window: resource.Window{Size: 20}},
			want:  resource.Window{Number: 2, Size: 5},
		},
		"a policy that states no page keeps the client's": {
			base:  resource.Params{Window: resource.Window{Number: 7, Size: 10}},
			layer: resource.Params{Window: resource.Window{}},
			want:  resource.Window{Number: 7, Size: 10},
		},
		"unset becomes the default": {
			base:  resource.Params{},
			layer: resource.Params{},
			want:  resource.Window{Number: 1, Size: 20},
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			merged, err := resource.MergeParams(testCase.base, testCase.layer)
			if err != nil {
				t.Fatalf("MergeParams: %v", err)
			}
			if got := merged.Query().Window; got != testCase.want {
				t.Fatalf("window = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// TestMergeParamsRejectsAnOversizedWindowFromAnyLayer proves the bounds hold per
// layer: a policy that asks for a thousand rows is a bug, and clamping it
// silently would ship the bug.
func TestMergeParamsRejectsAnOversizedWindowFromAnyLayer(t *testing.T) {
	_, err := resource.MergeParams(
		resource.Params{Window: resource.Window{Size: 10}},
		resource.Params{Window: resource.Window{Size: resource.MaxPageSize + 1}},
	)
	if code := codeOf(err); code != "resource.page_size_invalid" {
		t.Fatalf("MergeParams = %v, want resource.page_size_invalid", err)
	}
}

// TestMergeWriteMasksIntersectsAllowAndUnionsDeny proves the two directions of
// the write mask. Intersecting the deny lists instead would let a second layer
// re-permit a field the first one forbade.
func TestMergeWriteMasksIntersectsAllowAndUnionsDeny(t *testing.T) {
	first := resource.WriteMask{Allow: []string{"name", "role"}, Deny: []string{"role"}}
	second := resource.WriteMask{Allow: []string{"name", "email"}, Deny: []string{"email"}}

	merged := resource.MergeWriteMasks(first, second)
	assertFields(t, "merged whitelist", merged.Allow, []string{"name"})
	assertFields(t, "merged blacklist", merged.Deny, []string{"role", "email"})
	for _, field := range []string{"role", "email"} {
		if merged.Allows(field) {
			t.Fatalf("%q is writable after a layer denied it", field)
		}
	}
	if !merged.Allows("name") {
		t.Fatal("name is not writable, though every layer allowed it")
	}
}

// TestWriteMaskDistinguishesNoWhitelistFromAnEmptyOne pins the nil-versus-empty
// convention the whole merge rests on. It fails closed: the ambiguous spelling
// (an empty literal) denies everything rather than permitting everything.
func TestWriteMaskDistinguishesNoWhitelistFromAnEmptyOne(t *testing.T) {
	var unrestricted resource.WriteMask
	if !unrestricted.Allows("anything") {
		t.Fatal("the zero mask restricts a field")
	}
	empty := resource.WriteMask{Allow: []string{}}
	if empty.Allows("anything") {
		t.Fatal("an empty whitelist permitted a field")
	}
	// The case that matters: two disjoint whitelists must not collapse into an
	// unrestricted mask.
	disjoint := resource.MergeWriteMasks(
		resource.WriteMask{Allow: []string{"role"}},
		resource.WriteMask{Allow: []string{"email"}},
	)
	if disjoint.Allow == nil {
		t.Fatal("two disjoint whitelists merged to nil, which permits every field")
	}
	if disjoint.Allows("role") || disjoint.Allows("email") {
		t.Fatal("a disjoint merge permitted a field")
	}
}

// TestWriteMaskCheckNamesTheField proves a refused write names what was refused.
// A mask that silently dropped the field would answer 200 to a create that
// stored something else.
func TestWriteMaskCheckNamesTheField(t *testing.T) {
	mask := resource.WriteMask{Allow: []string{"name"}}
	err := mask.Check(resource.Record{"role": "admin", "name": "ling"})
	if code := codeOf(err); code != "resource.field_not_writable" {
		t.Fatalf("Check = %v, want resource.field_not_writable", err)
	}
	if !strings.Contains(err.Error(), "role") {
		t.Fatalf("Check = %v, does not name the refused field", err)
	}
}

// TestParamsValidateRejectsAMaskNamingNoField proves a typo in a deny list is
// reported. It is the difference between a permission and the belief in one.
func TestParamsValidateRejectsAMaskNamingNoField(t *testing.T) {
	schema := mergeSchema(t)
	params := resource.Params{Writable: resource.WriteMask{Deny: []string{"roles"}}}
	if code := codeOf(params.Validate(schema)); code != "resource.unknown_field" {
		t.Fatalf("Validate = %v, want resource.unknown_field", params.Validate(schema))
	}
	params = resource.Params{Writable: resource.WriteMask{Deny: []string{"email"}}}
	if err := params.Validate(schema); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func assertFields(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

func equalBools(left, right []bool) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
