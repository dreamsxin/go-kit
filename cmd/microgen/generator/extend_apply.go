package generator

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
)

type ExtendResult struct {
	Plan *ArtifactPlan
}

// ApplyAppendService performs a safe append-service update against an existing
// generated project. The provided source project must contain the existing
// services plus the new service to append so generator-owned aggregation files
// can be regenerated deterministically.
func ApplyAppendService(templateFS fs.FS, root string, source *ir.Project, opts ExtendOptions, idlSourcePath string) (*ExtendResult, error) {
	existing, err := ScanExistingProject(root)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("source project is required")
	}
	if idlSourcePath == "" || strings.HasSuffix(strings.ToLower(idlSourcePath), ".proto") {
		return nil, fmt.Errorf("append-service currently requires a Go IDL source file containing the full combined contract; .proto input is not supported")
	}
	if err := validateSourceCoversExistingServices(existing, source); err != nil {
		return nil, err
	}

	genOpts := optionsFromExistingProject(existing)
	genOpts.TemplateFS = templateFS
	genOpts.OutputDir = root
	genOpts.ImportPath = existing.ModulePath
	genOpts.IDLSrcPath = idlSourcePath

	plan, err := BuildAppendServicePlan(existing, source, genOpts, opts)
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "microgen-append-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	tempOpts := genOpts
	tempOpts.OutputDir = tempDir
	gen, err := New(tempOpts)
	if err != nil {
		return nil, err
	}
	if err := gen.GenerateIR(source); err != nil {
		return nil, err
	}

	for _, file := range plan.NewFiles {
		if err := copyPlannedFile(existing.Root, tempDir, file.Path); err != nil {
			return nil, err
		}
	}
	for _, file := range plan.UpdatedFiles {
		if err := copyPlannedFile(existing.Root, tempDir, file.Path); err != nil {
			return nil, err
		}
	}

	return &ExtendResult{Plan: plan}, nil
}

// ApplyAppendModel performs a safe append-model update against an existing
// generated project. The source contract must contain the full combined
// service/model contract so generator-owned wiring can be refreshed safely.
func ApplyAppendModel(templateFS fs.FS, root string, source *ir.Project, opts ExtendOptions, idlSourcePath string) (*ExtendResult, error) {
	existing, err := ScanExistingProject(root)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("source project is required")
	}
	if idlSourcePath == "" || strings.HasSuffix(strings.ToLower(idlSourcePath), ".proto") {
		return nil, fmt.Errorf("append-model currently requires a Go IDL source file containing the full combined contract; .proto input is not supported")
	}
	if err := validateSourceCoversExistingServices(existing, source); err != nil {
		return nil, err
	}
	if err := validateSourceCoversExistingModels(existing, source); err != nil {
		return nil, err
	}

	genOpts := optionsFromExistingProject(existing)
	genOpts.TemplateFS = templateFS
	genOpts.OutputDir = root
	genOpts.ImportPath = existing.ModulePath
	genOpts.IDLSrcPath = idlSourcePath

	plan, err := BuildAppendModelPlan(existing, source, genOpts, opts)
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "microgen-append-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	tempOpts := genOpts
	tempOpts.OutputDir = tempDir
	gen, err := New(tempOpts)
	if err != nil {
		return nil, err
	}
	if err := gen.GenerateIR(source); err != nil {
		return nil, err
	}

	for _, file := range plan.NewFiles {
		if err := copyPlannedFile(existing.Root, tempDir, file.Path); err != nil {
			return nil, err
		}
	}
	for _, file := range plan.UpdatedFiles {
		if err := copyPlannedFile(existing.Root, tempDir, file.Path); err != nil {
			return nil, err
		}
	}

	return &ExtendResult{Plan: plan}, nil
}

