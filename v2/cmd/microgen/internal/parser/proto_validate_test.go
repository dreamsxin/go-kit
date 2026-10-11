package parser

import (
	"os"
	"strings"
	"testing"
)

// protovalidate field options, in both spellings the ecosystem writes.
const validateOptionsProto = `syntax = "proto3";

package shop;

message CreateUserRequest {
  string username = 1 [(buf.validate.field).string = {min_len: 1, max_len: 64}];
  string email = 2 [(buf.validate.field).string.min_len = 5];
  int32 age = 3 [(buf.validate.field).int32 = {gte: 0, lte: 150}];
  string status = 4 [(buf.validate.field).string = {in: ["active", "suspended"]}];
}
`

// TestProtoValidateOptionsBecomeConstraints: the contract constraints come
// from the proto's own validation options, in either spelling.
func TestProtoValidateOptionsBecomeConstraints(t *testing.T) {
	result := parseProtoContent(t, validateOptionsProto)
	model := result.Models[0]
	if len(model.Fields) != 4 {
		t.Fatalf("fields = %d, want 4", len(model.Fields))
	}
	get := func(name string) ModelField {
		for _, f := range model.Fields {
			if strings.EqualFold(f.Name, name) {
				return f
			}
		}
		t.Fatalf("field %s missing", name)
		return ModelField{}
	}

	username := get("username")
	if username.Constraints.MinLen == nil || *username.Constraints.MinLen != 1 {
		t.Errorf("username minLen = %v, want 1", username.Constraints.MinLen)
	}
	if username.Constraints.MaxLen == nil || *username.Constraints.MaxLen != 64 {
		t.Errorf("username maxLen = %v, want 64", username.Constraints.MaxLen)
	}

	email := get("email")
	if email.Constraints.MinLen == nil || *email.Constraints.MinLen != 5 {
		t.Errorf("email minLen = %v, want 5 (separate named option)", email.Constraints.MinLen)
	}

	age := get("age")
	if age.Constraints.Min == nil || *age.Constraints.Min != 0 {
		t.Errorf("age min = %v, want 0", age.Constraints.Min)
	}
	if age.Constraints.Max == nil || *age.Constraints.Max != 150 {
		t.Errorf("age max = %v, want 150", age.Constraints.Max)
	}

	status := get("status")
	if strings.Join(status.Constraints.Enum, ",") != "active,suspended" {
		t.Errorf("status enum = %v, want active,suspended", status.Constraints.Enum)
	}
}

// TestProtoRPCAuthDoc: the auth: directives parse from the RPC's comment, the
// same grammar the Go IDL uses.
func TestProtoRPCAuthDoc(t *testing.T) {
	protoText := `syntax = "proto3";

package shop;

service ShopService {
  // DeleteProduct removes a product.
  //
  // auth: roles=admin
  rpc DeleteProduct (DeleteProductRequest) returns (DeleteProductResponse);
}

message DeleteProductRequest {
  int64 id = 1;
}

message DeleteProductResponse {}
`
	result := parseProtoContent(t, protoText)
	method := result.Services[0].Methods[0]
	if method.Auth.Required || len(method.Auth.Roles) != 1 || method.Auth.Roles[0] != "admin" {
		t.Fatalf("auth = %+v, want roles=admin and no required flag", method.Auth)
	}

	broken := strings.Replace(protoText, "auth: roles=admin", "auth: sso", 1)
	if _, err := ParseProto(writeTemp(t, broken)); err == nil || !strings.Contains(err.Error(), "unknown directive") {
		t.Errorf("a broken auth directive was not refused: %v", err)
	}
}

func parseProtoContent(t *testing.T, content string) *ParseResult {
	t.Helper()
	result, err := ParseProto(writeTemp(t, content))
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}
	return result
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	protoPath := dir + "/inline.proto"
	if err := os.WriteFile(protoPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return protoPath
}
