package generator

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// TestQuoteGoStringSurvivesHostileText covers the failure the old escape helper
// let through. It replaced only the double quote, and no template used it, so a
// doc comment or a database column comment containing a quote produced
// `Description: "a "user" record"` — valid Go syntax, which is why the
// generator's own format check passed it through to fail in the user's build.
func TestQuoteGoStringSurvivesHostileText(t *testing.T) {
	for name, input := range map[string]string{
		"double quote":    `Creates a "user" record.`,
		"backslash":       `Path is C:\temp\x`,
		"trailing escape": `ends with a backslash \`,
		"newline":         "first line\nsecond line",
		"carriage return": "first\rsecond",
		"backtick":        "uses `code` markup",
		"control byte":    "bell\x07here",
	} {
		t.Run(name, func(t *testing.T) {
			source := "package p\n\nvar s = " + quoteGoString(input) + "\n"
			if _, err := parser.ParseFile(token.NewFileSet(), "x.go", source, 0); err != nil {
				t.Fatalf("generated source does not parse: %v\n%s", err, source)
			}
			// Valid syntax is not enough — the literal has to still mean the
			// input, or the generated documentation quietly says something else.
			got, err := strconv.Unquote(quoteGoString(input))
			if err != nil {
				t.Fatalf("the literal does not unquote: %v", err)
			}
			if got != input {
				t.Fatalf("round trip changed the text: %q -> %q", input, got)
			}

		})
	}
}

func TestOneLineCommentFoldsEveryTerminator(t *testing.T) {
	for name, input := range map[string]string{
		"lf":   "first\nsecond",
		"crlf": "first\r\nsecond",
		"cr":   "first\rsecond",
	} {
		t.Run(name, func(t *testing.T) {
			got := oneLineComment(input)
			if strings.ContainsAny(got, "\r\n") {
				t.Fatalf("comment still spans lines: %q", got)
			}
			if got != "first second" {
				t.Fatalf("comment = %q, want %q", got, "first second")
			}
		})
	}
}

// TestStructTagValueCannotTerminateTheRawString is the model.tmpl case: the tag
// is a back-quoted literal, and neither a backtick nor a quote can be escaped
// inside one.
func TestStructTagValueCannotTerminateTheRawString(t *testing.T) {
	got := structTagValue("we`ird\"name\nhere")
	if strings.ContainsAny(got, "`\"\r\n") {
		t.Fatalf("tag value can still break its literal: %q", got)
	}

	source := "package p\n\ntype T struct {\n\tF string `json:\"" + got + "\"`\n}\n"
	if _, err := parser.ParseFile(token.NewFileSet(), "x.go", source, 0); err != nil {
		t.Fatalf("generated struct does not parse: %v\n%s", err, source)
	}

}

// TestTemplateSetRegistersTheEscapingHelpers guards against the previous state,
// where the escaping machinery existed and nothing referenced it.
func TestTemplateSetRegistersTheEscapingHelpers(t *testing.T) {
	set := newTemplateSet()
	for _, name := range []string{"quote", "comment", "tag"} {
		if _, err := set.New(name + "-probe").Parse("{{" + name + ` "x"}}`); err != nil {
			t.Fatalf("template helper %q is not registered: %v", name, err)
		}
	}
}
