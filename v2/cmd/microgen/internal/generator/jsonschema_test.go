package generator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/internal/ir"
)

func TestBuildJSONSchemaDocument(t *testing.T) {
	profile := &ir.Message{Name: "Profile", Description: "Public profile", Fields: []*ir.Field{
		{Name: "DisplayName", JSONName: "display_name", GoType: "string", SchemaType: "string", Required: true},
	}}
	user := &ir.Message{Name: "User", Fields: []*ir.Field{
		{Name: "Profile", JSONName: "profile", GoType: "*Profile", SchemaType: "object", Required: true},
		{Name: "Aliases", JSONName: "aliases", GoType: "[]string", SchemaType: "array"},
	}}
	project := &ir.Project{
		PackageName: "accounts",
		Messages:    []*ir.Message{profile, user},
		Services: []*ir.Service{{
			Name:        "UserService",
			Description: "User operations",
		}},
	}

	doc := buildJSONSchemaDocument(project)
	if doc.Schema != jsonSchemaVersion {
		t.Fatalf("schema = %q", doc.Schema)
	}
	if doc.Title != "UserService API Schemas" {
		t.Fatalf("title = %q", doc.Title)
	}
	// Profile is a *Profile with no omitempty, so the field can arrive as null and
	// the reference has to be wrapped rather than named directly.
	profileSchema := doc.Defs["User"].Properties["profile"]
	if len(profileSchema.AnyOf) != 2 || profileSchema.AnyOf[0].Ref != "#/$defs/Profile" || profileSchema.AnyOf[1].Type != "null" {
		t.Fatalf("profile schema = %#v", profileSchema)
	}
	if got := doc.Defs["Profile"].Description; got != "Public profile" {
		t.Fatalf("profile description = %q", got)
	}
	if len(doc.Defs["User"].Required) != 1 || doc.Defs["User"].Required[0] != "profile" {
		t.Fatalf("user required = %#v", doc.Defs["User"].Required)
	}
	if doc.Defs["ErrorResponse"] == nil {
		t.Fatal("ErrorResponse schema is missing")
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal document: %v", err)
	}
	if strings.Contains(string(data), "#/components/schemas/") {
		t.Fatalf("JSON Schema contains OpenAPI refs: %s", data)
	}
}
