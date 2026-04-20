package generator

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
)

type ExtendOptions struct {
	AppendService    string
	AppendModel      string
	AppendMiddleware []string
}

type PlannedFile struct {
	Path      string
	Ownership OwnershipTier
	Reason    string
}

type PlannedUpdate struct {
	Path      string
	Ownership OwnershipTier
	Reason    string
}

type ArtifactPlan struct {
	NewFiles       []PlannedFile
	UpdatedFiles   []PlannedUpdate
	ProtectedSkips []string
	Warnings       []string
}

func missingExtendFilesMessage(action string, missing []string, recommendation string) string {
	return fmt.Sprintf("%s requires generator-owned compatibility seams; missing %s in the target project. %s", action, strings.Join(missing, ", "), recommendation)
}

// BuildAppendServicePlan validates an append-service request and reports the
// exact files that a future extend apply phase would need to create or update.
func BuildAppendServicePlan(existing *ExistingProject, project *ir.Project, opts Options, extend ExtendOptions) (*ArtifactPlan, error) {
	if existing == nil {
		return nil, fmt.Errorf("existing project scan is required")
	}
	if project == nil {
		return nil, fmt.Errorf("source project is required")
	}

	target, err := resolveAppendService(project, extend.AppendService)
	if err != nil {
		return nil, err
	}

	if serviceAlreadyExists(existing, target) {
		return nil, fmt.Errorf("append-service target already exists in the project: %s", target.Name)
	}
	var missing []string
	if existing.AggregationPoints.GeneratedServices == "" {
		missing = append(missing, "cmd/generated_services.go")
	}
	if existing.AggregationPoints.GeneratedRoutes == "" {
		missing = append(missing, "cmd/generated_routes.go")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s", missingExtendFilesMessage(
			"append-service",
			missing,
			"Re-generate the project with the current microgen templates, or add the missing generated cmd seams before retrying extend.",
		))
	}

	layout := newProjectLayout(existing.Root)
	plan := &ArtifactPlan{
		NewFiles: []PlannedFile{
			{
				Path:      layout.serviceFile(target.Name),
				Ownership: OwnershipUserProtected,
				Reason:    "new service implementation scaffold",
			},
			{
				Path:      layout.endpointsFile(target.Name),
				Ownership: OwnershipUserProtected,
				Reason:    "new endpoint scaffold",
			},
			{
				Path:      layout.endpointGeneratedChainFile(target.Name),
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "generated endpoint middleware chain for appended service",
			},
			{
				Path:      layout.endpointCustomChainFile(target.Name),
				Ownership: OwnershipUserProtected,
				Reason:    "custom endpoint middleware seam for appended service",
			},
			{
				Path:      layout.httpTransportFile(target.Name),
				Ownership: OwnershipUserProtected,
				Reason:    "new HTTP transport scaffold",
			},
			{
				Path:      layout.clientDemoFile(target.Name),
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "generated demo client for appended service",
			},
			{
				Path:      layout.sdkFile(target.Name),
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "generated sdk for appended service",
			},
		},
		UpdatedFiles: []PlannedUpdate{
			{
				Path:      filepath.Join(existing.Root, "idl.go"),
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "refresh generator-managed source contract snapshot",
			},
			{
				Path:      existing.AggregationPoints.GeneratedServices,
				Ownership: OwnershipGeneratorAggregation,
				Reason:    "register appended service wiring",
			},
			{
				Path:      existing.AggregationPoints.GeneratedRoutes,
				Ownership: OwnershipGeneratorAggregation,
				Reason:    "register appended HTTP routes",
			},
		},
		Warnings: append([]string(nil), existing.Warnings...),
	}

	if opts.WithGRPC {
		plan.NewFiles = append(plan.NewFiles,
			PlannedFile{
				Path:      layout.grpcTransportFile(target.Name),
				Ownership: OwnershipUserProtected,
				Reason:    "new gRPC transport scaffold",
			},
			PlannedFile{
				Path:      layout.protoFile(target.Name),
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "generated proto contract for appended service",
			},
		)
	}
	if opts.WithTests {
		plan.NewFiles = append(plan.NewFiles, PlannedFile{
			Path:      layout.serviceTestFile(target.Name),
			Ownership: OwnershipGeneratorRebuildable,
			Reason:    "generated service test scaffold",
		})
	}
	if opts.WithSkill {
		plan.UpdatedFiles = append(plan.UpdatedFiles, PlannedUpdate{
			Path:      filepath.Join(existing.Root, "skill", "skill.go"),
			Ownership: OwnershipGeneratorRebuildable,
			Reason:    "refresh generated skill output to include appended service",
		})
	}
	if existing.AggregationPoints.GeneratedRuntime != "" {
		plan.UpdatedFiles = append(plan.UpdatedFiles, PlannedUpdate{
			Path:      existing.AggregationPoints.GeneratedRuntime,
			Ownership: OwnershipGeneratorAggregation,
			Reason:    "refresh runtime wiring for appended service",
		})
	}

	slices.SortFunc(plan.NewFiles, func(a, b PlannedFile) int {
		return strings.Compare(a.Path, b.Path)
	})
	slices.SortFunc(plan.UpdatedFiles, func(a, b PlannedUpdate) int {
		return strings.Compare(a.Path, b.Path)
	})
	return plan, nil
}

