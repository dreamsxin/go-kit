package generator

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/internal/ir"
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
	if err := validateExistingManifestForMutation(existing); err != nil {
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
	if err := copyPlannedUpdates(existing.Root, tempDir, plan.UpdatedFiles); err != nil {
		return nil, err
	}

	return &ExtendResult{Plan: plan}, nil
}

// ApplyRefresh regenerates the project's generator-owned artifacts from a
// full combined contract, without appending anything: the natural loop when a
// service grew a method, a type changed, or validate tags were added. The
// capabilities come from the project's own manifest, so nothing has to be
// re-typed; the plan copies only files the new generation declares as
// generator-owned artifacts, which is what keeps every user-owned file
// — service logic, main, custom chains, configuration — untouched. A
// generator-owned artifact the new contract no longer produces is reported as
// a warning and left in place, because deleting a file is a decision, not a
// side effect.
//
// Stable: extend.refresh — extend without append flags refreshes every
// generator-owned artifact from the passed contract under the manifest's
// capabilities; user-owned files are never written; artifacts the new
// generation no longer declares are warned about and left in place.
// Covered by: TestApplyRefreshUpdatesGeneratedArtifacts,
// TestApplyRefreshPreservesUserFiles, TestApplyRefreshReportsObsoleteArtifacts
func ApplyRefresh(templateFS fs.FS, root string, source *ir.Project, idlSourcePath string) (*ExtendResult, error) {
	existing, err := ScanExistingProject(root)
	if err != nil {
		return nil, err
	}
	if err := validateExistingManifestForMutation(existing); err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("source project is required")
	}
	if idlSourcePath == "" || strings.HasSuffix(strings.ToLower(idlSourcePath), ".proto") {
		return nil, fmt.Errorf("refresh requires a Go IDL source file containing the full combined contract; .proto input is not supported")
	}

	genOpts := optionsFromExistingProject(existing)
	genOpts.TemplateFS = templateFS
	genOpts.IDLSrcPath = idlSourcePath

	tempDir, err := os.MkdirTemp("", "microgen-refresh-*")
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

	tempManifest, err := readProjectManifest(newProjectLayout(tempDir).manifestFile(), genOpts.ImportPath)
	if err != nil {
		return nil, err
	}
	if tempManifest == nil {
		return nil, fmt.Errorf("refresh regeneration produced no manifest")
	}
	newArtifacts := make(map[string]struct{}, len(tempManifest.Artifacts))
	for _, artifact := range tempManifest.Artifacts {
		newArtifacts[artifact] = struct{}{}
	}

	oldArtifacts := make(map[string]struct{}, len(existing.Manifest.Artifacts))
	for _, artifact := range existing.Manifest.Artifacts {
		oldArtifacts[artifact] = struct{}{}
	}

	plan := &ArtifactPlan{}
	obsolete := make(map[string]struct{}, len(oldArtifacts))

	// The project's contract copy must reflect the contract this refresh ran
	// with: the next extend without -idl reads it, so a stale copy would
	// silently rewind the loop.
	contractSource, err := os.ReadFile(idlSourcePath)
	if err != nil {
		return nil, fmt.Errorf("read contract source: %w", err)
	}
	rootCopy := filepath.Join(root, "idl.go")
	contractCopyCurrent, err := os.ReadFile(rootCopy)
	contractCopyNeedsWrite := false
	switch {
	case errors.Is(err, os.ErrNotExist):
		plan.NewFiles = append(plan.NewFiles, PlannedFile{
			Path:      rootCopy,
			Ownership: OwnershipGeneratorRebuildable,
			Reason:    "refresh: the project's contract copy",
		})
		contractCopyNeedsWrite = true
	case err != nil:
		return nil, fmt.Errorf("read existing contract copy: %w", err)
	case !bytes.Equal(contractCopyCurrent, contractSource):
		plan.UpdatedFiles = append(plan.UpdatedFiles, PlannedUpdate{
			Path:      rootCopy,
			Ownership: OwnershipGeneratorRebuildable,
			Reason:    "refresh: the contract copy updates to the passed contract",
		})
		contractCopyNeedsWrite = true
	}
	for artifact := range oldArtifacts {
		if _, produced := newArtifacts[artifact]; !produced {
			obsolete[artifact] = struct{}{}
		}
	}
	for artifact := range newArtifacts {
		rootPath := filepath.Join(root, filepath.FromSlash(artifact))
		tempPath := filepath.Join(tempDir, filepath.FromSlash(artifact))
		generated, readErr := os.ReadFile(tempPath)
		if readErr != nil {
			return nil, fmt.Errorf("read regenerated %s: %w", artifact, readErr)
		}
		current, err := os.ReadFile(rootPath)
		switch {
		case errors.Is(err, os.ErrNotExist):
			plan.NewFiles = append(plan.NewFiles, PlannedFile{
				Path:      rootPath,
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "refresh: artifact is new in the regenerated project",
			})
		case err != nil:
			return nil, fmt.Errorf("read existing %s: %w", artifact, err)
		case !bytes.Equal(current, generated):
			plan.UpdatedFiles = append(plan.UpdatedFiles, PlannedUpdate{
				Path:      rootPath,
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "refresh: regenerated artifact differs",
			})
		}
	}
	sort.Slice(plan.NewFiles, func(i, j int) bool { return plan.NewFiles[i].Path < plan.NewFiles[j].Path })
	sort.Slice(plan.UpdatedFiles, func(i, j int) bool { return plan.UpdatedFiles[i].Path < plan.UpdatedFiles[j].Path })
	for artifact := range obsolete {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("obsolete generator-owned artifact %s is no longer produced by the contract; it is left in place — remove it and its references yourself", artifact))
	}
	sort.Strings(plan.Warnings)

	for _, file := range plan.NewFiles {
		if err := copyPlannedFile(root, tempDir, file.Path); err != nil {
			return nil, err
		}
	}
	for _, update := range plan.UpdatedFiles {
		if err := copyPlannedFile(root, tempDir, update.Path); err != nil {
			return nil, err
		}
	}
	if contractCopyNeedsWrite {
		if err := os.WriteFile(rootCopy, contractSource, 0o644); err != nil {
			return nil, fmt.Errorf("write contract copy: %w", err)
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
	if err := validateExistingManifestForMutation(existing); err != nil {
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
	if err := copyPlannedUpdates(existing.Root, tempDir, plan.UpdatedFiles); err != nil {
		return nil, err
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
	if err := validateExistingManifestForMutation(existing); err != nil {
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

	if err := copyPlannedUpdates(existing.Root, tempDir, plan.UpdatedFiles); err != nil {
		return nil, err
	}
	return &ExtendResult{Plan: plan}, nil
}

func validateExistingManifestForMutation(existing *ExistingProject) error {
	if existing == nil || existing.Manifest == nil {
		return fmt.Errorf("extend requires .microgen/manifest.json; regenerate the project with the current microgen before applying changes")
	}
	if len(existing.ManifestDrift) > 0 {
		return fmt.Errorf("extend refused because the project manifest has drift: %s", strings.Join(existing.ManifestDrift, "; "))
	}
	return nil
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
		ConfigMode:           existing.Features.ConfigMode,
		RemoteProvider:       existing.Features.RemoteProvider,
		WithDocs:             existing.Features.WithDocs,
		WithTests:            existing.Features.WithTests,
		WithModel:            existing.Features.WithModel,
		WithGRPC:             existing.Features.WithGRPC,
		WithDB:               existing.Features.WithDB,
		DBDriver:             existing.Features.DBDriver,
		WithOpenAPI:          existing.Features.WithOpenAPI,
		WithInteraction:      existing.Features.WithInteraction,
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

func copyPlannedUpdates(root, tempDir string, updates []PlannedUpdate) error {
	manifestPath := newProjectLayout(root).manifestFile()
	var manifestTarget string
	for i := range updates {
		file := updates[i]
		if filepath.Clean(file.Path) == filepath.Clean(manifestPath) {
			manifestTarget = file.Path
			continue
		}
		if err := copyPlannedFile(root, tempDir, file.Path); err != nil {
			return err
		}
	}
	if manifestTarget != "" {
		return copyPlannedFile(root, tempDir, manifestTarget)
	}
	return nil
}
