package generator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
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

// newTmpDir 返回临时目录，测试结束自动清理�?
func newTmpDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// mustExist 断言路径存在�?
func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected path to exist: %s", path)
	}
}

// mustNotExist 断言路径不存在�?
func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("expected path NOT to exist: %s", path)
	}
}

// readFile 读取文件内容，失败时 Fatal�?
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	return string(b)
}

// mustContain 断言文件内容包含指定子串�?
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

// mustNewGenerator 创建 Generator，失败时 Fatal�?
func mustNewGenerator(t *testing.T, opts generator.Options) *generator.Generator {
	t.Helper()
	// 未指�?TemplateFS 时使用测试全局 FS
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

	// HTTP only �?不应生成 gRPC 相关
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

func TestGenerateFull_DirectoryStructure_WithSwag(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   false,
		WithSwag:   true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "docs", "docs.go"))
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
}

// TestGenerateFull_GoMod_ModuleUpdatedWhenMismatch 验证：go.mod 已存在但 module 名与 -import 不符时，
// generator 只更�?module 行，其余内容（go 版本、require 块等）保持不变�?
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
	// go 版本行应保留（其余内容未丢失�?	mustContain(t, goModPath, "go 1.22")
}

// TestGenerateFull_GoMod_SkippedWhenModuleMatches 验证：go.mod 已存在且 module 名与 -import 一致时�?// 整个文件内容不被改动（用户的自定�?require 等不丢失）�?
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
	mustContain(t, configPath, "remote:")
	mustContain(t, configPath, "fallback_to_local: true")
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
	mustExist(t, codePath)
	mustContain(t, codePath, "type Config struct")
	mustContain(t, codePath, "func Load(path string)")
	mustContain(t, codePath, "func LoadLocal(path string)")
	mustContain(t, codePath, "func ApplyEnv(cfg *Config) error")
	mustContain(t, codePath, "func LoadRemote(cfg *Config) (*Config, error)")
	mustContain(t, codePath, "func Default()")
	mustContain(t, codePath, `yaml:"server"`)
	mustContain(t, codePath, "type RemoteConfig struct")
	mustContain(t, codePath, `yaml:"remote"`)
}

func TestGenerateFull_ConfigCode_EnvOverrides(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "mysql",
		WithDB:     true,
		WithSwag:   true,
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	codePath := filepath.Join(outDir, "config", "config.go")
	mustContain(t, codePath, `const envPrefix = "APP_"`)
	mustContain(t, codePath, `readString("HTTP_ADDR"`)
	mustContain(t, codePath, `readString("GRPC_ADDR"`)
	mustContain(t, codePath, `readString("LOG_LEVEL"`)
	mustContain(t, codePath, `readString("DB_DSN"`)
	mustContain(t, codePath, `readString("SWAGGER_HOST"`)
	mustContain(t, codePath, `readBool("DEBUG_ROUTES_ENABLED"`)
	mustContain(t, codePath, `readBool("REMOTE_ENABLED"`)
	mustContain(t, codePath, `readString("REMOTE_PROVIDER"`)
	mustContain(t, codePath, `readDuration("REMOTE_TIMEOUT"`)
	mustContain(t, codePath, `readBool("REMOTE_FALLBACK_TO_LOCAL"`)
	mustContain(t, codePath, "strconv.ParseBool")
	mustContain(t, codePath, "strconv.Atoi")
	mustContain(t, codePath, "strconv.ParseFloat")
	mustContain(t, codePath, "time.ParseDuration")
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
	mustContain(t, codePath, "Enabled         bool")
	mustContain(t, codePath, "Provider        string")
	mustContain(t, codePath, "DataID          string")
	mustContain(t, codePath, "FallbackToLocal bool")
	mustContain(t, codePath, "Timeout:         5 * time.Second")
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
	mustContain(t, codePath, `"mysql"`) // Default() 中包�?Driver: "mysql"
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
}

// ─────────────────────────── docs/docs.go ─────────────────────────────────

func TestGenerateFull_DocsStub_Contents(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	docsPath := filepath.Join(outDir, "docs", "docs.go")
	mustContain(t, docsPath, "package docs")
	mustContain(t, docsPath, "SwaggerInfo")
	mustContain(t, docsPath, "swag.Register")
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
	mustContain(t, readmePath, "go run ./cmd/main.go")
	mustNotContain(t, readmePath, "protoc --go_out=.")
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
	mustContain(t, readmePath, "go run ./cmd/main.go")
}

