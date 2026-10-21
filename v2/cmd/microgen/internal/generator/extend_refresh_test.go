package generator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/internal/generator"
	"github.com/dreamsxin/go-kit/v2/cmd/microgen/internal/ir"
)

const refreshIDL = `package shop

import "context"

type HelloRequest struct {
	Name string ` + "`" + `json:"name" validate:"minLen=1"` + "`" + `
}

type HelloResponse struct {
	Message string ` + "`" + `json:"message"` + "`" + `
}

type HelloService interface {
	SayHello(context.Context, HelloRequest) (HelloResponse, error)
}
`

// refreshFixture generates a one-service project and marks the user-owned
// service file with an application marker, the way a real project looks
// before its first extend.
func refreshFixture(t *testing.T, root string) {
	t.Helper()
	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   root,
		ImportPath:  "example.com/shop",
		Protocols:   []string{"http"},
		WithDocs:    true,
		WithOpenAPI: true,
		IDLSrcPath:  filepath.Join(root, "idl.go"),
	})
	if err := os.WriteFile(filepath.Join(root, "idl.go"), []byte(refreshIDL), 0o644); err != nil {
		t.Fatalf("write contract copy: %v", err)
	}
	project := parseIDLContentProject(t, refreshIDL)
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	servicePath := filepath.Join(root, "service", "helloservice", "service.go")
	data, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("read service file: %v", err)
	}
	marked := strings.Replace(string(data),
		`return idl.HelloResponse{}, errors.New("SayHello: not implemented")`,
		`return idl.HelloResponse{Message: "user logic"}, nil // user marker`,
		1)
	if err := os.WriteFile(servicePath, []byte(marked), 0o644); err != nil {
		t.Fatalf("mark service file: %v", err)
	}
}

const refreshIDLWithFarewell = `package shop

import "context"

type HelloRequest struct {
	Name string ` + "`" + `json:"name" validate:"minLen=1"` + "`" + `
}

type FarewellRequest struct {
	Note string ` + "`" + `json:"note" validate:"maxLen=8"` + "`" + `
}

type FarewellResponse struct {
	Message string ` + "`" + `json:"message"` + "`" + `
}

type HelloService interface {
	SayHello(context.Context, HelloRequest) (HelloResponse, error)
	Farewell(context.Context, FarewellRequest) (FarewellResponse, error)
}
`

// TestApplyRefreshUpdatesGeneratedArtifacts: extending a service with a new
// method refreshes every generated surface that mentions it, while the
// user's business logic survives untouched.
func TestApplyRefreshUpdatesGeneratedArtifacts(t *testing.T) {
	t.Parallel()
	root := newTmpDir(t)
	refreshFixture(t, root)

	// The user edits the project's own contract copy in place — the same file
	// the next extend without -idl reads.
	if err := os.WriteFile(filepath.Join(root, "idl.go"), []byte(refreshIDLWithFarewell), 0o644); err != nil {
		t.Fatalf("edit contract copy: %v", err)
	}
	project := parseIDLContentProject(t, refreshIDLWithFarewell)
	result, err := generator.ApplyRefresh(testTemplateFS, root, project, filepath.Join(root, "idl.go"))
	if err != nil {
		t.Fatalf("ApplyRefresh: %v", err)
	}
	if len(result.Plan.NewFiles) != 0 {
		t.Errorf("new files = %v, want none: a method addition creates nothing", result.Plan.NewFiles)
	}
	if len(result.Plan.UpdatedFiles) == 0 {
		t.Fatal("no files were updated; the new method did not reach any artifact")
	}
	if len(result.Plan.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", result.Plan.Warnings)
	}
	for _, file := range result.Plan.UpdatedFiles {
		t.Logf("updated: %s", file.Path)
	}

	mustContain(t, filepath.Join(root, "endpoint", "helloservice", "endpoints.go"), "FarewellEndpoint")
	mustContain(t, filepath.Join(root, "docs", "openapi.json"), "/farewell")
	mustContain(t, filepath.Join(root, "sdk", "typescript", "client.ts"), "Farewell")
	mustContain(t, filepath.Join(root, "idl.go"), "FarewellRequest")

	service := readFile(t, filepath.Join(root, "service", "helloservice", "service.go"))
	if !strings.Contains(service, "user marker") {
		t.Error("the user's business logic was overwritten by the refresh")
	}
	if strings.Contains(service, "Farewell") {
		t.Error("the user-owned service file gained generated content; the refresh must leave it for the compiler to guide")
	}
}

// TestApplyRefreshReportsObsoleteArtifacts: a contract that stops producing a
// model warns about the artifacts it no longer declares and leaves them in
// place — deletion is a decision.
func TestApplyRefreshReportsObsoleteArtifacts(t *testing.T) {
	t.Parallel()
	root := newTmpDir(t)

	modelFixture := `package shop

import "context"

type HelloRequest struct {
	Name string ` + "`" + `json:"name"` + "`" + `
}

type HelloResponse struct {
	Message string ` + "`" + `json:"message"` + "`" + `
}

type Product struct {
	ID   uint   ` + "`" + `json:"id" gorm:"primaryKey"` + "`" + `
	Name string ` + "`" + `json:"name" gorm:"column:name;not null"` + "`" + `
}

type HelloService interface {
	SayHello(context.Context, HelloRequest) (HelloResponse, error)
}
`
	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  root,
		ImportPath: "example.com/shop",
		Protocols:  []string{"http"},
		WithModel:  true,
		WithDB:     true,
		DBDriver:   "sqlite",
		WithDocs:   true,
		IDLSrcPath: filepath.Join(root, "idl.go"),
	})
	if err := os.WriteFile(filepath.Join(root, "idl.go"), []byte(modelFixture), 0o644); err != nil {
		t.Fatalf("write contract copy: %v", err)
	}
	if err := gen.GenerateIR(parseIDLContentProject(t, modelFixture)); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}
	mustExist(t, filepath.Join(root, "model", "generated_product.go"))

	project := parseIDLContentProject(t, refreshIDL)
	result, err := generator.ApplyRefresh(testTemplateFS, root, project, filepath.Join(root, "idl.go"))
	if err != nil {
		t.Fatalf("ApplyRefresh: %v", err)
	}
	joined := strings.Join(result.Plan.Warnings, "\n")
	for _, obsolete := range []string{"model/generated_product.go", "repository/generated_product_repository.go"} {
		if !strings.Contains(joined, obsolete) {
			t.Errorf("warnings do not name the obsolete artifact %s:\n%s", obsolete, joined)
		}
	}
	mustExist(t, filepath.Join(root, "model", "generated_product.go"))
}

// TestApplyRefreshRejectsNonGoSource: the refresh cannot regenerate from a
// proto source, because the contract copy is a Go file.
func TestApplyRefreshRejectsNonGoSource(t *testing.T) {
	t.Parallel()
	root := newTmpDir(t)
	refreshFixture(t, root)

	project := &ir.Project{}
	if _, err := generator.ApplyRefresh(testTemplateFS, root, project, "service.proto"); err == nil {
		t.Fatal("a proto source was accepted for refresh")
	}
}
