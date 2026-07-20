package generator

import (
	"encoding/json"
	"fmt"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const ProjectManifestSchemaVersion = "microgen.project.v2"

// ProjectManifest records the generator-owned identity and capabilities of a
// generated project. Extend mode treats it as the primary configuration source.
type ProjectManifest struct {
	SchemaVersion        string                      `json:"schemaVersion"`
	ModulePath           string                      `json:"modulePath"`
	Source               string                      `json:"source"`
	RoutePrefix          string                      `json:"routePrefix,omitempty"`
	Capabilities         ProjectManifestCapabilities `json:"capabilities"`
	Services             []string                    `json:"services"`
	Models               []string                    `json:"models"`
	GeneratedMiddlewares []string                    `json:"generatedMiddlewares"`
	Artifacts            []string                    `json:"artifacts"`
}

// ProjectManifestCapabilities contains options required to regenerate
// generator-owned artifacts without inferring configuration from Go source.
type ProjectManifestCapabilities struct {
	Config         bool   `json:"config"`
	Docs           bool   `json:"docs"`
	Tests          bool   `json:"tests"`
	Model          bool   `json:"model"`
	GRPC           bool   `json:"grpc"`
	Database       bool   `json:"database"`
	OpenAPI        bool   `json:"openapi"`
	Interaction    bool   `json:"interaction"`
	ConfigMode     string `json:"configMode,omitempty"`
	RemoteProvider string `json:"remoteProvider,omitempty"`
	DatabaseDriver string `json:"databaseDriver,omitempty"`
}

func (g *Generator) generateProjectManifest(ctx generationContext) error {
	manifest := g.buildProjectManifest(ctx)
	if err := writeJSONDocument(g.layout.manifestFile(), manifest); err != nil {
		return fmt.Errorf("write project manifest: %w", err)
	}
	return nil
}

func (g *Generator) buildProjectManifest(ctx generationContext) ProjectManifest {
	services := make([]string, 0, len(ctx.services))
	for _, service := range ctx.services {
		services = append(services, service.ServiceName)
	}
	models := make([]string, 0, len(ctx.models))
	if g.config.WithModel {
		for _, model := range ctx.models {
			models = append(models, model.Name)
		}
	}
	sort.Strings(services)
	sort.Strings(models)

	middlewares := normalizeGeneratedMiddlewares(g.config.GeneratedMiddlewares)
	if middlewares == nil {
		middlewares = []string{}
	}
	databaseDriver := ""
	if g.config.WithDB {
		databaseDriver = g.config.DBDriver
	}

	manifest := ProjectManifest{
		SchemaVersion: ProjectManifestSchemaVersion,
		ModulePath:    g.config.ImportPath,
		Source:        manifestSource(ctx.source),
		RoutePrefix:   g.config.RoutePrefix,
		Capabilities: ProjectManifestCapabilities{
			Config:         g.config.WithConfig,
			Docs:           g.config.WithDocs,
			Tests:          g.config.WithTests,
			Model:          g.config.WithModel,
			GRPC:           g.config.WithGRPC,
			Database:       g.config.WithDB,
			OpenAPI:        g.config.WithOpenAPI,
			Interaction:    g.config.WithInteraction,
			ConfigMode:     g.config.ConfigMode,
			RemoteProvider: g.config.RemoteProvider,
			DatabaseDriver: databaseDriver,
		},
		Services:             services,
		Models:               models,
		GeneratedMiddlewares: middlewares,
	}
	manifest.Artifacts = g.manifestArtifacts(ctx)
	return manifest
}

func manifestSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return "ir"
	}
	return source
}