// BuildAppendModelPlan validates an append-model request and reports the exact
// generated files that can be safely added or refreshed.
func BuildAppendModelPlan(existing *ExistingProject, project *ir.Project, opts Options, extend ExtendOptions) (*ArtifactPlan, error) {
	if existing == nil {
		return nil, fmt.Errorf("existing project scan is required")
	}
	if project == nil {
		return nil, fmt.Errorf("source project is required")
	}
	if !existing.Features.WithModel {
		return nil, fmt.Errorf("append-model requires an existing generated project with model/repository output enabled")
	}

	target, err := resolveAppendModel(project, extend.AppendModel)
	if err != nil {
		return nil, err
	}
	if modelAlreadyExists(existing, target) {
		return nil, fmt.Errorf("append-model target already exists in the project: %s", target.Name)
	}

	var missing []string
	if existing.Features.WithDB {
		if existing.AggregationPoints.GeneratedServices == "" {
			missing = append(missing, "cmd/generated_services.go")
		}
		if existing.AggregationPoints.GeneratedRuntime == "" {
			missing = append(missing, "cmd/generated_runtime.go")
		}
	}
	layout := newProjectLayout(existing.Root)
	for _, svc := range existing.Services {
		path := layout.serviceGeneratedReposFile(svc.Name)
		rel, _ := filepath.Rel(existing.Root, path)
		rel = filepath.ToSlash(rel)
		own, ok := existing.Ownership[rel]
		if !ok || own.Tier != OwnershipGeneratorRebuildable {
			missing = append(missing, rel)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s", missingExtendFilesMessage(
			"append-model",
			missing,
			"Re-generate the project with the current microgen templates so generated model, repository, and runtime seams are present, then retry extend.",
		))
	}

	plan := &ArtifactPlan{
		NewFiles: []PlannedFile{
			{
				Path:      layout.generatedModelFile(target.Name),
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "generated model schema for appended model",
			},
			{
				Path:      layout.modelHooksFile(target.Name),
				Ownership: OwnershipUserProtected,
				Reason:    "new model customization seam",
			},
			{
				Path:      layout.repositoryFile(target.Name),
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "generated repository for appended model",
			},
		},
		UpdatedFiles: []PlannedUpdate{
			{
				Path:      filepath.Join(existing.Root, "idl.go"),
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "refresh generator-managed source contract snapshot",
			},
		},
		Warnings: append([]string(nil), existing.Warnings...),
	}

	if existing.Features.WithDB {
		plan.UpdatedFiles = append(plan.UpdatedFiles,
			PlannedUpdate{
				Path:      existing.AggregationPoints.GeneratedRuntime,
				Ownership: OwnershipGeneratorAggregation,
				Reason:    "refresh generated model migration wiring",
			},
			PlannedUpdate{
				Path:      existing.AggregationPoints.GeneratedServices,
				Ownership: OwnershipGeneratorAggregation,
				Reason:    "refresh generated repository wiring",
			},
		)
	}

	for _, svc := range existing.Services {
		plan.UpdatedFiles = append(plan.UpdatedFiles, PlannedUpdate{
			Path:      layout.serviceGeneratedReposFile(svc.Name),
			Ownership: OwnershipGeneratorRebuildable,
			Reason:    "refresh generated service repository dependency seam",
		})
	}

	slices.SortFunc(plan.NewFiles, func(a, b PlannedFile) int {
		return strings.Compare(a.Path, b.Path)
	})
	slices.SortFunc(plan.UpdatedFiles, func(a, b PlannedUpdate) int {
		return strings.Compare(a.Path, b.Path)
	})
	return plan, nil
}

