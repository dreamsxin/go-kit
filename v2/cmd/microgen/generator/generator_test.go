package generator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/generator"
	"github.com/dreamsxin/go-kit/v2/cmd/microgen/ir"
	"github.com/dreamsxin/go-kit/v2/cmd/microgen/parser"
)

// Test helpers

// parseIDL parses an IDL file from parser testdata.
func parseIDL(t *testing.T, name string) *parser.ParseResult {
	idlPath := filepath.Join("..", "parser", "testdata", name)
	result, err := parser.ParseFull(idlPath)
	if err != nil {
		t.Fatalf("ParseFull(%q): %v", name, err)
	}
	return result
}

func parseIDLProject(t *testing.T, name string) *ir.Project {
	t.Helper()
	return ir.FromParseResult(parseIDL(t, name))
}

func parseIDLContent(t *testing.T, content string) *parser.ParseResult {
	t.Helper()
	dir := t.TempDir()
	idlPath := filepath.Join(dir, "inline.go")
	if err := os.WriteFile(idlPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", idlPath, err)
	}
	result, err := parser.ParseFull(idlPath)
	if err != nil {
		t.Fatalf("ParseFull(%q): %v", idlPath, err)
	}
	return result
}

func parseIDLContentProject(t *testing.T, content string) *ir.Project {
	t.Helper()
	return ir.FromParseResult(parseIDLContent(t, content))
}

// newTmpDir 返回临时目录，测试结束后自动清理。
func newTmpDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// mustExist 断言路径存在。
func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected path to exist: %s", path)
	}
}

// mustNotExist 断言路径不存在。
func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("expected path NOT to exist: %s", path)
	}
}

// readFile 读取文件内容，失败时调用 Fatal。
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	return string(b)
}

// mustContain 断言文件内容包含指定子串。
func mustContain(t *testing.T, path, substr string) {
	t.Helper()
	content := readFile(t, path)
	if !strings.Contains(content, substr) {
		t.Errorf("file %q should contain %q\ncontent snippet:\n%s", path, substr, content[:minLen(200, len(content))])
	}
}

func mustNotContain(t *testing.T, path, substr string) {
	t.Helper()
	content := readFile(t, path)
	if strings.Contains(content, substr) {
		t.Errorf("file %q should not contain %q", path, substr)
	}
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// mustNewGenerator 创建 Generator，失败时调用 Fatal。
func mustNewGenerator(t *testing.T, opts generator.Options) *generator.Generator {
	t.Helper()
	// 未指定 TemplateFS 时使用测试全局 FS。
	if opts.TemplateFS == nil {
		opts.TemplateFS = testTemplateFS
	}
	gen, err := generator.New(opts)
	if err != nil {
		t.Fatalf("generator.New: %v", err)
	}
	return gen
}

// ─────────────────────────── generator.New ─────────────────────────────────

func TestNew_DefaultDriver(t *testing.T) {
	_, err := generator.New(generator.Options{
		TemplateFS: testTemplateFS,
		OutputDir:  t.TempDir(),
		DBDriver:   "",
	})
	if err != nil {
		t.Errorf("New with empty DBDriver: unexpected error: %v", err)
	}
}

func TestNew_WithDBRequiresDriver(t *testing.T) {
	_, err := generator.New(generator.Options{
		TemplateFS: testTemplateFS,
		OutputDir:  t.TempDir(),
		WithDB:     true,
	})
	if err == nil || !strings.Contains(err.Error(), "requires a db driver") {
		t.Fatalf("New error = %v, want missing db driver error", err)
	}
}

func TestNew_UnsupportedDriver(t *testing.T) {
	_, err := generator.New(generator.Options{
		TemplateFS: testTemplateFS,
		OutputDir:  t.TempDir(),
		DBDriver:   "oracle",
	})
	if err == nil {
		t.Error("expected error for unsupported driver, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported db driver") {
		t.Errorf("error message: want %q, got %q", "unsupported db driver", err.Error())
	}
}

func TestNew_AllSupportedDrivers(t *testing.T) {
	drivers := []string{"sqlite", "mysql", "postgres", "sqlserver", "clickhouse"}
	for _, d := range drivers {
		d := d
		t.Run(d, func(t *testing.T) {
			_, err := generator.New(generator.Options{
				TemplateFS: testTemplateFS,
				OutputDir:  t.TempDir(),
				DBDriver:   d,
			})
			if err != nil {
				t.Errorf("driver %q: unexpected error: %v", d, err)
			}
		})
	}
}

func TestNew_GRPCProtocol(t *testing.T) {
	outDir := t.TempDir()
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/test",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	svcPkg := strings.ToLower(project.Services[0].Name)
	mustExist(t, filepath.Join(outDir, "pb", svcPkg, svcPkg+".proto"))
	mustExist(t, filepath.Join(outDir, "transport", svcPkg, "transport_grpc.go"))
}

// ─────────────────────────── 目录结构 ─────────────────────────────────────

func TestGenerateFull_DirectoryStructure_HTTP(t *testing.T) {
	outDir := newTmpDir(t)
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

	svcPkg := "userservice"
	mustExist(t, filepath.Join(outDir, "cmd", "main.go"))
	mustExist(t, filepath.Join(outDir, "service", svcPkg, "service.go"))
	mustExist(t, filepath.Join(outDir, "endpoint", svcPkg, "endpoints.go"))
	mustExist(t, filepath.Join(outDir, "transport", svcPkg, "transport_http.go"))
	mustExist(t, filepath.Join(outDir, "client", svcPkg, "demo.go"))

	// 仅启用 HTTP 时，不应生成 gRPC 相关文件。
	mustNotExist(t, filepath.Join(outDir, "pb"))
	mustNotExist(t, filepath.Join(outDir, "transport", svcPkg, "transport_grpc.go"))
}

func TestGenerateFull_DirectoryStructure_WithModel(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithModel:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "model", "generated_user.go"))
	mustExist(t, filepath.Join(outDir, "repository", "generated_user_repository.go"))
}

