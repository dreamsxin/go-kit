package parser_test

import (
	"os"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

// ── GoTypeToSwagType ──────────────────────────────────────────────────────────

func TestGoTypeToSwagType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"int", "integer"},
		{"int8", "integer"},
		{"int16", "integer"},
		{"int32", "integer"},
		{"int64", "integer"},
		{"uint", "integer"},
		{"uint8", "integer"},
		{"uint32", "integer"},
		{"uint64", "integer"},
		{"float32", "number"},
		{"float64", "number"},
		{"bool", "boolean"},
		{"string", "string"},
		{"*string", "string"},
		{"*int", "integer"},
		{"[]string", "array"},
		{"[]int", "array"},
		{"map[string]string", "object"},
		{"SomeStruct", "object"},
		{"*SomeStruct", "object"},
	}
	for _, c := range cases {
		got := parser.GoTypeToSwagType(c.in)
		if got != c.want {
			t.Errorf("GoTypeToSwagType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── ToSnakeCase 扩展 ──────────────────────────────────────────────────────────

func TestToSnakeCase_Extended(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"A", "a"},
		{"AbcDef", "abc_def"},
		{"UserProfileID", "user_profile_id"},
	}
	for _, c := range cases {
		got := parser.ToSnakeCase(c.in)
		if got != c.want {
			t.Errorf("ToSnakeCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── ParseResult.Source ────────────────────────────────────────────────────────

func TestParseFull_SourceIsGo(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	if result.Source != parser.SourceGo {
		t.Errorf("Source: want %q, got %q", parser.SourceGo, result.Source)
	}
}

// ── Method.Route 格式 ─────────────────────────────────────────────────────────

func TestMethod_RouteStartsWithSlash(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	for _, m := range result.Services[0].Methods {
		if len(m.Route) == 0 || m.Route[0] != '/' {
			t.Errorf("method %s: Route %q should start with /", m.Name, m.Route)
		}
	}
}

// ── ModelField.Example ────────────────────────────────────────────────────────

func TestModelField_Example_NonEmpty(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	for _, m := range result.Models {
		if m.Name == "User" {
			for _, f := range m.Fields {
				if f.Example == "" {
					t.Errorf("field %s: Example should not be empty", f.Name)
				}
			}
			return
		}
	}
}

// ── ParseProto HTTP method inference ─────────────────────────────────────────

func TestParseProto_HTTPMethodInference(t *testing.T) {
	protoContent := `
syntax = "proto3";
package svc;

service TestSvc {
  rpc CreateItem (Req) returns (Resp);
  rpc GetItem (Req) returns (Resp);
  rpc ListItems (Req) returns (Resp);
  rpc FindItem (Req) returns (Resp);
  rpc SearchItems (Req) returns (Resp);
  rpc QueryItems (Req) returns (Resp);
  rpc UpdateItem (Req) returns (Resp);
  rpc EditItem (Req) returns (Resp);
  rpc ModifyItem (Req) returns (Resp);
  rpc PatchItem (Req) returns (Resp);
  rpc DeleteItem (Req) returns (Resp);
  rpc RemoveItem (Req) returns (Resp);
}

message Req {}
message Resp {}
`
	protoPath := writeTempProto(t, protoContent)
	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}

	methodMap := map[string]string{}
	for _, m := range result.Services[0].Methods {
		methodMap[m.Name] = m.HTTPMethod
	}

	cases := map[string]string{
		"CreateItem":  "post",
		"GetItem":     "get",
		"ListItems":   "get",
		"FindItem":    "get",
		"SearchItems": "get",
		"QueryItems":  "get",
		"UpdateItem":  "put",
		"EditItem":    "put",
		"ModifyItem":  "put",
		"PatchItem":   "put",
		"DeleteItem":  "delete",
		"RemoveItem":  "delete",
	}
	for name, want := range cases {
		if got := methodMap[name]; got != want {
			t.Errorf("method %s: HTTPMethod want %q, got %q", name, want, got)
		}
	}
}

// ── ParseProto service description ───────────────────────────────────────────

func TestParseProto_ServiceDescription(t *testing.T) {
	protoContent := `
syntax = "proto3";
package desc;

// GreetingService provides greeting functionality.
service GreetingService {
  // SayHi says hi to the user.
  rpc SayHi (Req) returns (Resp);
}

message Req {}
message Resp {}
`
	protoPath := writeTempProto(t, protoContent)
	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}

	svc := result.Services[0]
	if svc.Description == "" {
		t.Error("service Description should be set from comment")
	}
	if svc.Methods[0].Summary == "" {
		t.Error("method Summary should be set from comment")
	}
}

// ── ParseProto map fields ─────────────────────────────────────────────────────

func TestParseProto_MapField(t *testing.T) {
	protoContent := `
syntax = "proto3";
package maptest;

message Config {
  map<string, string> labels = 1;
  map<string, int32> counts = 2;
}
`
	protoPath := writeTempProto(t, protoContent)
	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}

	if len(result.Models) == 0 {
		t.Fatal("expected at least one model")
	}
	fieldMap := map[string]string{}
	for _, f := range result.Models[0].Fields {
		fieldMap[f.Name] = f.Type
	}

	if fieldMap["Labels"] != "map[string]string" {
		t.Errorf("Labels type: got %q, want %q", fieldMap["Labels"], "map[string]string")
	}
	if fieldMap["Counts"] != "map[string]int32" {
		t.Errorf("Counts type: got %q, want %q", fieldMap["Counts"], "map[string]int32")
	}
}

// ── ParseProto source type ────────────────────────────────────────────────────

func TestParseProto_SourceIsProto(t *testing.T) {
	protoContent := `
syntax = "proto3";
package test;
service S { rpc M (Req) returns (Resp); }
message Req {}
message Resp {}
`
	protoPath := writeTempProto(t, protoContent)
	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}
	if result.Source != parser.SourceProto {
		t.Errorf("Source: want %q, got %q", parser.SourceProto, result.Source)
	}
}

// ── helper ────────────────────────────────────────────────────────────────────

func writeTempProto(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/svc.proto"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}
