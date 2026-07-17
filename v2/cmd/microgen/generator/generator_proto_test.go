package generator_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/generator"
	"github.com/dreamsxin/go-kit/v2/cmd/microgen/ir"
	"github.com/dreamsxin/go-kit/v2/cmd/microgen/parser"
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

func parseProtoProject(t *testing.T, content string) *ir.Project {
	t.Helper()
	return ir.FromParseResult(parseProto(t, content))
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

var chatStreamProto = `
syntax = "proto3";
package chat;

service ChatService {
  rpc SendMessage (SendMessageRequest) returns (SendMessageResponse);
  rpc WatchMessages (WatchMessagesRequest) returns (stream MessageEvent);
  rpc UploadMessages (stream MessageEvent) returns (UploadSummary);
  rpc Interact (stream MessageEvent) returns (stream MessageEvent);
}

message SendMessageRequest { string body = 1; }
message SendMessageResponse { string id = 1; }
message WatchMessagesRequest { string room_id = 1; }
message MessageEvent { string id = 1; string body = 2; }
message UploadSummary { int32 count = 1; }
`

// ── Proto → HTTP 生成 ─────────────────────────────────────────────────────────

func TestGenerateFull_FromProto_HTTPService(t *testing.T) {
	outDir := t.TempDir()
	project := parseProtoProject(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
		WithTests:  true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
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
	project := parseProtoProject(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	svcPath := filepath.Join(outDir, "service", "greeter", "service.go")
	mustContain(t, svcPath, "Greeter")
	mustContain(t, svcPath, "SayHello")
	mustContain(t, svcPath, "GetStatus")
}

func TestGenerateFull_FromProto_HTTPMethodInference(t *testing.T) {
	outDir := t.TempDir()
	project := parseProtoProject(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
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
	project := parseProtoProject(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
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
	project := parseProtoProject(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	protoPath := filepath.Join(outDir, "pb", "greeter", "greeter.proto")
	mustContain(t, protoPath, `syntax = "proto3"`)
	mustContain(t, protoPath, "service Greeter")
	mustContain(t, protoPath, "rpc SayHello")
	mustContain(t, protoPath, "rpc SayHello (HelloRequest) returns (HelloResponse);")
	mustContain(t, protoPath, "string name = 1;")
	mustContain(t, protoPath, "string message = 1;")
	mustContain(t, protoPath, "int32 page = 1;")
	mustNotContain(t, protoPath, "TODO: fill in the message fields")
}

func TestGenerateFull_FromProto_ServerStreamContents(t *testing.T) {
	outDir := t.TempDir()
	project := parseProtoProject(t, chatStreamProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/chat",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	servicePath := filepath.Join(outDir, "service", "chatservice", "service.go")
	mustContain(t, servicePath, "SendMessage(ctx context.Context, req idl.SendMessageRequest) (idl.SendMessageResponse, error)")
	mustContain(t, servicePath, "WatchMessages(ctx context.Context, req idl.WatchMessagesRequest, send func(idl.MessageEvent) error) error")
	mustContain(t, servicePath, "UploadMessages(ctx context.Context, recv func() (idl.MessageEvent, error)) (idl.UploadSummary, error)")
	mustContain(t, servicePath, "Interact(ctx context.Context, recv func() (idl.MessageEvent, error), send func(idl.MessageEvent) error) error")

	endpointPath := filepath.Join(outDir, "endpoint", "chatservice", "endpoints.go")
	mustContain(t, endpointPath, "SendMessageEndpoint endpoint.Endpoint")
	mustNotContain(t, endpointPath, "WatchMessagesEndpoint")

	httpPath := filepath.Join(outDir, "transport", "chatservice", "transport_http.go")
	mustContain(t, httpPath, "SendMessage")
	mustNotContain(t, httpPath, "WatchMessages")

	grpcPath := filepath.Join(outDir, "transport", "chatservice", "transport_grpc.go")
	mustContain(t, grpcPath, "RegisterGRPCServer(s *grpc.Server, service streamService, endpoints genendpoint.ChatServiceEndpoints)")
	mustContain(t, grpcPath, "func (s *grpcServer) WatchMessages(req *idl.WatchMessagesRequest, stream idl.ChatService_WatchMessagesServer) error")
	mustContain(t, grpcPath, "return stream.Send(&resp)")
	mustContain(t, grpcPath, "NewGRPCWatchMessagesClient(ctx context.Context, client idl.ChatServiceClient, req idl.WatchMessagesRequest")
	mustContain(t, grpcPath, "func (s *grpcServer) UploadMessages(stream idl.ChatService_UploadMessagesServer) error")
	mustContain(t, grpcPath, "return stream.SendAndClose(&resp)")
	mustContain(t, grpcPath, "NewGRPCUploadMessagesClient(ctx context.Context, client idl.ChatServiceClient")
	mustContain(t, grpcPath, "func (s *grpcServer) Interact(stream idl.ChatService_InteractServer) error")
	mustContain(t, grpcPath, "NewGRPCInteractClient(ctx context.Context, client idl.ChatServiceClient")

	routesPath := filepath.Join(outDir, "cmd", "generated_routes.go")
	mustContain(t, routesPath, "RegisterGRPCServer(server, g.chatserviceSvc, g.chatserviceEndpoints)")
	mustNotContain(t, routesPath, `Handler: "WatchMessages"`)

	protoPath := filepath.Join(outDir, "pb", "chatservice", "chatservice.proto")
	mustContain(t, protoPath, "rpc WatchMessages (WatchMessagesRequest) returns (stream MessageEvent);")
	mustContain(t, protoPath, "rpc UploadMessages (stream MessageEvent) returns (UploadSummary);")
	mustContain(t, protoPath, "rpc Interact (stream MessageEvent) returns (stream MessageEvent);")

	sdkPath := filepath.Join(outDir, "sdk", "chatservicesdk", "client.go")
	mustContain(t, sdkPath, "type StreamingClient interface")
	mustContain(t, sdkPath, "func NewGRPCStreaming(conn *grpc.ClientConn) StreamingClient")
	mustContain(t, sdkPath, "WatchMessages(ctx context.Context, req idl.WatchMessagesRequest, send func(idl.MessageEvent) error) error")
	mustContain(t, sdkPath, "UploadMessages(ctx context.Context, recv func() (idl.MessageEvent, error)) (idl.UploadSummary, error)")
	mustContain(t, sdkPath, "Interact(ctx context.Context, recv func() (idl.MessageEvent, error), send func(idl.MessageEvent) error) error")
}

// ── Proto → Skill 生成 ────────────────────────────────────────────────────────

func TestGenerateFull_FromProto_SkillFile(t *testing.T) {
	outDir := t.TempDir()
	project := parseProtoProject(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		DBDriver:   "sqlite",
		WithSkill:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	skillPath := filepath.Join(outDir, "skill", "skill.go")
	mustExist(t, skillPath)
	mustContain(t, skillPath, "SayHello")
}

// ── WithTests 生成 ────────────────────────────────────────────────────────────

func TestGenerateFull_WithTests_Contents(t *testing.T) {
	outDir := t.TempDir()
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithTests:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	testPath := filepath.Join(outDir, "test", "userservice_test.go")
	mustExist(t, testPath)
	mustContain(t, testPath, "UserService")
	mustContain(t, testPath, "func Test")
}

// ── RoutePrefix ───────────────────────────────────────────────────────────────

func TestGenerateFull_RoutePrefix_InGeneratedRoutes(t *testing.T) {
	outDir := t.TempDir()
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		RoutePrefix: "api/v1",
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	routesPath := filepath.Join(outDir, "cmd", "generated_routes.go")
	mustContain(t, routesPath, "/api/v1/userservice")
}

func TestGenerateFull_RoutePrefix_WithLeadingSlash(t *testing.T) {
	outDir := t.TempDir()
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		RoutePrefix: "/v2",
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	routesPath := filepath.Join(outDir, "cmd", "generated_routes.go")
	mustContain(t, routesPath, "/v2/userservice")
}

// ── SDK 内容 ──────────────────────────────────────────────────────────────────

func TestGenerateFull_SDK_Contents(t *testing.T) {
	outDir := t.TempDir()
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	sdkPath := filepath.Join(outDir, "sdk", "userservicesdk", "client.go")
	mustExist(t, sdkPath)
	mustContain(t, sdkPath, "type Client interface")
	mustContain(t, sdkPath, "CreateUser")
	mustContain(t, sdkPath, "GetUser")
}

func TestGenerateFull_SDK_WithGRPC_Contents(t *testing.T) {
	outDir := t.TempDir()
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
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
	project := parseIDLProject(t, "multi.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/multi",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	// OrderService → orderservice, ProductService → productservice
	mustExist(t, filepath.Join(outDir, "service", "orderservice", "service.go"))
	mustExist(t, filepath.Join(outDir, "service", "productservice", "service.go"))
}

// ── Skill 内容验证 ────────────────────────────────────────────────────────────

func TestGenerateFull_Skill_Contents(t *testing.T) {
	outDir := t.TempDir()
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSkill:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
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
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSkill:  false,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustNotExist(t, filepath.Join(outDir, "skill", "skill.go"))
}

// ── 多服务 Skill ──────────────────────────────────────────────────────────────

func TestGenerateFull_MultiService_Skill(t *testing.T) {
	outDir := t.TempDir()
	project := parseIDLProject(t, "multi.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/multi",
		DBDriver:   "sqlite",
		WithSkill:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	skillPath := filepath.Join(outDir, "skill", "skill.go")
	mustExist(t, skillPath)
	// multi.go has OrderService with PlaceOrder/GetOrder and ProductService
	mustContain(t, skillPath, "PlaceOrder")
	mustContain(t, skillPath, "GetOrder")
}
