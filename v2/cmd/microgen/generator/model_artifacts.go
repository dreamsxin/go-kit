package generator

import (
	"os"
	"strings"
)

func (g *Generator) generateModelFile(model *modelView) error {
	addAuditFields := g.config.WithModel && !strings.EqualFold(model.Source, "db")
	data := modelTemplateData{
		Model:          model,
		ImportPath:     g.config.ImportPath,
		AddAuditFields: addAuditFields,
		NeedsTime:      modelNeedsImport(model, "time.") || addAuditFields,
		NeedsGorm:      modelNeedsImport(model, "gorm.") || addAuditFields,
	}
	if err := g.executeTemplate("model.tmpl", g.layout.generatedModelFile(model.Name), data); err != nil {
		return err
	}
	path := g.layout.modelHooksFile(model.Name)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	hooksData := modelHooksTemplateData{
		Name: model.Name,
	}
	return g.executeTemplate("model_hooks.tmpl", path, hooksData)
}

func modelNeedsImport(model *modelView, token string) bool {
	if model == nil {
		return false
	}
	for _, field := range model.Fields {
		if strings.Contains(field.Type, token) {
			return true
		}
	}
	return false
}

func (g *Generator) generateRepositoryBaseFile() error {
	data := repositoryBaseTemplateData{
		ImportPath: g.config.ImportPath,
	}
	return g.executeTemplate("repository_base.tmpl", g.layout.repositoryBaseFile(), data)
}

func (g *Generator) generateRepositoryFile(model *modelView) error {
	data := repositoryTemplateData{
		Model:      model,
		ImportPath: g.config.ImportPath,
	}
	return g.executeTemplate("repository.tmpl", g.layout.repositoryFile(model.Name), data)
}
