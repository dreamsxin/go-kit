package generator

import (
	"testing"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/ir"
)

func TestBuildTypeScriptSDKData(t *testing.T) {
	user := &ir.Message{Name: "User", Fields: []*ir.Field{
		{Name: "ID", JSONName: "id", GoType: "uint64", SchemaType: "integer", Required: true},
	}}
	request := &ir.Message{Name: "GetUserRequest", Fields: []*ir.Field{
		{Name: "ID", JSONName: "id", GoType: "uint64", SchemaType: "integer", Required: true},
		{Name: "Tags", JSONName: "tag", GoType: "[]string", SchemaType: "array"},
		{Name: "Timeout", JSONName: "timeout", GoType: "time.Duration", SchemaType: "integer"},
	}}
	response := &ir.Message{Name: "GetUserResponse", Fields: []*ir.Field{
		{Name: "User", JSONName: "user", GoType: "*User", SchemaType: "object"},
	}}
	project := &ir.Project{
		Messages: []*ir.Message{user, request, response},
		Services: []*ir.Service{{
			Name: "UserService",
			Methods: []*ir.Method{
				{
					Name:       "GetUser",
					HTTPMethod: "GET",
					Route:      "/users/{id}",
					InputName:  request.Name,
					OutputName: response.Name,
					Input:      request,
					Output:     response,
				},
				{
					Name:       "UpdateUser",
					HTTPMethod: "PUT",
					Route:      "/users/{id}",
					InputName:  request.Name,
					OutputName: response.Name,
					Input:      request,
					Output:     response,
				},
			},
		}},
	}

	data := buildTypeScriptSDKData(project, "/api/v1")
	if data.CompilerVersion != TypeScriptCompilerVersion {
		t.Fatalf("compiler version = %q, want %q", data.CompilerVersion, TypeScriptCompilerVersion)
	}
	if len(data.Services) != 1 || data.Services[0].PropertyName != "userService" {
		t.Fatalf("services = %#v", data.Services)
	}
	method := data.Services[0].Methods[0]
	if method.Name != "getUser" || method.Route != "/api/v1/userservice/users/{id}" {
		t.Fatalf("method = %#v", method)
	}
	if len(method.PathFields) != 1 || method.PathFields[0].Name != "id" {
		t.Fatalf("path fields = %#v", method.PathFields)
	}
	if len(method.QueryFields) != 2 || !method.QueryFields[1].Duration {
		t.Fatalf("query fields = %#v", method.QueryFields)
	}
	update := data.Services[0].Methods[1]
	if !update.HasBody || len(update.PathFields) != 1 || len(update.QueryFields) != 0 {
		t.Fatalf("update method = %#v", update)
	}

	var responseView typeScriptMessage
	for _, message := range data.Messages {
		if message.Name == response.Name {
			responseView = message
			break
		}
	}
	if len(responseView.Fields) != 1 || responseView.Fields[0].Type != "User" {
		t.Fatalf("response view = %#v", responseView)
	}
}

func TestTypeScriptType(t *testing.T) {
	names := map[string]struct{}{"User": {}}
	cases := map[string]string{
		"[]*User":          "Array<User>",
		"map[string]*User": "Record<string, User>",
		"[]byte":           "string",
		"time.Time":        "string",
		"time.Duration":    "number",
		"interface{}":      "unknown",
	}
	for input, want := range cases {
		if got := typeScriptType(input, "", names); got != want {
			t.Errorf("typeScriptType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLowerFirstPreservesInitialism(t *testing.T) {
	if got := lowerFirst("URLService"); got != "urlService" {
		t.Fatalf("lowerFirst(URLService) = %q", got)
	}
}