func TestGenerateFull_DirectoryStructure_WithTests(t *testing.T) {
	outDir := newTmpDir(t)
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

	svcPkg := "userservice"
	mustExist(t, filepath.Join(outDir, "test", svcPkg+"_test.go"))
}

func TestGenerateFull_DirectoryStructure_WithConfig(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "config", "config.yaml"))
	mustExist(t, filepath.Join(outDir, "config", "config.go"))
	mustExist(t, filepath.Join(outDir, "config", "local.go"))
	mustExist(t, filepath.Join(outDir, "config", "env.go"))
	mustExist(t, filepath.Join(outDir, "config", "remote.go"))
	mustExist(t, filepath.Join(outDir, "config", "loader.go"))
}

func TestGenerateFull_DirectoryStructure_WithDocs(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "README.md"))
}

func TestGenerateFull_DirectoryStructure_WithOpenAPI(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithConfig:  false,
		WithDocs:    false,
		WithOpenAPI: true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "docs", "docs.go"))
	mustExist(t, filepath.Join(outDir, "docs", "openapi.json"))
	mustExist(t, filepath.Join(outDir, "docs", "schema.json"))
	mustExist(t, filepath.Join(outDir, "sdk", "typescript", "client.ts"))
	mustExist(t, filepath.Join(outDir, "sdk", "typescript", "README.md"))
	mustExist(t, filepath.Join(outDir, "sdk", "typescript", "tsconfig.json"))
}

// ─────────────────────────── 生成文件内容验证 ─────────────────────────────

func TestGenerateFull_ServiceFile_Contents(t *testing.T) {
	outDir := newTmpDir(t)
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

	servicePath := filepath.Join(outDir, "service", "userservice", "service.go")
	mustContain(t, servicePath, "UserService")
	mustContain(t, servicePath, "CreateUser")
	mustContain(t, servicePath, "GetUser")
	mustContain(t, servicePath, "ListUsers")
	mustContain(t, servicePath, "DeleteUser")
	mustContain(t, servicePath, "UpdateUser")
	mustContain(t, servicePath, "LoggingMiddleware")
}

func TestGenerateFull_EndpointsFile_Contents(t *testing.T) {
	outDir := newTmpDir(t)
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

	epPath := filepath.Join(outDir, "endpoint", "userservice", "endpoints.go")
	mustContain(t, epPath, "UserServiceEndpoints")
	mustContain(t, epPath, "MakeServerEndpoints")
	mustContain(t, epPath, "MakeCreateUserEndpoint")
	mustContain(t, epPath, "RetryEnabled:       false")
	mustContain(t, epPath, "RetryMiddleware")
	mustContain(t, epPath, "retryableEndpointError")
}

func TestGenerateFull_TransportHTTP_Contents(t *testing.T) {
	outDir := newTmpDir(t)
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

	httpPath := filepath.Join(outDir, "transport", "userservice", "transport_http.go")
	mustContain(t, httpPath, "NewHTTPHandler")
	mustContain(t, httpPath, "server.NewServer")
	mustContain(t, httpPath, "DefaultMaxJSONBodyBytes")
	mustContain(t, httpPath, "JSONDecodeError")
	mustContain(t, httpPath, "transporthttp.DecodeQueryRequest")
	mustContain(t, httpPath, "*http.ServeMux")
	mustNotContain(t, httpPath, "github.com/gorilla/mux")
	mustNotContain(t, httpPath, "reflect.Value")
	mustContain(t, httpPath, "server.JSONErrorEncoder")
	mustContain(t, httpPath, "decodeCreateUserRequest")
	mustContain(t, httpPath, "encodeCreateUserResponse")
}

func TestGenerateFull_TransportGRPC_Contents(t *testing.T) {
	outDir := newTmpDir(t)
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

	grpcPath := filepath.Join(outDir, "transport", "userservice", "transport_grpc.go")
	mustContain(t, grpcPath, "NewGRPCServer")
	mustContain(t, grpcPath, "NewGRPCCreateUserClient")
	mustContain(t, grpcPath, "decodeGRPCCreateUserRequest")
}

func TestGenerateFull_ProtoFile_Contents(t *testing.T) {
	outDir := newTmpDir(t)
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

	protoPath := filepath.Join(outDir, "pb", "userservice", "userservice.proto")
	mustContain(t, protoPath, `syntax = "proto3"`)
	mustContain(t, protoPath, "service UserService")
	mustContain(t, protoPath, "rpc CreateUser")
	mustContain(t, protoPath, "rpc GetUser")
	mustContain(t, protoPath, "string username = 1;")
	mustContain(t, protoPath, "string email = 2;")
	mustContain(t, protoPath, "User user = 1;")
	mustContain(t, protoPath, "string error = 2;")
	mustContain(t, protoPath, "int64 id = 1;")
	mustContain(t, protoPath, "double score = 5;")
	mustNotContain(t, protoPath, "TODO: fill in the message fields")
}

