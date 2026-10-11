package parser

import (
	"strings"
	"testing"
)

// TestParseAuthDoc covers the auth: doc-comment grammar and the refusals
// that keep a broken declaration from being silently ignored.
func TestParseAuthDoc(t *testing.T) {
	cases := []struct {
		name     string
		doc      string
		required bool
		roles    []string
		wantErr  string
	}{
		{name: "no auth lines", doc: "// Creates a user.", wantErr: ""},
		{name: "required", doc: "// Creates.\n// auth: required", required: true},
		{name: "roles only", doc: "// Creates.\n// auth: roles=admin,operator", roles: []string{"admin", "operator"}},
		{name: "combined one line", doc: "// auth: required roles=admin", required: true, roles: []string{"admin"}},
		{name: "multiple auth lines", doc: "// auth: required\n// auth: roles=admin", required: true, roles: []string{"admin"}},
		{name: "unknown directive", doc: "// auth: sso", wantErr: `unknown directive "sso"`},
		{name: "empty value", doc: "// auth:", wantErr: "directive has no value"},
		{name: "empty roles", doc: "// auth: roles=", wantErr: "roles lists no values"},
		{name: "empty role in list", doc: "// auth: roles=admin,,operator", wantErr: "empty role"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			auth, cleaned, err := parseAuthDoc(tc.doc)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseAuthDoc(%q) error = %v, want substring %q", tc.doc, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAuthDoc(%q): %v", tc.doc, err)
			}
			if auth.Required != tc.required {
				t.Errorf("required = %v, want %v", auth.Required, tc.required)
			}
			if strings.Join(auth.Roles, ",") != strings.Join(tc.roles, ",") {
				t.Errorf("roles = %v, want %v", auth.Roles, tc.roles)
			}
			// The directive lines never reach the public description.
			if strings.Contains(cleaned, "auth:") {
				t.Errorf("cleaned doc still carries the directive: %q", cleaned)
			}
			if tc.name == "required" && !strings.Contains(cleaned, "Creates.") {
				t.Errorf("cleaned doc lost the prose: %q", cleaned)
			}
		})
	}
}