// BuildAppendMiddlewarePlan validates an append-middleware request and reports
// which generated chain files would be refreshed.
func BuildAppendMiddlewarePlan(existing *ExistingProject, project *ir.Project, opts Options, extend ExtendOptions) (*ArtifactPlan, error) {
	if existing == nil {
		return nil, fmt.Errorf("existing project scan is required")
	}
	if project == nil {
		return nil, fmt.Errorf("source project is required")
	}
	if len(existing.Services) == 0 {
		return nil, fmt.Errorf("append-middleware requires a generated project with at least one service")
	}
	if err := validateGeneratedMiddlewareNames(extend.AppendMiddleware); err != nil {
		return nil, err
	}
	requested := normalizeGeneratedMiddlewares(extend.AppendMiddleware)
	if len(requested) == 0 {
		return nil, fmt.Errorf("append-middleware requires at least one supported middleware name")
	}
	layout := newProjectLayout(existing.Root)
	var missing []string
	if existing.AggregationPoints.GeneratedRoutes == "" {
		missing = append(missing, "cmd/generated_routes.go")
	}
	for _, svc := range existing.Services {
		path := layout.endpointGeneratedChainFile(svc.Name)
		rel, _ := filepath.Rel(existing.Root, path)
		rel = filepath.ToSlash(rel)
		own, ok := existing.Ownership[rel]
		if !ok || own.Tier != OwnershipGeneratorRebuildable {
			missing = append(missing, rel)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s", missingExtendFilesMessage(
			"append-middleware",
			missing,
			"Re-generate the project with the current microgen templates so generated route and middleware seams are present, then retry extend.",
		))
	}
	if err := validateSourceCoversExistingServiceNames(existing, project); err != nil {
		return nil, err
	}

	already := map[string]bool{}
	for _, name := range existing.Features.GeneratedMiddlewares {
		already[name] = true
	}
	var newNames []string
	for _, name := range requested {
		if !already[name] {
			newNames = append(newNames, name)
		}
	}
	if len(newNames) == 0 {
		return nil, fmt.Errorf("append-middleware target already exists in the project: %s", strings.Join(requested, ", "))
	}

	plan := &ArtifactPlan{
		UpdatedFiles: []PlannedUpdate{
			{
				Path:      filepath.Join(existing.Root, "idl.go"),
				Ownership: OwnershipGeneratorRebuildable,
				Reason:    "refresh generator-managed source contract snapshot",
			},
		},
		Warnings: append([]string(nil), existing.Warnings...),
	}
	for _, svc := range existing.Services {
		plan.UpdatedFiles = append(plan.UpdatedFiles, PlannedUpdate{
			Path:      layout.endpointGeneratedChainFile(svc.Name),
			Ownership: OwnershipGeneratorRebuildable,
			Reason:    "refresh generated endpoint middleware chain",
		})
	}
	slices.SortFunc(plan.UpdatedFiles, func(a, b PlannedUpdate) int {
		return strings.Compare(a.Path, b.Path)
	})
	return plan, nil
}

func resolveAppendService(project *ir.Project, requested string) (*ir.Service, error) {
	if project == nil || len(project.Services) == 0 {
		return nil, fmt.Errorf("source contract does not contain any services")
	}
	if requested == "" {
		if len(project.Services) == 1 {
			return project.Services[0], nil
		}
		return nil, fmt.Errorf("append-service requires an explicit service name when the source contract contains multiple services")
	}
	available := make([]string, 0, len(project.Services))
	for _, svc := range project.Services {
		available = append(available, svc.Name)
		if strings.EqualFold(svc.Name, requested) || strings.EqualFold(svc.PackageName, requested) {
			return svc, nil
		}
	}
	return nil, fmt.Errorf("append-service target not found in source contract: %s (available: %s)", requested, strings.Join(available, ", "))
}

func resolveAppendModel(project *ir.Project, requested string) (*ir.Message, error) {
	if project == nil {
		return nil, fmt.Errorf("source project is required")
	}
	var models []*ir.Message
	for _, msg := range project.Messages {
		if msg != nil && msg.HasGormTags {
			models = append(models, msg)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("source contract does not contain any generated models")
	}
	if requested == "" {
		if len(models) == 1 {
			return models[0], nil
		}
		names := make([]string, 0, len(models))
		for _, model := range models {
			names = append(names, model.Name)
		}
		return nil, fmt.Errorf("append-model requires an explicit model name when the source contract contains multiple generated models (available: %s)", strings.Join(names, ", "))
	}
	available := make([]string, 0, len(models))
	for _, model := range models {
		available = append(available, model.Name)
		if strings.EqualFold(model.Name, requested) {
			return model, nil
		}
	}
	return nil, fmt.Errorf("append-model target not found in source contract: %s (available: %s)", requested, strings.Join(available, ", "))
}

func serviceAlreadyExists(existing *ExistingProject, svc *ir.Service) bool {
	if existing == nil || svc == nil {
		return false
	}
	for _, current := range existing.Services {
		if strings.EqualFold(current.Name, svc.Name) || strings.EqualFold(current.PackageName, svc.PackageName) {
			return true
		}
	}
	return false
}

func modelAlreadyExists(existing *ExistingProject, model *ir.Message) bool {
	if existing == nil || model == nil {
		return false
	}
	for _, current := range existing.Models {
		if strings.EqualFold(current.Name, model.Name) {
			return true
		}
	}
	return false
}

func validateSourceCoversExistingServiceNames(existing *ExistingProject, project *ir.Project) error {
	if existing == nil || project == nil {
		return fmt.Errorf("existing project and source project are required")
	}
	sourceNames := make([]string, 0, len(project.Services))
	for _, svc := range project.Services {
		sourceNames = append(sourceNames, strings.ToLower(svc.Name))
	}
	var missing []string
	for _, svc := range existing.Services {
		if !slices.Contains(sourceNames, strings.ToLower(svc.Name)) {
			missing = append(missing, svc.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("append-middleware requires a full combined Go IDL contract; missing existing service definitions for: %s", strings.Join(missing, ", "))
	}
	return nil
}