func TestGenerateFull_ProtoFile_ComplexTypeMappings(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLContentProject(t, `package complex

import "context"
import "time"

type Attachment struct {
	Name string `+"`json:\"name\"`"+`
	Blob []byte `+"`json:\"blob\"`"+`
}

type CreateEventRequest struct {
	Title     string            `+"`json:\"title\"`"+`
	Nickname  *string           `+"`json:\"nickname\"`"+`
	Priority  *int32            `+"`json:\"priority\"`"+`
	Tags      []string          `+"`json:\"tags\"`"+`
	Metadata  map[string]string `+"`json:\"metadata\"`"+`
	TTL       time.Duration     `+"`json:\"ttl\"`"+`
	OccurredAt time.Time        `+"`json:\"occurred_at\"`"+`
	Attachment *Attachment      `+"`json:\"attachment\"`"+`
}

type CreateEventResponse struct {
	ID        uint64       `+"`json:\"id\"`"+`
	CreatedAt time.Time    `+"`json:\"created_at\"`"+`
	Payload   []byte       `+"`json:\"payload\"`"+`
	Items     []Attachment `+"`json:\"items\"`"+`
}

type EventService interface {
	CreateEvent(ctx context.Context, req CreateEventRequest) (CreateEventResponse, error)
}
`)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/complex",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	protoPath := filepath.Join(outDir, "pb", "eventservice", "eventservice.proto")
	mustContain(t, protoPath, `import "google/protobuf/timestamp.proto";`)
	mustContain(t, protoPath, `import "google/protobuf/duration.proto";`)
	mustContain(t, protoPath, "string title = 1;")
	mustContain(t, protoPath, "optional string nickname = 2;")
	mustContain(t, protoPath, "optional int32 priority = 3;")
	mustContain(t, protoPath, "repeated string tags = 4;")
	mustContain(t, protoPath, "map<string, string> metadata = 5;")
	mustContain(t, protoPath, "google.protobuf.Duration ttl = 6;")
	mustContain(t, protoPath, "google.protobuf.Timestamp occurred_at = 7;")
	mustContain(t, protoPath, "Attachment attachment = 8;")
	mustContain(t, protoPath, "uint64 id = 1;")
	mustContain(t, protoPath, "google.protobuf.Timestamp created_at = 2;")
	mustContain(t, protoPath, "bytes payload = 3;")
	mustContain(t, protoPath, "repeated Attachment items = 4;")
	mustContain(t, protoPath, "message Attachment")
	mustContain(t, protoPath, "bytes blob = 2;")
	mustNotContain(t, protoPath, "TODO: fill in the message fields")
}

func TestGenerateFull_ModelFile_Contents(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithModel:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	modelPath := filepath.Join(outDir, "model", "generated_user.go")
	mustContain(t, modelPath, "User")
	mustContain(t, modelPath, "TableName")
}

func TestGenerateFull_RepositoryFile_Contents(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithModel:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	repoPath := filepath.Join(outDir, "repository", "generated_user_repository.go")
	mustContain(t, repoPath, "Repository")
	mustContain(t, repoPath, "GetByID")
	mustContain(t, repoPath, "Create")
	mustContain(t, repoPath, "Delete")
}

func TestGenerateFull_ClientDemo_Contents(t *testing.T) {
	outDir := newTmpDir(t)
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

	demoPath := filepath.Join(outDir, "client", "userservice", "demo.go")
	mustContain(t, demoPath, "runDemo")
	mustContain(t, demoPath, "UserService")
}

func TestGenerateFull_ClientDemo_WithGRPC(t *testing.T) {
	outDir := newTmpDir(t)
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

	demoPath := filepath.Join(outDir, "client", "userservice", "demo.go")
	mustContain(t, demoPath, "GRPCClient")
}

// ─────────────────────────── main.go 生成 ─────────────────────────────────

func TestGenerateFull_MainFile_HTTPOnly(t *testing.T) {
	outDir := newTmpDir(t)
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

	mainPath := filepath.Join(outDir, "cmd", "main.go")
	mustContain(t, mainPath, "http.addr")
	mustContain(t, mainPath, "ListenAndServe")
}

func TestGenerateFull_MainFile_WithGRPC(t *testing.T) {
	outDir := newTmpDir(t)
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

	mainPath := filepath.Join(outDir, "cmd", "main.go")
	mustContain(t, mainPath, "grpc.addr")
	mustContain(t, mainPath, "grpcStopped := make(chan struct{})")
	mustContain(t, mainPath, "grpcServer.Stop()")
}

func TestGenerateFull_MainFile_WithDB(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "mysql",
		WithDB:     true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mainPath := filepath.Join(outDir, "cmd", "main.go")
	mustContain(t, mainPath, "gorm.Open")
	mustContain(t, mainPath, "db.dsn")
	mustContain(t, mainPath, "auto-migrate")
	mustContain(t, mainPath, "DB migration skipped")
	mustContain(t, mainPath, "redactDSN(*dsn)")
	mustContain(t, mainPath, "func redactDSN(dsn string) string")
	mustNotContain(t, mainPath, `dsn=%s]", "mysql", *dsn`)
}

func TestGenerateFull_MainFile_WithConfigUsesLoggingConfig(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mainPath := filepath.Join(outDir, "cmd", "main.go")
	mustContain(t, mainPath, "newConfiguredLogger(cfg.Logging)")
	mustContain(t, mainPath, "zap.NewProductionConfig()")
}

// ─────────────────────────── go.mod 生成 ─────────────────────────────────

func TestGenerateFull_GoMod_Created(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/myproject",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	goModPath := filepath.Join(outDir, "go.mod")
	mustExist(t, goModPath)
	mustContain(t, goModPath, "module example.com/myproject")
	mustContain(t, goModPath, "go 1.25.8")
	mustContain(t, goModPath, "github.com/dreamsxin/go-kit/v2 v2.0.0")
	mustNotContain(t, goModPath, "replace github.com/dreamsxin/go-kit/v2")
}

func TestGenerateFull_GoMod_WithConfigIncludesViper(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/myproject",
		DBDriver:   "sqlite",
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	goModPath := filepath.Join(outDir, "go.mod")
	mustContain(t, goModPath, "github.com/spf13/viper")
	mustContain(t, goModPath, "github.com/spf13/viper/remote")
}

func TestOptionsNormalize_DerivesDefaults(t *testing.T) {
	opt := generator.Options{
		WithConfig: true,
		Protocols:  []string{"http", " grpc "},
	}
	got := opt.Normalize()
	if got.OutputDir != "." {
		t.Fatalf("OutputDir = %q, want .", got.OutputDir)
	}
	if got.ConfigMode != "file" {
		t.Fatalf("ConfigMode = %q, want file", got.ConfigMode)
	}
	if !got.WithGRPC {
		t.Fatal("WithGRPC = false, want true")
	}
}