func (g *Generator) manifestArtifacts(ctx generationContext) []string {
	paths := []string{
		g.layout.manifestFile(),
		g.layout.cmdGeneratedRuntime(),
		g.layout.cmdGeneratedServices(),
		g.layout.cmdGeneratedRoutes(),
	}
	if g.shouldCopyIDLSource() {
		paths = append(paths, g.layout.idlCopy())
	}
	if g.config.WithConfig {
		paths = append(paths,
			g.layout.configCode(),
			g.layout.configLocal(),
			g.layout.configEnv(),
			g.layout.configRemote(),
			g.layout.configLoader(),
		)
	}
	if g.config.WithModel {
		for _, model := range ctx.models {
			paths = append(paths,
				g.layout.generatedModelFile(model.Name),
				g.layout.repositoryFile(model.Name),
			)
		}
		if len(ctx.models) > 0 {
			paths = append(paths, g.layout.repositoryBaseFile())
		}
	}
	for _, service := range ctx.services {
		paths = append(paths,
			g.layout.endpointGeneratedChainFile(service.ServiceName),
			g.layout.clientDemoFile(service.ServiceName),
			g.layout.sdkFile(service.ServiceName),
		)
		if g.config.WithModel {
			paths = append(paths, g.layout.serviceGeneratedReposFile(service.ServiceName))
		}
		if g.config.WithGRPC {
			paths = append(paths, g.layout.protoFile(service.ServiceName))
		}
		if g.config.WithTests {
			paths = append(paths, g.layout.serviceTestFile(service.ServiceName))
		}
	}
	if g.config.WithOpenAPI {
		paths = append(paths,
			g.layout.docsEmbed(),
			g.layout.openAPIFile(),
			g.layout.jsonSchemaFile(),
			g.layout.typeScriptClientFile(),
			g.layout.typeScriptReadme(),
			g.layout.typeScriptConfig(),
		)
	}
	if g.config.WithInteraction {
		paths = append(paths, g.layout.cmdGeneratedInteraction(), g.layout.aiProjectGuide())
	}

	artifacts := make([]string, 0, len(paths))
	for _, artifactPath := range paths {
		rel, err := filepath.Rel(g.outputDir, artifactPath)
		if err != nil {
			continue
		}
		artifacts = append(artifacts, filepath.ToSlash(rel))
	}
	sort.Strings(artifacts)
	return artifacts
}

func readProjectManifest(manifestPath, modulePath string) (*ProjectManifest, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var manifest ProjectManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid microgen project manifest %s: %w", manifestPath, err)
	}
	if err := validateProjectManifest(manifest, modulePath); err != nil {
		return nil, fmt.Errorf("invalid microgen project manifest %s: %w", manifestPath, err)
	}
	return &manifest, nil
}

func validateProjectManifest(manifest ProjectManifest, modulePath string) error {
	if manifest.SchemaVersion != ProjectManifestSchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %q (want %q)", manifest.SchemaVersion, ProjectManifestSchemaVersion)
	}
	if manifest.ModulePath != modulePath {
		return fmt.Errorf("modulePath %q does not match go.mod module %q", manifest.ModulePath, modulePath)
	}
	if strings.TrimSpace(manifest.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if manifest.Capabilities.Config && manifest.Capabilities.ConfigMode == "" {
		return fmt.Errorf("capabilities.configMode is required when config is enabled")
	}
	if !manifest.Capabilities.Config && (manifest.Capabilities.ConfigMode != "" || manifest.Capabilities.RemoteProvider != "") {
		return fmt.Errorf("configMode and remoteProvider require capabilities.config")
	}
	if manifest.Capabilities.Database {
		if _, ok := supportedDrivers[manifest.Capabilities.DatabaseDriver]; !ok {
			return fmt.Errorf("unsupported capabilities.databaseDriver %q", manifest.Capabilities.DatabaseDriver)
		}
	}
	if err := validateUniqueNames("service", manifest.Services); err != nil {
		return err
	}
	if err := validateUniqueNames("model", manifest.Models); err != nil {
		return err
	}
	if len(manifest.Models) > 0 && !manifest.Capabilities.Model {
		return fmt.Errorf("models require capabilities.model")
	}
	if err := validateGeneratedMiddlewareNames(manifest.GeneratedMiddlewares); err != nil {
		return err
	}

	seenArtifacts := make(map[string]struct{}, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		clean := path.Clean(artifact)
		if artifact == "" || clean != artifact || clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("artifact path must be a normalized relative slash path: %q", artifact)
		}
		if _, exists := seenArtifacts[artifact]; exists {
			return fmt.Errorf("duplicate artifact path %q", artifact)
		}
		seenArtifacts[artifact] = struct{}{}
	}
	if _, ok := seenArtifacts[".microgen/manifest.json"]; !ok {
		return fmt.Errorf("artifacts must include .microgen/manifest.json")
	}
	return nil
}

func validateUniqueNames(kind string, names []string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%s name must not be empty", kind)
		}
		if !token.IsIdentifier(name) {
			return fmt.Errorf("%s name must be a Go identifier: %q", kind, name)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate %s name %q", kind, name)
		}
		seen[key] = struct{}{}
	}
	return nil
}
