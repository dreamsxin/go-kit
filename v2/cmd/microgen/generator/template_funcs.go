package generator

import (
	"encoding/json"
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
		"escape": func(s string) string {
			return strings.ReplaceAll(s, "\"", "\\\"")
		},
	})
}