func TestNew_ConfigModeValidation(t *testing.T) {
	_, err := generator.New(generator.Options{
		TemplateFS: testTemplateFS,
		OutputDir:  t.TempDir(),
		WithConfig: true,
		ConfigMode: "invalid",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported -config-mode") {
		t.Fatalf("New invalid config mode error = %v, want unsupported -config-mode", err)
	}
}

func TestNew_RemoteProviderValidation(t *testing.T) {
	_, err := generator.New(generator.Options{
		TemplateFS:     testTemplateFS,
		OutputDir:      t.TempDir(),
		WithConfig:     true,
		ConfigMode:     "hybrid",
		RemoteProvider: "apollo",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported -remote-provider") {
		t.Fatalf("New invalid remote provider error = %v, want unsupported -remote-provider", err)
	}
}

func TestNew_RemoteModeRequiresProvider(t *testing.T) {
	_, err := generator.New(generator.Options{
		TemplateFS: testTemplateFS,
		OutputDir:  t.TempDir(),
		WithConfig: true,
		ConfigMode: "remote",
	})
	if err == nil || !strings.Contains(err.Error(), "-config-mode=remote requires -remote-provider") {
		t.Fatalf("New remote mode without provider error = %v, want provider requirement", err)
	}
}

// TestGenerateFull_GoMod_ModuleUpdatedWhenMismatch 验证：go.mod 已存在但 module 名与 -import 不符时，
// generator 只更新 module 行，其余内容（go 版本、require 块等）保持不变。
func TestGenerateFull_GoMod_ModuleUpdatedWhenMismatch(t *testing.T) {
	outDir := newTmpDir(t)

	// Pre-write go.mod with a different module path.
	existingContent := "module existing.com/pkg\n\ngo 1.22\n"
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), []byte(existingContent), 0o644); err != nil {
		t.Fatalf("pre-write go.mod: %v", err)
	}

	result := parseIDL(t, "basic.go")
	project := ir.FromParseResult(result)
	gen := mustNewGenerator(t, generator.Options{OutputDir: outDir, ImportPath: "example.com/new", DBDriver: "sqlite", WithConfig: false, WithDocs: false})

	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	goModPath := filepath.Join(outDir, "go.mod")
	// module 行应已被更新
	mustContain(t, goModPath, "module example.com/new")
	// go 版本行应保留，其他内容不应丢失。
	mustContain(t, goModPath, "go 1.22")
}

// TestGenerateFull_GoMod_SkippedWhenModuleMatches 验证 go.mod 的 module 与 -import
// 一致时，不修改文件内容，也不丢失用户自定义的 require。
func TestGenerateFull_GoMod_SkippedWhenModuleMatches(t *testing.T) {
	outDir := newTmpDir(t)

	// 预先写入 go.mod，module 名与后续 ImportPath 相同
	existingContent := "module example.com/same\n\ngo 1.22\n\nrequire some/dep v1.0.0\n"
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), []byte(existingContent), 0644); err != nil {
		t.Fatalf("pre-write go.mod: %v", err)
	}

	result := parseIDL(t, "basic.go")
	project := ir.FromParseResult(result)
	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/same",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	content := readFile(t, filepath.Join(outDir, "go.mod"))
	if content != existingContent {
		t.Errorf("go.mod should not be changed when module matches\ngot:\n%s\nwant:\n%s", content, existingContent)
	}
}

func TestGenerateFull_NoImportPath_SkipsGoMod(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustNotExist(t, filepath.Join(outDir, "go.mod"))
}

// ─────────────────────────── IDL 文件复制 ─────────────────────────────────

func TestGenerateFull_IDLFileCopied(t *testing.T) {
	outDir := newTmpDir(t)
	idlPath := filepath.Join("..", "parser", "testdata", "basic.go")
	result, err := parser.ParseFull(idlPath)
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	project := ir.FromParseResult(result)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		IDLSrcPath: idlPath,
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "idl.go"))
	content := readFile(t, filepath.Join(outDir, "idl.go"))
	if !strings.HasPrefix(strings.TrimSpace(content), "package basic") {
		t.Errorf("idl.go should start with 'package basic', got:\n%s", content[:minLen(80, len(content))])
	}
}

// ─────────────────────────── config.yaml ─────────────────────────────────

func TestGenerateFull_ConfigYAML_HTTPOnly(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	configPath := filepath.Join(outDir, "config", "config.yaml")
	mustContain(t, configPath, "http_addr")
	mustContain(t, configPath, "circuit_breaker")
	mustContain(t, configPath, "retry:")
	mustContain(t, configPath, "max_attempts: 3")
	mustContain(t, configPath, "backoff: \"2s\"")
	mustContain(t, configPath, "remote:")
	mustContain(t, configPath, "fallback_to_local: true")
	mustContain(t, configPath, `provider: ""`)
}

func TestGenerateFull_ConfigYAML_HybridModeDefaults(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:      outDir,
		ImportPath:     "example.com/basic",
		DBDriver:       "sqlite",
		WithConfig:     true,
		ConfigMode:     "hybrid",
		RemoteProvider: "consul",
		WithDocs:       false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	configPath := filepath.Join(outDir, "config", "config.yaml")
	mustContain(t, configPath, "enabled: true")
	mustContain(t, configPath, `provider: "consul"`)
	mustContain(t, configPath, "fallback_to_local: true")
}

func TestGenerateFull_ConfigYAML_RemoteModeDefaults(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:      outDir,
		ImportPath:     "example.com/basic",
		DBDriver:       "sqlite",
		WithConfig:     true,
		ConfigMode:     "remote",
		RemoteProvider: "consul",
		WithDocs:       false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	configPath := filepath.Join(outDir, "config", "config.yaml")
	mustContain(t, configPath, "enabled: true")
	mustContain(t, configPath, `provider: "consul"`)
	mustContain(t, configPath, "fallback_to_local: false")
}

func TestGenerateFull_ConfigYAML_WithGRPC(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustContain(t, filepath.Join(outDir, "config", "config.yaml"), "grpc_addr")
}

func TestGenerateFull_ConfigYAML_WithDB(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "mysql",
		WithDB:     true,
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	configPath := filepath.Join(outDir, "config", "config.yaml")
	mustContain(t, configPath, "database:")
	mustContain(t, configPath, `driver: "mysql"`)
	mustContain(t, configPath, "auto_migrate: false")
}

