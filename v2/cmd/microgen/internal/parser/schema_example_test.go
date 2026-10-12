package parser

import "testing"

// TestSchemaExample_SlicesAndTime pins the example shapes a generated form
// pre-fills: a slice's example must be an array — an object default makes the
// form submit {} and the service rightly refuse it — while maps, byte slices,
// and timestamps follow their own JSON shapes.
func TestSchemaExample_SlicesAndTime(t *testing.T) {
	cases := map[string]string{
		"[]string":       `["roles"]`,
		"[]RoleItem":     `[{}]`,
		"[][]string":     `[["roles"]]`,
		"*[]string":      `["roles"]`,
		"map[string]int": `{}`,
		"time.Time":      `"2026-01-01T00:00:00Z"`,
		"[]byte":         `""`,
		"string":         `"roles"`,
		"int":            "1",
	}
	for goType, want := range cases {
		if got := schemaExample(goType, "roles"); got != want {
			t.Errorf("schemaExample(%q) = %s, want %s", goType, got, want)
		}
	}
}