// ─────────────────────────── 多服�?IDL ─────────────────────────────────

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

	// �?service �?service/endpoint 目录可能存在（createDirStructure 总是创建），
	// 但不会有具体�?service/xxx/service.go 文件
	mustNotExist(t, filepath.Join(outDir, "service", "product", "service.go"))
	// model �?gorm tag �?生成 model
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

	// 服务�?UserService �?package �?userservice（全小写�?	mustExist(t, filepath.Join(outDir, "service", "userservice", "service.go"))

	content := readFile(t, filepath.Join(outDir, "service", "userservice", "service.go"))
	if !strings.Contains(content, "package userservice") {
		t.Error("service.go should declare 'package userservice'")
	}
}

// ─────────────────────────── Swagger / swag 文档 ─────────────────────────────

// TestGenerateFull_Swag_DocsStub_FullContent 验证 docs/docs.go 的完整内容：
// - package 声明
// - SwaggerInfo 变量（含 BasePath、Title、Version�?// - swag.Register 调用
// - docTemplate 包含 swagger 2.0 结构
func TestGenerateFull_Swag_DocsStub_FullContent(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	docsPath := filepath.Join(outDir, "docs", "docs.go")
	mustExist(t, docsPath)

	// 结构性内�?	mustContain(t, docsPath, "package docs")
	mustContain(t, docsPath, "SwaggerInfo")
	mustContain(t, docsPath, "swag.Register")
	mustContain(t, docsPath, `"swagger": "2.0"`)

	// SwaggerInfo 字段
	mustContain(t, docsPath, `Version:`)
	mustContain(t, docsPath, `BasePath:`)
	mustContain(t, docsPath, `Title:`)

	// init() 注册
	mustContain(t, docsPath, "func init()")
}

// TestGenerateFull_Swag_TransportAnnotations 验证 transport_http.go 中的 swag 注释�?// - @Summary、@Description、@Tags
// - @Param（GET �?query，POST �?body�?// - @Success、@Failure
// - @Router（含正确�?HTTP 方法�?
func TestGenerateFull_Swag_TransportAnnotations(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	httpPath := filepath.Join(outDir, "transport", "userservice", "transport_http.go")

	// 每个方法都应�?swag 注释
	mustContain(t, httpPath, "// @Summary")
	mustContain(t, httpPath, "// @Tags")
	mustContain(t, httpPath, "// @Accept       json")
	mustContain(t, httpPath, "// @Produce      json")
	mustContain(t, httpPath, "// @Success      200")
	mustContain(t, httpPath, "// @Failure      400")
	mustContain(t, httpPath, "// @Failure      500")

	// POST 方法�?body 参数
	mustContain(t, httpPath, `// @Param        request  body`)

	// GET 方法�?query 参数（ListUsers、GetUser 等）
	mustContain(t, httpPath, `// @Param        request  query`)

	// @Router 注释包含路由路径�?HTTP 方法
	mustContain(t, httpPath, "// @Router")
	mustContain(t, httpPath, "[post]")
	mustContain(t, httpPath, "[get]")
}

// TestGenerateFull_Swag_RouterAnnotations_Methods 验证各方法的 @Router 注释
// 包含正确�?HTTP 方法标记�?
func TestGenerateFull_Swag_RouterAnnotations_Methods(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	httpPath := filepath.Join(outDir, "transport", "userservice", "transport_http.go")
	content := readFile(t, httpPath)

	// CreateUser �?POST
	if !strings.Contains(content, "// @Router") {
		t.Error("transport_http.go should contain @Router annotations")
	}

	// 验证 CreateUser �?@Router 包含 [post]
	lines := strings.Split(content, "\n")
	routerLines := []string{}
	for _, l := range lines {
		if strings.Contains(l, "// @Router") {
			routerLines = append(routerLines, strings.TrimSpace(l))
		}
	}
	if len(routerLines) == 0 {
		t.Fatal("no @Router annotations found")
	}

	// 至少有一�?[post] 和一�?[get]
	hasPost, hasGet := false, false
	for _, l := range routerLines {
		if strings.Contains(l, "[post]") {
			hasPost = true
		}
		if strings.Contains(l, "[get]") {
			hasGet = true
		}
	}
	if !hasPost {
		t.Errorf("expected at least one [post] @Router, got: %v", routerLines)
	}
	if !hasGet {
		t.Errorf("expected at least one [get] @Router, got: %v", routerLines)
	}
}