// ─────────────────────────── config/config.go ─────────────────────────────────

func TestGenerateFull_ConfigCode_Generated(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	codePath := filepath.Join(outDir, "config", "config.go")
	localPath := filepath.Join(outDir, "config", "local.go")
	envPath := filepath.Join(outDir, "config", "env.go")
	remotePath := filepath.Join(outDir, "config", "remote.go")
	loaderPath := filepath.Join(outDir, "config", "loader.go")
	mustExist(t, codePath)
	mustExist(t, localPath)
	mustExist(t, envPath)
	mustExist(t, remotePath)
	mustExist(t, loaderPath)
	mustContain(t, codePath, "type Config struct")
	mustContain(t, codePath, "func Default()")
	mustContain(t, codePath, "func (cfg *Config) Validate() error")
	mustContain(t, codePath, `yaml:"server"`)
	mustContain(t, codePath, "type RemoteConfig struct")
	mustContain(t, codePath, `yaml:"remote"`)
	mustContain(t, localPath, "func LoadLocal(path string)")
	mustContain(t, envPath, "func ApplyEnv(cfg *Config) error")
	mustContain(t, remotePath, "func LoadRemote(cfg *Config) (*Config, error)")
	mustContain(t, loaderPath, "func Load(path string)")
	mustContain(t, loaderPath, "cfg, err = LoadRemote(cfg)")
	mustContain(t, loaderPath, "if err := cfg.Validate(); err != nil")
}

func TestGenerateFull_ConfigCode_EnvOverrides(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		Protocols:   []string{"http", "grpc"},
		DBDriver:    "mysql",
		WithDB:      true,
		WithOpenAPI: true,
		WithConfig:  true,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	codePath := filepath.Join(outDir, "config", "config.go")
	envPath := filepath.Join(outDir, "config", "env.go")
	mustContain(t, codePath, `const envPrefix = "APP_"`)
	mustContain(t, envPath, `readString("HTTP_ADDR"`)
	mustContain(t, envPath, `readString("GRPC_ADDR"`)
	mustContain(t, envPath, `readString("LOG_LEVEL"`)
	mustContain(t, envPath, `readString("DB_DSN"`)
	mustContain(t, envPath, `readBool("DB_AUTO_MIGRATE"`)
	mustNotContain(t, envPath, `readString("SWAGGER_HOST"`)
	mustContain(t, envPath, `readBool("RETRY_ENABLED"`)
	mustContain(t, envPath, `readInt("RETRY_MAX_ATTEMPTS"`)
	mustContain(t, envPath, `readDuration("RETRY_BACKOFF"`)
	mustContain(t, envPath, `readBool("DEBUG_ROUTES_ENABLED"`)
	mustContain(t, envPath, `readBool("REMOTE_ENABLED"`)
	mustContain(t, envPath, `readString("REMOTE_PROVIDER"`)
	mustContain(t, envPath, `readDuration("REMOTE_TIMEOUT"`)
	mustContain(t, envPath, `readBool("REMOTE_FALLBACK_TO_LOCAL"`)
	mustContain(t, envPath, "strconv.ParseBool")
	mustContain(t, envPath, "strconv.Atoi")
	mustContain(t, envPath, "strconv.ParseFloat")
	mustContain(t, envPath, "time.ParseDuration")
}

func TestGenerateFull_ConfigCode_RemoteConfigDefaults(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	codePath := filepath.Join(outDir, "config", "config.go")
	remotePath := filepath.Join(outDir, "config", "remote.go")
	mustContain(t, codePath, "Enabled         bool")
	mustContain(t, codePath, "Provider        string")
	mustContain(t, codePath, "DataID          string")
	mustContain(t, codePath, "FallbackToLocal bool")
	mustContain(t, codePath, "Timeout:         5 * time.Second")
	mustContain(t, remotePath, `"github.com/spf13/viper/remote"`)
	mustContain(t, remotePath, `"github.com/spf13/viper"`)
	mustContain(t, remotePath, `case "consul":`)
	mustContain(t, remotePath, "func loadRemoteFromConsul(cfg *Config) (*Config, error)")
	mustContain(t, remotePath, `v.AddRemoteProvider("consul", endpoint, dataID)`)
	mustContain(t, remotePath, "v.ReadRemoteConfig()")
	mustContain(t, remotePath, "v.Unmarshal(&merged)")
}

func TestGenerateFull_ConfigCode_WithGRPC(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	codePath := filepath.Join(outDir, "config", "config.go")
	mustContain(t, codePath, "GRPCAddr")
}

func TestGenerateFull_ConfigCode_WithDB(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "mysql",
		WithDB:     true,
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	codePath := filepath.Join(outDir, "config", "config.go")
	mustContain(t, codePath, "type DatabaseConfig struct")
	mustContain(t, codePath, "AutoMigrate     bool")
	mustContain(t, codePath, "AutoMigrate:     false")
	mustContain(t, codePath, "Retry: RetryConfig")
	mustContain(t, codePath, "MaxAttempts: 3")
	mustContain(t, codePath, `"mysql"`) // Default() 中包含 Driver: "mysql"。
}

func TestGenerateFull_ConfigCode_NotGeneratedWhenWithConfigFalse(t *testing.T) {
	outDir := newTmpDir(t)
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

	codePath := filepath.Join(outDir, "config", "config.go")
	if _, err := os.Stat(codePath); err == nil {
		t.Errorf("config.go should NOT be generated when WithConfig=false")
	}
	for _, path := range []string{
		filepath.Join(outDir, "config", "local.go"),
		filepath.Join(outDir, "config", "env.go"),
		filepath.Join(outDir, "config", "remote.go"),
		filepath.Join(outDir, "config", "loader.go"),
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s should NOT be generated when WithConfig=false", path)
		}
	}
}

// ─────────────────────────── docs/docs.go ─────────────────────────────────

