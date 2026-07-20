package generator

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type OwnershipTier string

const (
	OwnershipGeneratorRebuildable OwnershipTier = "generator_rebuildable"
	OwnershipGeneratorAggregation OwnershipTier = "generator_aggregation"
	OwnershipUserProtected        OwnershipTier = "user_protected"
)

type FileOwnership struct {
	Path   string
	Tier   OwnershipTier
	Reason string
}

type ExistingService struct {
	Name              string
	PackageName       string
	ServiceFile       string
	EndpointFile      string
	HTTPTransportFile string
	GRPCTransportFile string
}

type ExistingModel struct {
	Name string
	File string
}

type AggregationPoints struct {
	GeneratedServices string
	GeneratedRoutes   string
	GeneratedRuntime  string
	GeneratedChain    string
}

type ExistingProject struct {
	Root              string
	ModulePath        string
	IDLPath           string
	Manifest          *ProjectManifest
	ManifestDrift     []string
	Services          []ExistingService
	Models            []ExistingModel
	AggregationPoints AggregationPoints
	Ownership         map[string]FileOwnership
	Warnings          []string
	Features          ExistingProjectFeatures
}

type ExistingProjectFeatures struct {
	WithConfig           bool
	WithDocs             bool
	WithTests            bool
	WithModel            bool
	WithGRPC             bool
	WithDB               bool
	WithOpenAPI          bool
	WithInteraction      bool
	ConfigMode           string
	RemoteProvider       string
	DBDriver             string
	RoutePrefix          string
	GeneratedMiddlewares []string
}

var (
	typeInterfacePattern = regexp.MustCompile(`type\s+([A-Za-z0-9_]+)\s+interface\s*\{`)
	typeStructPattern    = regexp.MustCompile(`type\s+([A-Za-z0-9_]+)\s+struct\s*\{`)
)

// ScanExistingProject inspects an existing target tree and classifies files
// according to the microgen ownership model used by future extend flows.
func ScanExistingProject(root string) (*ExistingProject, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	modulePath, err := readModulePath(filepath.Join(absRoot, "go.mod"))
	if err != nil {
		return nil, err
	}

	layout := newProjectLayout(absRoot)
	manifest, err := readProjectManifest(layout.manifestFile(), modulePath)
	if err != nil {
		return nil, err
	}

	project := &ExistingProject{
		Root:       absRoot,
		ModulePath: modulePath,
		IDLPath:    filepath.Join(absRoot, "idl.go"),
		Manifest:   manifest,
		Ownership:  map[string]FileOwnership{},
	}

	if err := validateProjectLayout(absRoot, manifest != nil); err != nil {
		return nil, err
	}

	if err := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		project.Ownership[rel] = classifyOwnership(absRoot, rel)
		return nil
	}); err != nil {
		return nil, err
	}

	project.AggregationPoints = detectAggregationPoints(absRoot, project.Ownership)
	if manifest != nil {
		project.Services = existingServicesFromManifest(absRoot, manifest.Services)
		project.Models = existingModelsFromManifest(absRoot, manifest.Models)
		project.Features = projectFeaturesFromManifest(*manifest)
		project.ManifestDrift = detectManifestDrift(project)
	} else {
		project.Services = detectExistingServices(absRoot)
		project.Models = detectExistingModels(absRoot)
		project.Features = detectProjectFeatures(absRoot, project)
		project.ManifestDrift = []string{".microgen/manifest.json is missing; regenerate the project with the current microgen before extending it"}
	}
	project.Warnings = detectWarnings(project)

	return project, nil
}

func validateProjectLayout(root string, hasManifest bool) error {
	required := []string{
		filepath.Join(root, "go.mod"),
		filepath.Join(root, "cmd"),
	}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("unsupported existing project layout: missing %s; extend mode expects a microgen-generated project root with go.mod and cmd/", filepath.Base(path))
			}
			return err
		}
	}

	hasGeneratedArea := false
	for _, dir := range []string{"service", "endpoint", "transport"} {
		if _, err := os.Stat(filepath.Join(root, dir)); err == nil {
			hasGeneratedArea = true
			break
		}
	}
	if !hasGeneratedArea && !hasManifest {
		return fmt.Errorf("unsupported existing project layout: expected at least one of service/, endpoint/, or transport/; extend mode currently supports only generated microgen project layouts")
	}
	return nil
}

func readModulePath(goModPath string) (string, error) {
	f, err := os.Open(goModPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("unsupported existing project layout: missing go.mod; extend mode expects a microgen-generated project root")
		}
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			modulePath := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if modulePath == "" {
				break
			}
			return modulePath, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("unsupported existing project layout: could not determine module path from go.mod; extend mode requires a valid module declaration")
}