// TestGenerateFull_Swag_MainFile_SwaggerRoute 验证 main.go 包含 Swagger UI 路由�?
func TestGenerateFull_Swag_MainFile_SwaggerRoute(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mainPath := filepath.Join(outDir, "cmd", "main.go")
	mustContain(t, mainPath, "/swagger/")
	mustContain(t, mainPath, "httpSwagger")
	mustContain(t, mainPath, "swagger/doc.json")
}

// TestGenerateFull_Swag_MainFile_SwaggerAnnotations 验证 main.go 顶部�?swag 全局注释�?
func TestGenerateFull_Swag_MainFile_SwaggerAnnotations(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mainPath := filepath.Join(outDir, "cmd", "main.go")
	mustContain(t, mainPath, "// @title")
	mustContain(t, mainPath, "// @version")
	mustContain(t, mainPath, "// @host")
	mustContain(t, mainPath, "// @BasePath")
}

// TestGenerateFull_Swag_ConfigYAML_SwaggerHost 验证 config.yaml 包含 swagger_host 字段�?
func TestGenerateFull_Swag_ConfigYAML_SwaggerHost(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	configPath := filepath.Join(outDir, "config", "config.yaml")
	mustContain(t, configPath, "swagger_host")
}

// TestGenerateFull_Swag_ConfigCode_SwaggerHost 验证 config.go 包含 SwaggerHost 字段�?
func TestGenerateFull_Swag_ConfigCode_SwaggerHost(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithConfig: true,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	codePath := filepath.Join(outDir, "config", "config.go")
	mustContain(t, codePath, "SwaggerHost")
}

// TestGenerateFull_Swag_DocsStub_NotOverwrittenBySecondRun 验证�?// �?docs.go 已存在且不是 stub（不�?"paths": {}），第二次生成不会覆盖它�?
func TestGenerateFull_Swag_DocsStub_NotOverwrittenBySecondRun(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	opts := generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithConfig: false,
		WithDocs:   false,
	}

	// First generation run.
	gen := mustNewGenerator(t, opts)
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("first GenerateIR: %v", err)
	}

	docsPath := filepath.Join(outDir, "docs", "docs.go")
	realDocs := `package docs

// This is a real swag-generated file.
var SwaggerInfo = &swag.Spec{
    Version: "2.0",
    Title:   "Real Docs",
}
`
	if err := os.WriteFile(docsPath, []byte(realDocs), 0o644); err != nil {
		t.Fatalf("write real docs: %v", err)
	}

	gen2 := mustNewGenerator(t, opts)
	if err := gen2.GenerateIR(project); err != nil {
		t.Fatalf("second GenerateIR: %v", err)
	}

	content := readFile(t, docsPath)
	if !strings.Contains(content, "Real Docs") {
		t.Error("real docs.go should not be overwritten by second generation")
	}
}

// TestGenerateFull_Swag_MultiService_AllAnnotated 验证多服�?IDL 时，
// 每个服务�?transport_http.go 都包�?swag 注释�?
func TestGenerateFull_Swag_MultiService_AllAnnotated(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "multi.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/multi",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	for _, svcPkg := range []string{"orderservice", "productservice"} {
		httpPath := filepath.Join(outDir, "transport", svcPkg, "transport_http.go")
		mustExist(t, httpPath)
		mustContain(t, httpPath, "// @Summary")
		mustContain(t, httpPath, "// @Router")
	}
}

// TestGenerateFull_Swag_RoutePrefix_InAnnotations 验证使用 -prefix 时，
// @Router 注释包含正确的前缀路径�?
func TestGenerateFull_Swag_RoutePrefix_InAnnotations(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		WithSwag:    true,
		WithConfig:  false,
		WithDocs:    false,
		RoutePrefix: "/api/v1",
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	httpPath := filepath.Join(outDir, "transport", "userservice", "transport_http.go")
	mustContain(t, httpPath, "/api/v1/userservice")
}

// TestGenerateFull_Swag_WithoutSwag_NoAnnotations 验证不启�?-swag 时，
// transport_http.go 不包�?swag 注释（避免误导）�?// 注意：当前模板始终生�?swag 注释，此测试验证现有行为�?
func TestGenerateFull_Swag_WithoutSwag_DocsNotGenerated(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   false,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	// docs/ 目录不应存在
	mustNotExist(t, filepath.Join(outDir, "docs", "docs.go"))

	// main.go 不应包含 swagger 路由
	mainPath := filepath.Join(outDir, "cmd", "main.go")
	content := readFile(t, mainPath)
	if strings.Contains(content, "httpSwagger") {
		t.Error("main.go should not contain httpSwagger when WithSwag=false")
	}
}
