package generator_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/generator"
)

func TestGenerateIR_WritesDeterministicProjectManifest(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")
	idlPath := filepath.Join("..", "parser", "testdata", "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:            outDir,
		ImportPath:           "example.com/manifest",
		IDLSrcPath:           idlPath,
		DBDriver:             "sqlite",
		WithConfig:           true,
		ConfigMode:           "hybrid",
		RemoteProvider:       "consul",
		WithDocs:             true,
		WithTests:            true,
		WithModel:            true,
		WithGRPC:             true,
		WithDB:               true,
		WithOpenAPI:          true,
		WithSkill:            true,
		WithInteraction:      true,
		RoutePrefix:          "/api/v2",
		GeneratedMiddlewares: []string{"metrics", "tracing"},
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	manifest := readGeneratedManifest(t, outDir)
	if manifest.SchemaVersion != generator.ProjectManifestSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", manifest.SchemaVersion, generator.ProjectManifestSchemaVersion)
	}
	if manifest.ModulePath != "example.com/manifest" || manifest.Source != "go" || manifest.RoutePrefix != "/api/v2" {
		t.Fatalf("manifest identity = %+v", manifest)
	}
	if !slices.Equal(manifest.Services, []string{"UserService"}) || !slices.Equal(manifest.Models, []string{"User"}) {
		t.Fatalf("manifest services/models = %v/%v", manifest.Services, manifest.Models)
	}
	if !slices.Equal(manifest.GeneratedMiddlewares, []string{"metrics", "tracing"}) {
		t.Fatalf("GeneratedMiddlewares = %v, want [metrics tracing]", manifest.GeneratedMiddlewares)
	}
	capabilities := manifest.Capabilities
	if !capabilities.Config || !capabilities.Docs || !capabilities.Tests || !capabilities.Model || !capabilities.GRPC || !capabilities.Database || !capabilities.OpenAPI || !capabilities.Skill || !capabilities.Interaction {
		t.Fatalf("manifest capabilities = %+v, want all enabled", capabilities)
	}
	if capabilities.ConfigMode != "hybrid" || capabilities.RemoteProvider != "consul" || capabilities.DatabaseDriver != "sqlite" {
		t.Fatalf("manifest capability configuration = %+v", capabilities)
	}
	if !slices.IsSorted(manifest.Artifacts) {
		t.Fatalf("manifest artifacts are not sorted: %v", manifest.Artifacts)
	}
	for _, want := range []string{
		".ai/PROJECT_GUIDE.md",
		".microgen/manifest.json",
		"cmd/generated_interaction.go",
		"config/loader.go",
		"docs/openapi.json",
		"docs/schema.json",
		"endpoint/userservice/generated_chain.go",
		"idl.go",
		"model/generated_user.go",
		"pb/userservice/userservice.proto",
		"repository/generated_user_repository.go",
		"sdk/typescript/client.ts",
		"service/userservice/generated_repos.go",
		"test/userservice_test.go",
	} {
		if !slices.Contains(manifest.Artifacts, want) {
			t.Fatalf("manifest artifacts missing %q: %v", want, manifest.Artifacts)
		}
	}

	existing, err := generator.ScanExistingProject(outDir)
	if err != nil {
		t.Fatalf("ScanExistingProject: %v", err)
	}
	if existing.Manifest == nil || len(existing.ManifestDrift) != 0 {
		t.Fatalf("manifest scan = %+v, drift = %v", existing.Manifest, existing.ManifestDrift)
	}
	if !existing.Features.WithInteraction || existing.Features.ConfigMode != "hybrid" || existing.Features.DBDriver != "sqlite" || existing.Features.RoutePrefix != "/api/v2" {
		t.Fatalf("features from manifest = %+v", existing.Features)
	}
}

func TestScanExistingProject_ReportsManifestArtifactDrift(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")
	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/drift",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}
	if err := os.Remove(filepath.Join(outDir, "docs", "schema.json")); err != nil {
		t.Fatalf("Remove(schema.json): %v", err)
	}

	existing, err := generator.ScanExistingProject(outDir)
	if err != nil {
		t.Fatalf("ScanExistingProject: %v", err)
	}
	if !containsSubstring(existing.ManifestDrift, "manifest artifact is missing: docs/schema.json") {
		t.Fatalf("ManifestDrift = %v, want missing schema", existing.ManifestDrift)
	}
}

func TestScanExistingProject_RejectsManifestModuleMismatch(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")
	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/original",
		DBDriver:   "sqlite",
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	manifest := readGeneratedManifest(t, outDir)
	manifest.ModulePath = "example.com/other"
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(outDir, ".microgen", "manifest.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest): %v", err)
	}

	_, err = generator.ScanExistingProject(outDir)
	if err == nil || !strings.Contains(err.Error(), "does not match go.mod module") {
		t.Fatalf("ScanExistingProject error = %v, want module mismatch", err)
	}
}

func TestScanExistingProject_RejectsInvalidManifestServiceName(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")
	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/invalid-name",
		DBDriver:   "sqlite",
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	manifest := readGeneratedManifest(t, outDir)
	manifest.Services = []string{"../escape"}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(outDir, ".microgen", "manifest.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest): %v", err)
	}

	_, err = generator.ScanExistingProject(outDir)
	if err == nil || !strings.Contains(err.Error(), "service name must be a Go identifier") {
		t.Fatalf("ScanExistingProject error = %v, want invalid service name", err)
	}
}

func TestApplyAppendMiddleware_RejectsManifestDrift(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")
	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   outDir,
		ImportPath:  "example.com/drifted-extend",
		DBDriver:    "sqlite",
		WithOpenAPI: true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}
	if err := os.Remove(filepath.Join(outDir, "docs", "schema.json")); err != nil {
		t.Fatalf("Remove(schema.json): %v", err)
	}

	idlPath := filepath.Join("..", "parser", "testdata", "basic.go")
	_, err := generator.ApplyAppendMiddleware(testTemplateFS, outDir, project, generator.ExtendOptions{
		AppendMiddleware: []string{"tracing"},
	}, idlPath)
	if err == nil || !strings.Contains(err.Error(), "project manifest has drift") {
		t.Fatalf("ApplyAppendMiddleware error = %v, want manifest drift refusal", err)
	}
}

func readGeneratedManifest(t *testing.T, root string) generator.ProjectManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".microgen", "manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile(manifest): %v", err)
	}
	var manifest generator.ProjectManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal(manifest): %v", err)
	}
	return manifest
}

func containsSubstring(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}