func classifyOwnership(root, rel string) FileOwnership {
	fullPath := filepath.Join(root, filepath.FromSlash(rel))
	switch rel {
	case "cmd/generated_services.go":
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorAggregation, Reason: "generator-owned service aggregation"}
	case "cmd/generated_routes.go":
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorAggregation, Reason: "generator-owned route aggregation"}
	case "cmd/generated_runtime.go":
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorAggregation, Reason: "generator-owned runtime aggregation"}
	case "endpoint/generated_chain.go":
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorAggregation, Reason: "generator-owned endpoint chain aggregation"}
	}

	switch {
	case rel == ".microgen/manifest.json":
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated project identity manifest"}
	case rel == "cmd/generated_interaction.go":
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated interaction runtime wiring"}
	case strings.HasPrefix(rel, ".ai/"):
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated AI project metadata"}
	case strings.HasPrefix(rel, "sdk/"):
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated sdk output"}
	case strings.HasPrefix(rel, "client/"):
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated demo client output"}
	case rel == "docs/docs.go" || rel == "docs/openapi.json" || rel == "docs/schema.json":
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated API contract output"}
	case rel == "config/config.go" || rel == "config/local.go" || rel == "config/env.go" || rel == "config/remote.go" || rel == "config/loader.go":
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated config schema"}
	case strings.HasPrefix(rel, "test/") && strings.HasSuffix(rel, "_test.go"):
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated service test scaffold"}
	case strings.HasPrefix(rel, "model/generated_"):
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated model schema output"}
	case strings.HasPrefix(rel, "repository/generated_"):
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated repository output"}
	case strings.HasPrefix(rel, "pb/"):
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated proto contract output"}
	case rel == "config/config.yaml":
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "user-edited config values"}
	case rel == "cmd/custom_routes.go":
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "custom route hook is user-owned"}
	case strings.HasPrefix(rel, "endpoint/") && strings.HasSuffix(rel, "/generated_chain.go"):
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated endpoint middleware seam"}
	case strings.HasPrefix(rel, "endpoint/") && strings.HasSuffix(rel, "/custom_chain.go"):
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "custom endpoint middleware seam is user-owned"}
	case strings.HasPrefix(rel, "service/") && strings.HasSuffix(rel, "/generated_repos.go"):
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generated service repository dependency seam"}
	case strings.HasPrefix(rel, "service/"):
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "service implementations are user-owned after creation"}
	case strings.HasPrefix(rel, "endpoint/"):
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "endpoint code is not a supported extend mutation point"}
	case strings.HasPrefix(rel, "transport/"):
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "transport files may be customized and are protected in extend mode"}
	case strings.HasPrefix(rel, "model/"):
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "model customization files are treated as protected"}
	case strings.HasPrefix(rel, "repository/"):
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "custom repository files are treated as protected"}
	case rel == "cmd/main.go":
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: mainOwnershipReason(fullPath)}
	case rel == "go.mod":
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "module file is compatibility-sensitive"}
	case rel == "README.md":
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "readme is treated as user-owned documentation"}
	case rel == "idl.go":
		return FileOwnership{Path: rel, Tier: OwnershipGeneratorRebuildable, Reason: "generator-managed source contract snapshot"}
	default:
		return FileOwnership{Path: rel, Tier: OwnershipUserProtected, Reason: "unrecognized file defaults to protected"}
	}
}

func mainOwnershipReason(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "startup file is protected"
	}
	if strings.Contains(string(data), "Code generated by microgen") {
		return "startup file still appears generator-owned, but extend mode does not rewrite it directly"
	}
	return "startup file is treated as user-owned because current templates mix generated and handwritten logic"
}

func detectAggregationPoints(root string, ownership map[string]FileOwnership) AggregationPoints {
	var points AggregationPoints
	for _, rel := range []string{
		"cmd/generated_services.go",
		"cmd/generated_routes.go",
		"cmd/generated_runtime.go",
		"endpoint/generated_chain.go",
	} {
		info, ok := ownership[rel]
		if !ok || info.Tier != OwnershipGeneratorAggregation {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		switch rel {
		case "cmd/generated_services.go":
			points.GeneratedServices = path
		case "cmd/generated_routes.go":
			points.GeneratedRoutes = path
		case "cmd/generated_runtime.go":
			points.GeneratedRuntime = path
		case "endpoint/generated_chain.go":
			points.GeneratedChain = path
		}
	}
	return points
}

func detectExistingServices(root string) []ExistingService {
	serviceRoot := filepath.Join(root, "service")
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		return nil
	}

	services := make([]ExistingService, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pkg := entry.Name()
		serviceFile := filepath.Join(serviceRoot, pkg, "service.go")
		if _, err := os.Stat(serviceFile); err != nil {
			continue
		}
		name := detectTypeName(serviceFile, typeInterfacePattern)
		services = append(services, ExistingService{
			Name:              name,
			PackageName:       pkg,
			ServiceFile:       serviceFile,
			EndpointFile:      filepath.Join(root, "endpoint", pkg, "endpoints.go"),
			HTTPTransportFile: filepath.Join(root, "transport", pkg, "transport_http.go"),
			GRPCTransportFile: filepath.Join(root, "transport", pkg, "transport_grpc.go"),
		})
	}

	sort.Slice(services, func(i, j int) bool {
		return services[i].PackageName < services[j].PackageName
	})
	return services
}

