package generator

import (
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
)

type generationContext struct {
	project  *ir.Project
	services []*serviceView
	models   []*modelView
	source   string
}

type serviceView struct {
	ServiceName string
	PackageName string
	Title       string
	Description string
	Methods     []methodView
}

type methodView struct {
	Name       string
	Input      string
	Output     string
	Doc        string
	Summary    string
	Tags       string
	HTTPMethod string
	Route      string
}

type modelView struct {
	Name        string
	TableName   string
	Comment     string
	Fields      []modelFieldView
	HasGormTags bool
}

type modelFieldView struct {
	Name       string
	Type       string
	JSONTag    string
	GormTag    string
	Comment    string
	IsPrimary  bool
	IsAutoIncr bool
	IsNotNull  bool
	IsUnique   bool
	SwagType   string
	Example    string
}

func newGenerationContext(project *ir.Project) generationContext {
	ctx := generationContext{
		project:  project,
		services: servicesFromProject(project),
		models:   modelsFromProject(project),
	}

	if ctx.project == nil {
		ctx.project = &ir.Project{}
	}
	if ctx.source == "" {
		ctx.source = sourceTypeFromProject(ctx.project)
	}

	return ctx
}

func servicesFromProject(project *ir.Project) []*serviceView {
	if project == nil {
		return nil
	}

	services := make([]*serviceView, 0, len(project.Services))
	for _, svc := range project.Services {
		if svc == nil {
			continue
		}
		view := &serviceView{
			ServiceName: svc.Name,
			PackageName: svc.PackageName,
			Title:       svc.Title,
			Description: svc.Description,
		}
		for _, method := range svc.Methods {
			if method == nil {
				continue
			}
			view.Methods = append(view.Methods, methodView{
				Name:       method.Name,
				Input:      method.InputName,
				Output:     method.OutputName,
				Doc:        method.Description,
				Summary:    method.Summary,
				Tags:       strings.Join(method.Tags, ", "),
				HTTPMethod: strings.ToLower(method.HTTPMethod),
				Route:      method.Route,
			})
		}
		services = append(services, view)
	}
	return services
}

func modelsFromProject(project *ir.Project) []*modelView {
	if project == nil || len(project.Messages) == 0 {
		return nil
	}

	out := make([]*modelView, 0, len(project.Messages))
	for _, msg := range project.Messages {
		if msg == nil || !msg.HasGormTags {
			continue
		}

		mv := &modelView{
			Name:        msg.Name,
			TableName:   msg.TableName,
			Comment:     msg.Description,
			HasGormTags: msg.HasGormTags,
		}
		for _, field := range msg.Fields {
			if field == nil {
				continue
			}
			mv.Fields = append(mv.Fields, modelFieldView{
				Name:       field.Name,
				Type:       field.GoType,
				JSONTag:    field.JSONName,
				GormTag:    field.GormTag,
				Comment:    field.Description,
				IsPrimary:  field.IsPrimary,
				IsAutoIncr: field.IsAutoIncr,
				IsNotNull:  field.Required,
				IsUnique:   field.IsUnique,
				SwagType:   field.SwagType,
				Example:    field.Example,
			})
		}
		out = append(out, mv)
	}
	return out
}

func sourceTypeFromProject(project *ir.Project) string {
	if project == nil {
		return ""
	}
	return strings.ToLower(project.Source)
}
