package generator_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

// parseProto 解析临时写入的 .proto 文件
func parseProto(t *testing.T, content string) *parser.ParseResult {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "svc.proto")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	result, err := parser.ParseProto(path)
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}
	return result
}

var greeterProto = `
syntax = "proto3";
package greeter;

service Greeter {
  rpc SayHello (HelloRequest) returns (HelloResponse);
  rpc GetStatus (StatusRequest) returns (StatusResponse);
  rpc ListGreetings (ListRequest) returns (ListResponse);
  rpc DeleteGreeting (DeleteRequest) returns (DeleteResponse);
  rpc UpdateGreeting (UpdateRequest) returns (UpdateResponse);
}

message HelloRequest  { string name = 1; }
message HelloResponse { string message = 1; }
message StatusRequest  { string id = 1; }
message StatusResponse { bool online = 1; }
message ListRequest    { int32 page = 1; }
message ListResponse   { repeated string items = 1; }
message DeleteRequest  { string id = 1; }
message DeleteResponse { bool ok = 1; }
message UpdateRequest  { string id = 1; string name = 2; }
message UpdateResponse { string message = 1; }
`

// ── Proto → HTTP 生成 ─────────────────────────────────────────────────────────

func TestGenerateFull_FromProto_HTTPService(t *testing.T) {
	outDir := t.TempDir()
	result := parseProto(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	svcPkg := "greeter"
	mustExist(t, filepath.Join(outDir, "service", svcPkg, "service.go"))
	mustExist(t, filepath.Join(outDir, "endpoint", svcPkg, "endpoints.go"))
	mustExist(t, filepath.Join(outDir, "transport", svcPkg, "transport_http.go"))
	mustExist(t, filepath.Join(outDir, "client", svcPkg, "demo.go"))
	mustExist(t, filepath.Join(outDir, "cmd", "main.go"))
}

func TestGenerateFull_FromProto_ServiceContents(t *testing.T) {
	outDir := t.TempDir()
	result := parseProto(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	svcPath := filepath.Join(outDir, "service", "greeter", "service.go")
	mustContain(t, svcPath, "Greeter")
	mustContain(t, svcPath, "SayHello")
	mustContain(t, svcPath, "GetStatus")
}

func TestGenerateFull_FromProto_HTTPMethodInference(t *testing.T) {
	outDir := t.TempDir()
	result := parseProto(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	httpPath := filepath.Join(outDir, "transport", "greeter", "transport_http.go")
	// Template uses uppercase method strings like "POST /sayhello"
	mustContain(t, httpPath, "POST")
	mustContain(t, httpPath, "GET")
	mustContain(t, httpPath, "DELETE")
	mustContain(t, httpPath, "PUT")
}

// ── Proto → gRPC 生成 ─────────────────────────────────────────────────────────

func TestGenerateFull_FromProto_GRPCTransport(t *testing.T) {
	outDir := t.TempDir()
	result := parseProto(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	svcPkg := "greeter"
	mustExist(t, filepath.Join(outDir, "transport", svcPkg, "transport_grpc.go"))
	mustExist(t, filepath.Join(outDir, "pb", svcPkg, svcPkg+".proto"))

	grpcPath := filepath.Join(outDir, "transport", svcPkg, "transport_grpc.go")
	mustContain(t, grpcPath, "NewGRPCServer")
	mustContain(t, grpcPath, "NewGRPCSayHelloClient")
}

func TestGenerateFull_FromProto_ProtoFileContents(t *testing.T) {
	outDir := t.TempDir()
	result := parseProto(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	protoPath := filepath.Join(outDir, "pb", "greeter", "greeter.proto")
	mustContain(t, protoPath, `syntax = "proto3"`)
	mustContain(t, protoPath, "service Greeter")
	mustContain(t, protoPath, "rpc SayHello")
}

// ── Proto → Skill 生成 ────────────────────────────────────────────────────────

func TestGenerateFull_FromProto_SkillFile(t *testing.T) {
	outDir := t.TempDir()
	result := parseProto(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		DBDriver:   "sqlite",
		WithSkill:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	skillPath := filepath.Join(outDir, "skill", "skill.go")
	mustExist(t, skillPath)
	mustContain(t, skillPath, "SayHello")
}

// ── WithTests 生成 ────────────────────────────────────────────────────────────

func TestGenerateFull_WithTests_Contents(t *testing.T) {
	outDir := t.TempDir()
	result := parseIDL(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithTests:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	testPath := filepath.Join(outDir, "test", "userservice_test.go")
	mustExist(t, testPath)
	mustContain(t, testPath, "UserService")
	mustContain(t, testPath, "func Test")
}

// ── RoutePrefix ───────────────────────────────────────────────────────────────

func TestGenerateFull_RoutePrefix_InTransport(t *testing.T) {
	outDir := t.TempDir()
	result := parseIDL(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		RoutePrefix: "api/v1",
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	httpPath := filepath.Join(outDir, "transport", "userservice", "transport_http.go")
	mustContain(t, httpPath, "/api/v1/userservice")
}

func TestGenerateFull_RoutePrefix_WithLeadingSlash(t *testing.T) {
	outDir := t.TempDir()
	result := parseIDL(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		RoutePrefix: "/v2",
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	httpPath := filepath.Join(outDir, "transport", "userservice", "transport_http.go")
	mustContain(t, httpPath, "/v2/userservice")
}

// ── SDK 内容 ──────────────────────────────────────────────────────────────────

func TestGenerateFull_SDK_Contents(t *testing.T) {
	outDir := t.TempDir()
	result := parseIDL(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	sdkPath := filepath.Join(outDir, "sdk", "userservicesdk", "client.go")
	mustExist(t, sdkPath)
	mustContain(t, sdkPath, "type Client interface")
	mustContain(t, sdkPath, "CreateUser")
	mustContain(t, sdkPath, "GetUser")
}

func TestGenerateFull_SDK_WithGRPC_Contents(t *testing.T) {
	outDir := t.TempDir()
	result := parseIDL(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	sdkPath := filepath.Join(outDir, "sdk", "userservicesdk", "client.go")
	mustContain(t, sdkPath, "NewGRPC")
}

// ── toSnakeCase ───────────────────────────────────────────────────────────────

func TestToSnakeCase_Generator(t *testing.T) {
	// toSnakeCase is used internally; test via generated file names
	// The generator uses it for package names (already tested via PackageNameLowercased)
	// Test indirectly via multi-service generation
	outDir := t.TempDir()
	result := parseIDL(t, "multi.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/multi",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	// OrderService → orderservice, ProductService → productservice
	mustExist(t, filepath.Join(outDir, "service", "orderservice", "service.go"))
	mustExist(t, filepath.Join(outDir, "service", "productservice", "service.go"))
}

// ── Skill 内容验证 ────────────────────────────────────────────────────────────

func TestGenerateFull_Skill_Contents(t *testing.T) {
	outDir := t.TempDir()
	result := parseIDL(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSkill:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	skillPath := filepath.Join(outDir, "skill", "skill.go")
	mustExist(t, skillPath)
	mustContain(t, skillPath, "func Handler")
	mustContain(t, skillPath, "getOpenAITools")
	mustContain(t, skillPath, "getMCPTools")
	mustContain(t, skillPath, "CreateUser")
}

func TestGenerateFull_Skill_NotGeneratedWhenDisabled(t *testing.T) {
	outDir := t.TempDir()
	result := parseIDL(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSkill:  false,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	mustNotExist(t, filepath.Join(outDir, "skill", "skill.go"))
}

// ── 多服务 Skill ──────────────────────────────────────────────────────────────

func TestGenerateFull_MultiService_Skill(t *testing.T) {
	outDir := t.TempDir()
	result := parseIDL(t, "multi.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/multi",
		DBDriver:   "sqlite",
		WithSkill:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateFull(result); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	skillPath := filepath.Join(outDir, "skill", "skill.go")
	mustExist(t, skillPath)
	// multi.go has OrderService with PlaceOrder/GetOrder and ProductService
	mustContain(t, skillPath, "PlaceOrder")
	mustContain(t, skillPath, "GetOrder")
}