func detectExistingModels(root string) []ExistingModel {
	modelRoot := filepath.Join(root, "model")
	entries, err := os.ReadDir(modelRoot)
	if err != nil {
		return nil
	}

	models := make([]ExistingModel, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		path := filepath.Join(modelRoot, entry.Name())
		name, ok := detectTypeNameIfMatch(path, typeStructPattern)
		if !ok {
			continue
		}
		models = append(models, ExistingModel{Name: name, File: path})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].File < models[j].File
	})
	return models
}

func existingServicesFromManifest(root string, names []string) []ExistingService {
	services := make([]ExistingService, 0, len(names))
	for _, name := range names {
		pkg := strings.ToLower(name)
		services = append(services, ExistingService{
			Name:              name,
			PackageName:       pkg,
			ServiceFile:       filepath.Join(root, "service", pkg, "service.go"),
			EndpointFile:      filepath.Join(root, "endpoint", pkg, "endpoints.go"),
			HTTPTransportFile: filepath.Join(root, "transport", pkg, "transport_http.go"),
			GRPCTransportFile: filepath.Join(root, "transport", pkg, "transport_grpc.go"),
		})
	}
	sort.Slice(services, func(i, j int) bool {
		return services[i].PackageName < services[j].PackageName
	})
	return services
}

func existingModelsFromManifest(root string, names []string) []ExistingModel {
	models := make([]ExistingModel, 0, len(names))
	for _, name := range names {
		models = append(models, ExistingModel{
			Name: name,
			File: filepath.Join(root, "model", "generated_"+strings.ToLower(name)+".go"),
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].File < models[j].File
	})
	return models
}

func detectTypeName(path string, pattern *regexp.Regexp) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	match := pattern.FindSubmatch(data)
	if len(match) == 2 {
		return string(match[1])
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func detectTypeNameIfMatch(path string, pattern *regexp.Regexp) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	match := pattern.FindSubmatch(data)
	if len(match) != 2 {
		return "", false
	}
	return string(match[1]), true
}

func detectWarnings(project *ExistingProject) []string {
	var warnings []string
	if project.AggregationPoints.GeneratedServices == "" || project.AggregationPoints.GeneratedRoutes == "" {
		warnings = append(warnings, "append-service is not yet safely applicable because generator-owned cmd aggregation files are missing")
	}
	if own, ok := project.Ownership["cmd/main.go"]; ok && own.Tier == OwnershipUserProtected {
		warnings = append(warnings, "cmd/main.go is treated as protected and will not be rewritten by extend mode")
	}
	sort.Strings(warnings)
	return warnings
}

func projectFeaturesFromManifest(manifest ProjectManifest) ExistingProjectFeatures {
	capabilities := manifest.Capabilities
	return ExistingProjectFeatures{
		WithConfig:           capabilities.Config,
		WithDocs:             capabilities.Docs,
		WithTests:            capabilities.Tests,
		WithModel:            capabilities.Model,
		WithGRPC:             capabilities.GRPC,
		WithDB:               capabilities.Database,
		WithOpenAPI:          capabilities.OpenAPI,
		WithInteraction:      capabilities.Interaction,
		ConfigMode:           capabilities.ConfigMode,
		RemoteProvider:       capabilities.RemoteProvider,
		DBDriver:             capabilities.DatabaseDriver,
		RoutePrefix:          manifest.RoutePrefix,
		GeneratedMiddlewares: append([]string(nil), manifest.GeneratedMiddlewares...),
	}
}

