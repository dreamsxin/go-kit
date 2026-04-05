package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

func TestParseProto_Greeter(t *testing.T) {
	protoContent := `
syntax = "proto3";

package greeter;

// Greeter provides greeting messages.
service Greeter {
  // SayHello greets a user.
  rpc SayHello (HelloRequest) returns (HelloResponse);
  rpc GetStatus (Empty) returns (StatusResponse);
}

message HelloRequest {
  string name = 1;
  repeated string tags = 2;
}

message HelloResponse {
  string message = 1;
}

message Empty {}

message StatusResponse {
  bool online = 1;
}
`
	tmpDir := t.TempDir()
	protoPath := filepath.Join(tmpDir, "greeter.proto")
	if err := os.WriteFile(protoPath, []byte(protoContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}

	if result.PackageName != "greeter" {
		t.Errorf("PackageName: want %q, got %q", "greeter", result.PackageName)
	}

	if len(result.Services) != 1 {
		t.Fatalf("Services: want 1, got %d", len(result.Services))
	}

	svc := result.Services[0]
	if svc.ServiceName != "Greeter" {
		t.Errorf("ServiceName: want %q, got %q", "Greeter", svc.ServiceName)
	}

	if len(svc.Methods) != 2 {
		t.Fatalf("Methods: want 2, got %d", len(svc.Methods))
	}

	method := svc.Methods[0]
	if method.Name != "SayHello" {
		t.Errorf("Method Name: want %q, got %q", "SayHello", method.Name)
	}
	if method.Input != "HelloRequest" {
		t.Errorf("Method Input: want %q, got %q", "HelloRequest", method.Input)
	}
	if method.HTTPMethod != "post" {
		t.Errorf("Method HTTPMethod: want %q, got %q", "post", method.HTTPMethod)
	}

	if len(result.Models) != 4 {
		t.Fatalf("Models: want 4, got %d", len(result.Models))
	}

	var helloReq *parser.Model
	for _, m := range result.Models {
		if m.Name == "HelloRequest" {
			helloReq = m
			break
		}
	}
	if helloReq == nil {
		t.Fatal("HelloRequest model not found")
	}

	if len(helloReq.Fields) != 2 {
		t.Fatalf("HelloRequest fields: want 2, got %d", len(helloReq.Fields))
	}

	if helloReq.Fields[1].Name != "Tags" {
		t.Errorf("Field Name: want %q, got %q", "Tags", helloReq.Fields[1].Name)
	}
	if helloReq.Fields[1].Type != "[]string" {
		t.Errorf("Field Type: want %q, got %q", "[]string", helloReq.Fields[1].Type)
	}
}

func TestParseProto_Types(t *testing.T) {
	protoContent := `
syntax = "proto3";
package types;

message AllTypes {
  double d = 1;
  float f = 2;
  int32 i32 = 3;
  int64 i64 = 4;
  uint32 u32 = 5;
  uint64 u64 = 6;
  bool b = 7;
  string s = 8;
  bytes by = 9;
  map<string, int32> m = 10;
}
`
	tmpDir := t.TempDir()
	protoPath := filepath.Join(tmpDir, "types.proto")
	os.WriteFile(protoPath, []byte(protoContent), 0644)

	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatal(err)
	}

	model := result.Models[0]
	types := make(map[string]string)
	for _, f := range model.Fields {
		types[f.Name] = f.Type
	}

	tests := []struct {
		name string
		want string
	}{
		{"D", "float64"},
		{"F", "float32"},
		{"I32", "int32"},
		{"I64", "int64"},
		{"U32", "uint32"},
		{"U64", "uint64"},
		{"B", "bool"},
		{"S", "string"},
		{"By", "[]byte"},
		{"M", "map[string]int32"},
	}

	for _, tt := range tests {
		if types[tt.name] != tt.want {
			t.Errorf("Field %s: want %q, got %q", tt.name, tt.want, types[tt.name])
		}
	}
}

func TestParseProto_WellKnownTypes(t *testing.T) {
	protoContent := `
syntax = "proto3";
package wkt;

import "google/protobuf/timestamp.proto";
import "google/protobuf/empty.proto";

message WKT {
  google.protobuf.Timestamp ts = 1;
  google.protobuf.Empty empty = 2;
  Empty custom_empty = 3;
}

message Empty {}
`
	tmpDir := t.TempDir()
	protoPath := filepath.Join(tmpDir, "wkt.proto")
	os.WriteFile(protoPath, []byte(protoContent), 0644)

	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatal(err)
	}

	var model *parser.Model
	for _, m := range result.Models {
		if m.Name == "WKT" {
			model = m
			break
		}
	}
	if model == nil {
		t.Fatal("WKT model not found")
	}

	types := make(map[string]string)
	for _, f := range model.Fields {
		types[f.Name] = f.Type
	}

	if types["Ts"] != "time.Time" {
		t.Errorf("Field Ts: want %q, got %q", "time.Time", types["Ts"])
	}
	if types["Empty"] != "struct{}" {
		t.Errorf("Field Empty: want %q, got %q", "struct{}", types["Empty"])
	}
	if types["CustomEmpty"] != "struct{}" {
		t.Errorf("Field CustomEmpty: want %q, got %q", "struct{}", types["CustomEmpty"])
	}
}

func TestParseProto_MultipleServices(t *testing.T) {
	protoContent := `
syntax = "proto3";
package multi;

service Svc1 {
  rpc Call1 (Empty) returns (Empty);
}

service Svc2 {
  rpc Call2 (Empty) returns (Empty);
}

message Empty {}
`
	tmpDir := t.TempDir()
	protoPath := filepath.Join(tmpDir, "multi.proto")
	os.WriteFile(protoPath, []byte(protoContent), 0644)

	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Services) != 2 {
		t.Fatalf("Services: want 2, got %d", len(result.Services))
	}
	if result.Services[0].ServiceName != "Svc1" {
		t.Errorf("Svc1 name: want %q, got %q", "Svc1", result.Services[0].ServiceName)
	}
	if result.Services[1].ServiceName != "Svc2" {
		t.Errorf("Svc2 name: want %q, got %q", "Svc2", result.Services[1].ServiceName)
	}
}
