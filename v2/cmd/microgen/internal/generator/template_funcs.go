package generator

import (
	"encoding/json"
	"strconv"
	"strings"
	"text/template"
)

func newTemplateSet() *template.Template {
	return template.New("microgen").Funcs(template.FuncMap{
		"lower":     func(s string) string { return strings.ToLower(s) },
		"upper":     func(s string) string { return strings.ToUpper(s) },
		"title":     func(s string) string { return strings.Title(s) }, //nolint:staticcheck
		"snake":     toSnakeCase,
		"trimStar":  func(s string) string { return strings.TrimPrefix(s, "*") },
		"hasPrefix": strings.HasPrefix,
		"marshal": func(v any) string {
			a, _ := json.Marshal(v)
			return string(a)
		},
		"quote":   quoteGoString,
		"comment": oneLineComment,
		"tag":     structTagValue,
	})
}

// quoteGoString renders s as a complete Go string literal, quotes included.
//
// Templates must use it instead of writing "{{.Field}}" by hand. Doc comments and
// database column comments reach these templates verbatim, and a comment
// containing a double quote produced `Description: "a "user" record"` — which is
// syntactically valid Go, so it passed the generator's format check, was written
// to disk, and failed in the user's build. A backslash or a newline produced
// output that would not parse, aborting generation after earlier files had
// already been written.
//
// strconv.Quote handles quotes, backslashes, newlines and control characters
// together, which is the whole point of using it rather than replacing one
// character.
func quoteGoString(s string) string { return strconv.Quote(s) }

// oneLineComment folds s onto a single line for use after a // marker.
//
// A line comment ends at the newline, so a multi-line database column comment
// turned every line after the first into stray tokens in the middle of a struct
// body or a .proto message. Non-Go output never reaches a formatter, so nothing
// downstream would have caught it.
func oneLineComment(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\r", "\n")), " ")
}

// structTagValue renders s for use inside a back-quoted struct tag.
//
// The tag is a raw string literal, so a backtick in a column name or a database
// type terminates it and the generated file no longer parses. A quote breaks the
// tag's own key:"value" grammar. Neither can be escaped inside a raw string, so
// they are removed: a tag is a wire name, and a wire name containing a backtick
// is not a name this generator can carry.
func structTagValue(s string) string {
	return strings.NewReplacer("`", "", `"`, "").Replace(oneLineComment(s))
}
