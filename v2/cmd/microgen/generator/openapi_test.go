package generator

import (
	"encoding/json"
	"testing"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/ir"
)

func TestBuildOpenAPIDocument(t *testing.T) {
	getRequest := &ir.Message{Name: "GetUserRequest", Fields: []*ir.Field{
		{Name: "ID", JSONName: "id", GoType: "string", SchemaType: "string", Required: true},
		{Name: "Tags", JSONName: "tag", GoType: "[]string", SchemaType: "array"},
		{Name: "Timeout", JSONName: "timeout", GoType: "time.Duration", SchemaType: "integer"},
	}}
	user := &ir.Message{Name: "User", Fields: []*ir.Field{
		{Name: "ID", JSONName: "id", GoType: "string", SchemaType: "string", Required: true},
		{Name: "CreatedAt", JSONName: "created_at", GoType: "time.Time", SchemaType: "object"},
	}}
	createRequest := &ir.Message{Name: "CreateUserRequest", Fields: []*ir.Field{
		{Name: "User", JSONName: "user", GoType: "*User", SchemaType: "object", Required: true},
	}}
	project := &ir.Project{
		PackageName: "accounts",
		Messages:    []*ir.Message{getRequest, user, createRequest},
		Services: []*ir.Service{{
			Name:        "UserService",
			Title:       "Users API",
			Description: "User operations",
			Methods: []*ir.Method{
				{Name: "GetUser", Summary: "Get user", Kind: ir.MethodKindUnary, HTTPMethod: "GET", Route: "/users/{id}", InputName: getRequest.Name, OutputName: user.Name, Input: getRequest, Output: user},
				{Name: "CreateUser", Kind: ir.MethodKindUnary, HTTPMethod: "POST", Route: "/users", InputName: createRequest.Name, OutputName: user.Name, Input: createRequest, Output: user},
				{Name: "DeleteUser", Kind: ir.MethodKindUnary, HTTPMethod: "DELETE", Route: "/users/{id}", InputName: getRequest.Name, OutputName: user.Name, Input: getRequest, Output: user},
				{Name: "WatchUsers", Kind: ir.MethodKindServerStream, HTTPMethod: "GET", Route: "/users/watch", InputName: getRequest.Name, OutputName: user.Name, Input: getRequest, Output: user},
			},
		}},
	}

	doc := buildOpenAPIDocument(project, "/api/v1")
	if doc.OpenAPI != "3.1.0" {
		t.Fatalf("openapi = %q", doc.OpenAPI)
	}
	get := doc.Paths["/api/v1/userservice/users/{id}"]["get"]
	if get.OperationID != "UserService_GetUser" || len(get.Parameters) != 3 {
		t.Fatalf("GET operation = %#v", get)
	}
	if get.Parameters[0].In != "path" || !get.Parameters[0].Required {
		t.Fatalf("path parameter = %#v", get.Parameters[0])
	}
	if get.Parameters[1].Schema.Type != "array" || get.Parameters[1].Explode == nil || !*get.Parameters[1].Explode {
		t.Fatalf("array query parameter = %#v", get.Parameters[1])
	}
	if get.Parameters[2].Schema.Type != "string" {
		t.Fatalf("duration query parameter = %#v", get.Parameters[2])
	}
	post := doc.Paths["/api/v1/userservice/users"]["post"]
	if post.RequestBody == nil || post.RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/CreateUserRequest" {
		t.Fatalf("POST request body = %#v", post.RequestBody)
	}
	deleteOperation := doc.Paths["/api/v1/userservice/users/{id}"]["delete"]
	if len(deleteOperation.Parameters) != 1 || deleteOperation.Parameters[0].In != "path" || deleteOperation.RequestBody == nil {
		t.Fatalf("DELETE operation = %#v", deleteOperation)
	}
	if _, exists := doc.Paths["/api/v1/userservice/users/watch"]; exists {
		t.Fatal("streaming method must not be emitted as an HTTP operation")
	}
	if got := doc.Components.Schemas["CreateUserRequest"].Properties["user"].Ref; got != "#/components/schemas/User" {
		t.Fatalf("user schema ref = %q", got)
	}
	if got := doc.Components.Schemas["User"].Properties["created_at"].Format; got != "date-time" {
		t.Fatalf("time format = %q", got)
	}
	if _, err := json.Marshal(doc); err != nil {
		t.Fatalf("marshal document: %v", err)
	}
}