// ApplyAppendMiddleware refreshes generator-owned endpoint middleware seams for
// supported generated middleware names while preserving user custom chains.
func ApplyAppendMiddleware(templateFS fs.FS, root string, source *ir.Project, opts ExtendOptions, idlSourcePath string) (*ExtendResult, error) {
	existing, err := ScanExistingProject(root)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("source project is required")
	}
	if idlSourcePath == "" || strings.HasSuffix(strings.ToLower(idlSourcePath), ".proto") {
		return nil, fmt.Errorf("append-middleware currently requires a Go IDL source file containing the full combined contract; .proto input is not supported")
	}
	if err := validateSourceCoversExistingServiceNames(existing, source); err != nil {
		return nil, err
	}

	genOpts := optionsFromExistingProject(existing)
	genOpts.TemplateFS = templateFS
	genOpts.OutputDir = root
	genOpts.ImportPath = existing.ModulePath
	genOpts.IDLSrcPath = idlSourcePath
	genOpts.GeneratedMiddlewares = normalizeGeneratedMiddlewares(append(genOpts.GeneratedMiddlewares, opts.AppendMiddleware...))

	plan, err := BuildAppendMiddlewarePlan(existing, source, genOpts, opts)
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "microgen-append-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	tempOpts := genOpts
	tempOpts.OutputDir = tempDir
	gen, err := New(tempOpts)
	if err != nil {
		return nil, err
	}
	if err := gen.GenerateIR(source); err != nil {
		return nil, err
	}

	for _, file := range plan.UpdatedFiles {
		if err := copyPlannedFile(existing.Root, tempDir, file.Path); err != nil {
			return nil, err
		}
	}
	return &ExtendResult{Plan: plan}, nil
}

func optionsFromExistingProject(existing *ExistingProject) Options {
	protocols := []string{"http"}
	if existing.Features.WithGRPC {
		protocols = append(protocols, "grpc")
	}
	return Options{
		ImportPath:           existing.ModulePath,
		Protocols:            protocols,
		WithConfig:           existing.Features.WithConfig,
		WithDocs:             fileExists(filepath.Join(existing.Root, "README.md")),
		WithTests:            existing.Features.WithTests,
		WithModel:            existing.Features.WithModel,
		WithGRPC:             existing.Features.WithGRPC,
		WithDB:               existing.Features.WithDB,
		WithSwag:             existing.Features.WithSwag,
		WithSkill:            existing.Features.WithSkill,
		RoutePrefix:          existing.Features.RoutePrefix,
		GeneratedMiddlewares: append([]string(nil), existing.Features.GeneratedMiddlewares...),
	}
}

func validateSourceCoversExistingServices(existing *ExistingProject, source *ir.Project) error {
	if existing == nil || source == nil {
		return fmt.Errorf("existing project and source project are required")
	}
	if err := validateSourceCoversExistingServiceNames(existing, source); err != nil {
		return fmt.Errorf("append-service requires a full combined Go IDL contract; %s", strings.TrimPrefix(err.Error(), "append-middleware requires a full combined Go IDL contract; "))
	}
	return nil
}

func validateSourceCoversExistingModels(existing *ExistingProject, source *ir.Project) error {
	if existing == nil || source == nil {
		return fmt.Errorf("existing project and source project are required")
	}
	sourceNames := make([]string, 0, len(source.Messages))
	for _, model := range source.Messages {
		if model != nil && model.HasGormTags {
			sourceNames = append(sourceNames, strings.ToLower(model.Name))
		}
	}
	var missing []string
	for _, model := range existing.Models {
		if !slices.Contains(sourceNames, strings.ToLower(model.Name)) {
			missing = append(missing, model.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("append-model requires a full combined Go IDL contract; missing existing model definitions for: %s", strings.Join(missing, ", "))
	}
	return nil
}

func copyPlannedFile(root, tempDir, targetPath string) error {
	rel, err := filepath.Rel(root, targetPath)
	if err != nil {
		return err
	}
	srcPath := filepath.Join(tempDir, rel)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(targetPath, data, 0o644)
}