func TestGenerateFull_OpenAPIEmbed_Contents(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	docsPath := filepath.Join(outDir, "docs", "docs.go")
	mustContain(t, docsPath, "package docs")
	mustContain(t, docsPath, "go:embed openapi.json")
	mustContain(t, docsPath, "go:embed schema.json")
	mustContain(t, docsPath, "func Handler")
	mustContain(t, docsPath, "func SchemaHandler")
	mustContain(t, docsPath, "application/schema+json; charset=utf-8")
	mustContain(t, filepath.Join(outDir, "docs", "openapi.json"), `"openapi": "3.1.0"`)
	mustContain(t, filepath.Join(outDir, "docs", "schema.json"), `"$schema": "https://json-schema.org/draft/2020-12/schema"`)
}

// ─────────────────────────── README.md 内容 ─────────────────────────────

func TestGenerateFull_Readme_Contents(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	readmePath := filepath.Join(outDir, "README.md")
	mustContain(t, readmePath, "UserService")
	mustContain(t, readmePath, "go run ./cmd")
	mustContain(t, readmePath, "## Project Map")
	mustContain(t, readmePath, "service/<name>/service.go")
	mustContain(t, readmePath, "cmd/generated_*.go")
	mustContain(t, readmePath, "microgen extend -check -out .")
	mustContain(t, readmePath, "## Capability Contract")
	mustContain(t, readmePath, "The same contract drives HTTP routes, gRPC/proto assets, generated clients, SDKs, OpenAPI and JSON Schema output, README endpoint listings, and AI tool metadata.")
	mustContain(t, readmePath, "microgen.skill.v1")
	mustContain(t, readmePath, "`/skill?format=mcp` is discovery output, not a tool execution endpoint.")
	mustContain(t, readmePath, "interaction.NewRuntime")
	mustContain(t, readmePath, "interaction.AuthorizationHook")
	mustContain(t, readmePath, "interaction/mcp.NewHandler")
	mustContain(t, readmePath, "## Extend Existing Project")
	mustContain(t, readmePath, "microgen extend -idl full_combined.go -out . -append-service OrderService")
	mustContain(t, readmePath, "microgen extend -idl full_combined.go -out . -append-model Product")
	mustContain(t, readmePath, "microgen extend -idl full_combined.go -out . -append-middleware tracing,error-handling,metrics")
	mustContain(t, readmePath, "Keep business logic in user-owned files")
	mustContain(t, readmePath, "## Agent Workflow")
	mustContain(t, readmePath, "Read this README and inspect the source contract snapshot before editing.")
	mustContain(t, readmePath, "GET /skill?format=mcp")
	mustContain(t, readmePath, "Use `interaction` runtime hooks for executable AI sessions")
	mustContain(t, readmePath, "Run the smallest relevant validation first, usually `go test ./...`")
	mustContain(t, readmePath, "GET /debug/routes")
	mustNotContain(t, readmePath, "## Configuration")
	mustNotContain(t, readmePath, "protoc --go_out=.")
}

func TestGenerateFull_Readme_ConfigDefaults(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: true,
		WithDocs:   true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	readmePath := filepath.Join(outDir, "README.md")
	mustContain(t, readmePath, "## Configuration")
	mustContain(t, readmePath, "Generated config loads through `config.Load(path)`")
	mustContain(t, readmePath, "Current generated config mode: `file`")
	mustContain(t, readmePath, "Current remote provider: `none`")
	mustContain(t, readmePath, "APP_HTTP_ADDR")
}

func TestGenerateFull_Readme_RemoteConfigMode(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:      outDir,
		ImportPath:     "example.com/basic",
		DBDriver:       "sqlite",
		WithConfig:     true,
		ConfigMode:     "remote",
		RemoteProvider: "consul",
		WithDocs:       true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	readmePath := filepath.Join(outDir, "README.md")
	mustContain(t, readmePath, "Current generated config mode: `remote`")
	mustContain(t, readmePath, "Current remote provider: `consul`")
	mustContain(t, readmePath, "Remote mode enables strict remote loading")
}

func TestGenerateFull_Readme_ProtoQuickStart(t *testing.T) {
	outDir := newTmpDir(t)
	protoPath := filepath.Join(outDir, "service.proto")
	if err := os.WriteFile(protoPath, []byte(`syntax = "proto3";
package userservice;

service UserService {
  rpc GetUser (GetUserRequest) returns (GetUserResponse);
  rpc CreateUser (CreateUserRequest) returns (CreateUserResponse);
}

message GetUserRequest {}
message GetUserResponse {}
message CreateUserRequest {}
message CreateUserResponse {}
`), 0o644); err != nil {
		t.Fatalf("WriteFile proto: %v", err)
	}
	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}
	project := ir.FromParseResult(result)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/protoquickstart",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   true,
		WithGRPC:   true,
		IDLSrcPath: protoPath,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	readmePath := filepath.Join(outDir, "README.md")
	mustContain(t, readmePath, "protoc --go_out=. --go-grpc_out=.")
	mustContain(t, readmePath, "pb/userservice/userservice.proto")
	mustContain(t, readmePath, "Review the generated proto contract before generating stubs")
	mustContain(t, readmePath, "generated from the current service contract and should be reviewed before running `protoc`")
	mustContain(t, readmePath, "Generated streaming SDK callbacks are synchronous")
	mustContain(t, readmePath, "slow `send` callback applies backpressure to local message delivery")
	mustContain(t, readmePath, "context deadlines/cancellation")
	mustContain(t, readmePath, "go run ./cmd")
}

// ─────────────────────────── 多服务 IDL ─────────────────────────────────

func TestGenerateFull_Readme_MultiServiceEndpoints(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "multi.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/multi",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	readmePath := filepath.Join(outDir, "README.md")
	mustContain(t, readmePath, "OrderService")
	mustContain(t, readmePath, "ProductService")
	mustContain(t, readmePath, "**PlaceOrder**: `POST /placeorder`")
	mustContain(t, readmePath, "**IncrStock**: `POST /incrstock`")
}

