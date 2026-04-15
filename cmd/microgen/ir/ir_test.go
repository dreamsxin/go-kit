package ir_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/dbschema"
	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

func TestFromParseResult_GoIDL(t *testing.T) {
	result, err := parser.ParseFull(filepath.Join("..", "parser", "testdata", "basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}

	project := ir.FromParseResult(result)

	if project.PackageName != "basic" {
		t.Fatalf("PackageName = %q, want %q", project.PackageName, "basic")
	}
	if project.Source != "go" {
		t.Fatalf("Source = %q, want %q", project.Source, "go")
	}
	if len(project.Services) != 1 {
		t.Fatalf("len(Services) = %d, want 1", len(project.Services))
	}

	svc := project.Services[0]
	if svc.Name != "UserService" {
		t.Fatalf("service name = %q, want %q", svc.Name, "UserService")
	}
	if len(svc.Methods) == 0 {
		t.Fatal("expected service methods")
	}

	createUser := svc.Methods[0]
	if createUser.Name != "CreateUser" {
		t.Fatalf("method name = %q, want %q", createUser.Name, "CreateUser")
	}
	if createUser.HTTPMethod != "POST" {
		t.Fatalf("HTTPMethod = %q, want %q", createUser.HTTPMethod, "POST")
	}
	if createUser.Input == nil || createUser.Input.Name != "CreateUserRequest" {
		t.Fatalf("input message = %#v, want CreateUserRequest", createUser.Input)
	}
	if !createUser.Input.HasFields() {
		t.Fatal("expected input fields")
	}
	if createUser.Input.Fields[0].JSONName != "username" {
		t.Fatalf("first input JSONName = %q, want %q", createUser.Input.Fields[0].JSONName, "username")
	}
	if !createUser.Input.Fields[0].Required {
		t.Fatal("expected non-pointer field to be required")
	}
}

func TestFromParseResult_Proto(t *testing.T) {
	dir := t.TempDir()
	protoPath := filepath.Join(dir, "svc.proto")
	content := `
syntax = "proto3";
package greeter;

service Greeter {
  rpc SayHello (HelloRequest) returns (HelloResponse);
}

message HelloRequest {
  string name = 1;
}

message HelloResponse {
  string message = 1;
}
`
	if err := os.WriteFile(protoPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}

	project := ir.FromParseResult(result)

	if project.Source != "proto" {
		t.Fatalf("Source = %q, want %q", project.Source, "proto")
	}
	if len(project.Services) != 1 {
		t.Fatalf("len(Services) = %d, want 1", len(project.Services))
	}
	method := project.Services[0].Methods[0]
	if method.Input == nil || method.Input.Name != "HelloRequest" {
		t.Fatalf("input = %#v, want HelloRequest", method.Input)
	}
	if method.Input.Fields[0].SchemaType != "string" {
		t.Fatalf("schema type = %q, want %q", method.Input.Fields[0].SchemaType, "string")
	}
	if method.Output == nil || method.Output.Name != "HelloResponse" {
		t.Fatalf("output = %#v, want HelloResponse", method.Output)
	}
}

func TestFromTableSchemas(t *testing.T) {
	project := ir.FromTableSchemas([]*dbschema.TableSchema{
		{
			TableName: "users",
			Columns: []dbschema.ColumnInfo{
				{Name: "id", DBType: "bigint unsigned", IsPrimary: true, IsAutoIncr: true},
				{Name: "username", DBType: "varchar(64)", IsNullable: false, Comment: "login name"},
				{Name: "email", DBType: "varchar(128)", IsNullable: false},
				{Name: "age", DBType: "int", IsNullable: true},
			},
		},
	}, "UserAdminService", "")

	if project.Source != "db" {
		t.Fatalf("Source = %q, want %q", project.Source, "db")
	}
	if project.PackageName != "useradminservice" {
		t.Fatalf("PackageName = %q, want %q", project.PackageName, "useradminservice")
	}
	if len(project.Services) != 1 {
		t.Fatalf("len(Services) = %d, want 1", len(project.Services))
	}
	if len(project.Messages) == 0 {
		t.Fatal("expected generated messages")
	}

	svc := project.Services[0]
	if svc.Name != "UserAdminService" {
		t.Fatalf("service name = %q, want %q", svc.Name, "UserAdminService")
	}
	if len(svc.Methods) != 5 {
		t.Fatalf("len(Methods) = %d, want 5", len(svc.Methods))
	}

	createUser := svc.Methods[0]
	if createUser.Name != "CreateUser" {
		t.Fatalf("first method = %q, want %q", createUser.Name, "CreateUser")
	}
	if createUser.Input == nil || createUser.Input.Name != "CreateUserRequest" {
		t.Fatalf("input = %#v, want CreateUserRequest", createUser.Input)
	}
	if createUser.Output == nil || createUser.Output.Name != "CreateUserResponse" {
		t.Fatalf("output = %#v, want CreateUserResponse", createUser.Output)
	}
	if len(createUser.Input.Fields) != 3 {
		t.Fatalf("len(CreateUserRequest.Fields) = %d, want 3", len(createUser.Input.Fields))
	}
	if createUser.Input.Fields[0].JSONName != "username" {
		t.Fatalf("first create field = %q, want %q", createUser.Input.Fields[0].JSONName, "username")
	}
	if !createUser.Input.Fields[0].Required {
		t.Fatal("expected username to be required")
	}

	var userModel *ir.Message
	for _, msg := range project.Messages {
		if msg.Name == "User" {
			userModel = msg
			break
		}
	}
	if userModel == nil {
		t.Fatal("expected User message")
	}
	if len(userModel.Fields) != 4 {
		t.Fatalf("len(User.Fields) = %d, want 4", len(userModel.Fields))
	}
	if userModel.Fields[3].GoType != "*int" {
		t.Fatalf("nullable age GoType = %q, want %q", userModel.Fields[3].GoType, "*int")
	}
}