func detectManifestDrift(project *ExistingProject) []string {
	if project == nil || project.Manifest == nil {
		return nil
	}

	declared := make(map[string]struct{}, len(project.Manifest.Artifacts))
	var drift []string
	for _, rel := range project.Manifest.Artifacts {
		declared[rel] = struct{}{}
		if !fileExists(filepath.Join(project.Root, filepath.FromSlash(rel))) {
			drift = append(drift, "manifest artifact is missing: "+rel)
			continue
		}
		ownership, ok := project.Ownership[rel]
		if !ok || (ownership.Tier != OwnershipGeneratorRebuildable && ownership.Tier != OwnershipGeneratorAggregation) {
			drift = append(drift, "manifest artifact is not classified as generator-owned: "+rel)
		}
	}
	for rel, ownership := range project.Ownership {
		if ownership.Tier != OwnershipGeneratorRebuildable && ownership.Tier != OwnershipGeneratorAggregation {
			continue
		}
		if _, ok := declared[rel]; !ok {
			drift = append(drift, "generator-owned artifact is not declared by manifest: "+rel)
		}
	}

	for _, service := range project.Services {
		if !fileExists(service.ServiceFile) {
			drift = append(drift, "manifest service scaffold is missing: service/"+service.PackageName+"/service.go")
			continue
		}
		if !fileDeclaresType(service.ServiceFile, typeInterfacePattern, service.Name) {
			drift = append(drift, fmt.Sprintf("manifest service %s is not declared in service/%s/service.go", service.Name, service.PackageName))
		}
	}
	for _, model := range project.Models {
		if !fileExists(model.File) {
			continue
		}
		if !fileDeclaresType(model.File, typeStructPattern, model.Name) {
			rel, _ := filepath.Rel(project.Root, model.File)
			drift = append(drift, fmt.Sprintf("manifest model %s does not match generated type in %s", model.Name, filepath.ToSlash(rel)))
		}
	}

	sort.Strings(drift)
	return drift
}

func fileDeclaresType(filePath string, pattern *regexp.Regexp, expected string) bool {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}
	for _, match := range pattern.FindAllSubmatch(data, -1) {
		if len(match) == 2 && string(match[1]) == expected {
			return true
		}
	}
	return false
}

func detectProjectFeatures(root string, project *ExistingProject) ExistingProjectFeatures {
	features := ExistingProjectFeatures{
		WithConfig:      fileExists(filepath.Join(root, "config", "config.go")),
		WithDocs:        fileExists(filepath.Join(root, "README.md")),
		WithTests:       dirExists(filepath.Join(root, "test")),
		WithModel:       hasGeneratedModelFiles(filepath.Join(root, "model")) || hasAnyMatch(filepath.Join(root, "repository"), "generated_"),
		WithGRPC:        hasAnyMatch(filepath.Join(root, "transport"), "transport_grpc.go") || dirExists(filepath.Join(root, "pb")),
		WithOpenAPI:     fileExists(filepath.Join(root, "docs", "docs.go")),
		WithInteraction: fileExists(filepath.Join(root, "cmd", "generated_interaction.go")),
	}
	if features.WithConfig {
		features.ConfigMode = "file"
	}
	features.WithDB = fileContains(filepath.Join(root, "cmd", "main.go"), "repository.NewDB(")
	features.RoutePrefix = detectRoutePrefix(project.AggregationPoints.GeneratedRoutes)
	features.GeneratedMiddlewares = detectGeneratedMiddlewares(project.Services)
	return features
}

func detectGeneratedMiddlewares(services []ExistingService) []string {
	seen := map[string]bool{}
	for _, svc := range services {
		path := filepath.Join(filepath.Dir(svc.EndpointFile), "generated_chain.go")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		if strings.Contains(content, "endpoint.TracingMiddleware()") {
			seen["tracing"] = true
		}
		if strings.Contains(content, "endpoint.ErrorHandlingMiddleware(name)") {
			seen["error-handling"] = true
		}
		if strings.Contains(content, "endpoint.MetricsMiddleware(generatedMetrics(name))") {
			seen["metrics"] = true
		}
	}
	var out []string
	for _, name := range []string{"tracing", "error-handling", "metrics"} {
		if seen[name] {
			out = append(out, name)
		}
	}
	return out
}

func hasGeneratedModelFiles(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "generated_") && filepath.Ext(name) == ".go" {
			return true
		}
	}
	return false
}

func detectRoutePrefix(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(data)
	idx := strings.Index(content, `{Prefix: "`)
	if idx < 0 {
		return ""
	}
	rest := content[idx+len(`{Prefix: "`):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	prefix := rest[:end]
	if prefix == "" {
		return ""
	}
	if slash := strings.LastIndex(prefix, "/"); slash > 0 {
		return prefix[:slash]
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), needle)
}

func hasAnyMatch(root, needle string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if fileExists(filepath.Join(root, name, needle)) {
				return true
			}
			continue
		}
		if strings.Contains(name, needle) {
			return true
		}
	}
	return false
}