func TestGenerateFull_MultipleServices(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "multi.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/multi",
		DBDriver:   "sqlite",
		WithModel:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "service", "orderservice", "service.go"))
	mustExist(t, filepath.Join(outDir, "service", "productservice", "service.go"))
	mustExist(t, filepath.Join(outDir, "endpoint", "orderservice", "endpoints.go"))
	mustExist(t, filepath.Join(outDir, "endpoint", "productservice", "endpoints.go"))
	mustExist(t, filepath.Join(outDir, "transport", "orderservice", "transport_http.go"))
	mustExist(t, filepath.Join(outDir, "transport", "productservice", "transport_http.go"))
	mustExist(t, filepath.Join(outDir, "client", "orderservice", "demo.go"))
	mustExist(t, filepath.Join(outDir, "client", "productservice", "demo.go"))
	mustExist(t, filepath.Join(outDir, "sdk", "orderservicesdk", "client.go"))
	mustExist(t, filepath.Join(outDir, "sdk", "productservicesdk", "client.go"))
	mustExist(t, filepath.Join(outDir, "cmd", "main.go"))

	// Multi-service generation should keep the same layout contract as a
	// single-service project: one subtree per service, no special wrapper
	// directory or alternate "multi" structure.
	mustNotExist(t, filepath.Join(outDir, "services"))
	mustNotExist(t, filepath.Join(outDir, "multi"))
}

// ─────────────────────────── 仅有 Model（无 Service）─────────────────────

func TestGenerateFull_NoServiceIDL_ModelStillGenerated(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "noservice.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/noservice",
		DBDriver:   "sqlite",
		WithModel:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	// 没有 service 时 service/endpoint 目录可能存在（createDirStructure 总是创建），
	// 但不会生成具体的 service/xxx/service.go 文件。
	mustNotExist(t, filepath.Join(outDir, "service", "product", "service.go"))
	// model 有 gorm tag 时生成 model。
	mustExist(t, filepath.Join(outDir, "model", "generated_onlymodel.go"))
}

// ─────────────────────────── 各数据库驱动 DSN 验证 ─────────────────────

func TestGenerateFull_DBDriverDSN(t *testing.T) {
	cases := []struct {
		driver  string
		wantDSN string
	}{
		{"sqlite", "app.db"},
		{"mysql", "root:password@tcp"},
		{"postgres", "host=127.0.0.1"},
		{"sqlserver", "sqlserver://"},
		{"clickhouse", "tcp://127.0.0.1:9000"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.driver, func(t *testing.T) {
			outDir := newTmpDir(t)
			project := parseIDLProject(t, "basic.go")

			gen := mustNewGenerator(t, generator.Options{
				OutputDir:  outDir,
				ImportPath: "example.com/basic",
				DBDriver:   c.driver,
				WithDB:     true,
				WithConfig: false,
				WithDocs:   false,
			})
			if err := gen.GenerateIR(project); err != nil {
				t.Fatalf("GenerateIR: %v", err)
			}

			mainPath := filepath.Join(outDir, "cmd", "main.go")
			mustContain(t, mainPath, c.wantDSN)
		})
	}
}

// ─────────────────────────── generate 接口（旧 API 兼容）──────────────────

// ─────────────────────────── PackageName 命名转换 ─────────────────────────

func TestGenerateFull_PackageNameLowercased(t *testing.T) {
	outDir := newTmpDir(t)
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

	// 服务名 UserService 对应全小写 package userservice。
	mustExist(t, filepath.Join(outDir, "service", "userservice", "service.go"))

	content := readFile(t, filepath.Join(outDir, "service", "userservice", "service.go"))
	if !strings.Contains(content, "package userservice") {
		t.Error("service.go should declare 'package userservice'")
	}
}

// ─────────────────────────── OpenAPI contract ────────────────────────────────

// The generated embed wrapper and OpenAPI document are one owned contract.
func TestGenerateFull_OpenAPI_FullContent(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	docsPath := filepath.Join(outDir, "docs", "docs.go")
	mustExist(t, docsPath)
	openAPIPath := filepath.Join(outDir, "docs", "openapi.json")
	mustExist(t, openAPIPath)
	jsonSchemaPath := filepath.Join(outDir, "docs", "schema.json")
	mustExist(t, jsonSchemaPath)
	mustContain(t, docsPath, "package docs")

	mustContain(t, docsPath, "go:embed openapi.json")
	mustContain(t, docsPath, "go:embed schema.json")
	mustContain(t, docsPath, "func Handler")
	mustContain(t, docsPath, "func SchemaHandler")
	mustContain(t, openAPIPath, `"openapi": "3.1.0"`)
	mustContain(t, jsonSchemaPath, `"$defs"`)
	mustContain(t, jsonSchemaPath, `"ErrorResponse"`)
	mustContain(t, jsonSchemaPath, `"$ref": "#/$defs/User"`)
	mustNotContain(t, jsonSchemaPath, `#/components/schemas/`)

	mustNotContain(t, openAPIPath, `"jsonSchemaDialect"`)
	mustContain(t, openAPIPath, `"components"`)
	mustContain(t, openAPIPath, `"schemas"`)

	mustNotContain(t, docsPath, "swag.Register")
}

// The transport contains runtime adapters only; OpenAPI owns the description.
func TestGenerateFull_OpenAPI_IsNotDuplicatedInTransport(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	httpPath := filepath.Join(outDir, "transport", "userservice", "transport_http.go")

	mustNotContain(t, httpPath, "// @Summary")
	mustNotContain(t, httpPath, "// @Router")

	openAPIPath := filepath.Join(outDir, "docs", "openapi.json")
	mustContain(t, openAPIPath, `"requestBody"`)

	mustContain(t, openAPIPath, `"parameters"`)

	mustContain(t, openAPIPath, `"post"`)
	mustContain(t, openAPIPath, `"get"`)
	mustContain(t, openAPIPath, `"$ref": "#/components/schemas/ErrorResponse"`)
}

