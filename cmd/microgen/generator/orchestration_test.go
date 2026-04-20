package generator_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
)

func TestGenerateFull_CopiesGoIDLSource(t *testing.T) {
	outDir := newTmpDir(t)
	idlPath := filepath.Join("..", "parser", "testdata", "basic.go")
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		IDLSrcPath: idlPath,
		WithDocs:   false,
		WithConfig: false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	copiedPath := filepath.Join(outDir, "idl.go")
	mustExist(t, copiedPath)

	original, err := os.ReadFile(idlPath)
	if err != nil {
		t.Fatalf("ReadFile original: %v", err)
	}
	copied, err := os.ReadFile(copiedPath)
	if err != nil {
		t.Fatalf("ReadFile copied: %v", err)
	}
	if string(copied) != string(original) {
		t.Fatal("copied idl.go content did not match source")
	}
}

func TestGenerateFull_DoesNotCopyProtoSourceAsIDLGo(t *testing.T) {
	outDir := newTmpDir(t)
	protoPath := filepath.Join(outDir, "svc.proto")
	if err := os.WriteFile(protoPath, []byte(greeterProto), 0o644); err != nil {
		t.Fatalf("WriteFile proto: %v", err)
	}
	project := parseProtoProject(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		DBDriver:   "sqlite",
		IDLSrcPath: protoPath,
		WithDocs:   false,
		WithConfig: false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustNotExist(t, filepath.Join(outDir, "idl.go"))
}

func TestGenerateFull_GoMod_UsesTestdataRelativeReplacePath(t *testing.T) {
	base := filepath.Join(t.TempDir(), "testdata", "gen")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  base,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithDocs:   false,
		WithConfig: false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustContain(t, filepath.Join(base, "go.mod"), "replace github.com/dreamsxin/go-kit => ../../../")
}

func TestGenerateFull_RoutePrefix_AlignedAcrossArtifacts(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		DBDriver:    "sqlite",
		RoutePrefix: "/api/v2",
		WithDocs:    false,
		WithConfig:  false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	expectedPrefix := "/api/v2/userservice"
	mustContain(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"), expectedPrefix)
	mustContain(t, filepath.Join(outDir, "cmd", "generated_routes.go"), expectedPrefix)
}

func TestGenerateFull_WithSwag_GeneratesDocsStubAtConventionalPath(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithSwag:   true,
		WithDocs:   false,
		WithConfig: false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	docsPath := filepath.Join(outDir, "docs", "docs.go")
	mustExist(t, docsPath)
	mustContain(t, docsPath, "package docs")
}

func TestGenerateFull_FromProtoGRPC_GeneratesConventionalProtoAndClientArtifacts(t *testing.T) {
	outDir := newTmpDir(t)
	protoPath := filepath.Join(outDir, "svc.proto")
	if err := os.WriteFile(protoPath, []byte(greeterProto), 0o644); err != nil {
		t.Fatalf("WriteFile proto: %v", err)
	}
	project := parseProtoProject(t, greeterProto)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		IDLSrcPath: protoPath,
		WithDocs:   false,
		WithConfig: false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "pb", "greeter", "greeter.proto"))
	mustExist(t, filepath.Join(outDir, "client", "greeter", "demo.go"))
	mustNotExist(t, filepath.Join(outDir, "idl.go"))
}

func TestGenerateFull_FullFeatureSet_GeneratesArtifactsAcrossAllPhases(t *testing.T) {
	outDir := newTmpDir(t)
	idlPath := filepath.Join("..", "parser", "testdata", "basic.go")
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/basic",
		Protocols:   []string{"http", "grpc"},
		DBDriver:    "sqlite",
		IDLSrcPath:  idlPath,
		RoutePrefix: "/api/v3",
		WithConfig:  true,
		WithDocs:    true,
		WithTests:   true,
		WithModel:   true,
		WithSwag:    true,
		WithSkill:   true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "go.mod"))
	mustExist(t, filepath.Join(outDir, "idl.go"))
	mustExist(t, filepath.Join(outDir, "model", "generated_user.go"))
	mustExist(t, filepath.Join(outDir, "repository", "generated_user_repository.go"))
	mustExist(t, filepath.Join(outDir, "repository", "generated_base.go"))
	mustExist(t, filepath.Join(outDir, "service", "userservice", "service.go"))
	mustExist(t, filepath.Join(outDir, "endpoint", "userservice", "endpoints.go"))
	mustExist(t, filepath.Join(outDir, "endpoint", "userservice", "generated_chain.go"))
	mustExist(t, filepath.Join(outDir, "endpoint", "userservice", "custom_chain.go"))
	mustExist(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"))
	mustExist(t, filepath.Join(outDir, "transport", "userservice", "transport_grpc.go"))
	mustExist(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"))
	mustExist(t, filepath.Join(outDir, "client", "userservice", "demo.go"))
	mustExist(t, filepath.Join(outDir, "sdk", "userservicesdk", "client.go"))
	mustExist(t, filepath.Join(outDir, "test", "userservice_test.go"))
	mustExist(t, filepath.Join(outDir, "cmd", "main.go"))
	mustExist(t, filepath.Join(outDir, "cmd", "generated_runtime.go"))
	mustExist(t, filepath.Join(outDir, "cmd", "generated_services.go"))
	mustExist(t, filepath.Join(outDir, "cmd", "generated_routes.go"))
	mustExist(t, filepath.Join(outDir, "cmd", "custom_routes.go"))
	mustExist(t, filepath.Join(outDir, "config", "config.yaml"))
	mustExist(t, filepath.Join(outDir, "config", "config.go"))
	mustExist(t, filepath.Join(outDir, "README.md"))
	mustExist(t, filepath.Join(outDir, "docs", "docs.go"))
	mustExist(t, filepath.Join(outDir, "skill", "skill.go"))

	expectedPrefix := "/api/v3/userservice"
	mustContain(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"), expectedPrefix)
	mustContain(t, filepath.Join(outDir, "cmd", "generated_routes.go"), expectedPrefix)
	mustContain(t, filepath.Join(outDir, "docs", "docs.go"), "package docs")
	mustContain(t, filepath.Join(outDir, "skill", "skill.go"), "CreateUser")
}