// Operations are generated from IR methods, including their HTTP verbs.
func TestGenerateFull_OpenAPI_ContainsMethodOperations(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	httpPath := filepath.Join(outDir, "transport", "userservice", "transport_http.go")
	content := readFile(t, httpPath)
	if strings.Contains(content, "// @Router") {
		t.Error("transport_http.go should not duplicate the OpenAPI route contract")
	}

	openAPIPath := filepath.Join(outDir, "docs", "openapi.json")
	mustContain(t, openAPIPath, `"operationId": "UserService_CreateUser"`)
	mustContain(t, openAPIPath, `"operationId": "UserService_GetUser"`)
	mustContain(t, openAPIPath, `"post"`)
	mustContain(t, openAPIPath, `"get"`)
}

// The generated runtime serves the contract and configures Swagger UI as a viewer.
func TestGenerateFull_OpenAPI_MainRoutes(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mainPath := filepath.Join(outDir, "cmd", "main.go")
	mustContain(t, mainPath, "/swagger/")
	mustContain(t, mainPath, `swaggerUI "github.com/swaggest/swgui/v5"`)
	mustContain(t, mainPath, `swaggerUI.New("UserService API", "/openapi.json", "/swagger/")`)
	mustContain(t, mainPath, "/openapi.json")
	mustContain(t, mainPath, "/schema.json")
	mustContain(t, mainPath, "docs.Handler")
	mustContain(t, mainPath, "docs.SchemaHandler")
	mustNotContain(t, mainPath, "swagger/doc.json")
	mustContain(t, filepath.Join(outDir, "go.mod"), "github.com/swaggest/swgui v1.8.9")
	typeScriptPath := filepath.Join(outDir, "sdk", "typescript", "client.ts")
	mustContain(t, typeScriptPath, "export class APIClient")
	mustContain(t, typeScriptPath, "export class UserServiceClient")
	mustContain(t, typeScriptPath, `let path = "/getuser"`)
	mustContain(t, typeScriptPath, "appendQueryValue")
}

// main.go does not carry a second annotation-based API contract.
func TestGenerateFull_OpenAPI_MainHasNoAnnotationContract(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mainPath := filepath.Join(outDir, "cmd", "main.go")
	mustNotContain(t, mainPath, "// @title")
	mustNotContain(t, mainPath, "// @version")
	mustNotContain(t, mainPath, "// @host")
	mustNotContain(t, mainPath, "// @BasePath")
}

// Relative OpenAPI URLs do not require a generated host setting.
func TestGenerateFull_OpenAPI_ConfigYAMLHasNoHost(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  true,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	configPath := filepath.Join(outDir, "config", "config.yaml")
	mustNotContain(t, configPath, "swagger_host")
}

// Generated config code has no host override for OpenAPI.
func TestGenerateFull_OpenAPI_ConfigCodeHasNoHost(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  true,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	codePath := filepath.Join(outDir, "config", "config.go")
	mustNotContain(t, codePath, "SwaggerHost")
}

// Reruns refresh generator-owned documentation instead of preserving stale output.
func TestGenerateFull_OpenAPI_RerunRefreshesGeneratedDocs(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	opts := generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  false,
		WithDocs:    false,
	}

	gen := mustNewGenerator(t, opts)
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("first GenerateIR: %v", err)
	}

	docsPath := filepath.Join(outDir, "docs", "docs.go")
	realDocs := `package docs

// Real Docs is stale generated output.
`
	if err := os.WriteFile(docsPath, []byte(realDocs), 0o644); err != nil {
		t.Fatalf("write real docs: %v", err)
	}

	gen2 := mustNewGenerator(t, opts)
	if err := gen2.GenerateIR(project); err != nil {
		t.Fatalf("second GenerateIR: %v", err)
	}

	content := readFile(t, docsPath)
	if strings.Contains(content, "Real Docs") {
		t.Error("generated docs.go should be refreshed by second generation")
	}
	mustContain(t, docsPath, "go:embed openapi.json")
	mustContain(t, filepath.Join(outDir, "docs", "openapi.json"), `"openapi": "3.1.0"`)
}

// Multi-service projects emit every unary operation into one OpenAPI document.
func TestGenerateFull_OpenAPI_MultiServiceOperations(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "multi.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/multi",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	for _, svcPkg := range []string{"orderservice", "productservice"} {
		httpPath := filepath.Join(outDir, "transport", svcPkg, "transport_http.go")
		mustExist(t, httpPath)
		mustNotContain(t, httpPath, "// @Summary")
		mustNotContain(t, httpPath, "// @Router")
	}
	openAPIPath := filepath.Join(outDir, "docs", "openapi.json")
	mustContain(t, openAPIPath, `"operationId": "OrderService_PlaceOrder"`)
	mustContain(t, openAPIPath, `"operationId": "ProductService_IncrStock"`)
}

// OpenAPI paths use the same configured service prefix as runtime routes.
func TestGenerateFull_OpenAPI_RoutePrefix(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
		WithConfig:  false,
		WithDocs:    false,
		RoutePrefix: "/api/v1",
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	openAPIPath := filepath.Join(outDir, "docs", "openapi.json")
	mustContain(t, openAPIPath, `"/api/v1/userservice`)
}

// Disabling OpenAPI output leaves no docs files or runtime documentation routes.
func TestGenerateFull_OpenAPI_DisabledDoesNotGenerateDocs(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithOpenAPI: false,
		WithConfig:  false,
		WithDocs:    false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustNotExist(t, filepath.Join(outDir, "docs", "docs.go"))
	mustNotExist(t, filepath.Join(outDir, "docs", "openapi.json"))
	mustNotExist(t, filepath.Join(outDir, "docs", "schema.json"))
	mustNotExist(t, filepath.Join(outDir, "sdk", "typescript", "client.ts"))

	mainPath := filepath.Join(outDir, "cmd", "main.go")
	content := readFile(t, mainPath)
	if strings.Contains(content, "swaggerUI") {
		t.Error("main.go should not contain swaggerUI when WithOpenAPI=false")
	}
	mustNotContain(t, mainPath, `r.HandleFunc("GET /openapi.json"`)
	mustNotContain(t, mainPath, `r.HandleFunc("GET /schema.json"`)
}
