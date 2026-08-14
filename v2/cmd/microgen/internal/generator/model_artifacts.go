package generator

import (
	"os"
	"sort"
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
	orderColumns, defaultOrderBy := repositoryOrderColumns(model)
	data := repositoryTemplateData{
		Model:          model,
		ImportPath:     g.config.ImportPath,
		OrderColumns:   orderColumns,
		DefaultOrderBy: defaultOrderBy,
	}
	return g.executeTemplate("repository.tmpl", g.layout.repositoryFile(model.Name), data)
}

func repositoryOrderColumns(model *modelView) ([]repositoryOrderColumn, string) {
	if model == nil {
		return nil, ""
	}
	aliases := make(map[string]string)
	defaultAlias := ""
	for _, field := range model.Fields {
		column := gormColumnName(field)
		if column == "" {
			continue
		}
		for _, alias := range []string{field.JSONTag, toSnakeCase(field.Name), strings.ToLower(field.Name)} {
			alias = strings.ToLower(strings.TrimSpace(strings.Split(alias, ",")[0]))
			if alias != "" && alias != "-" {
				aliases[alias] = column
			}
		}
		if field.IsPrimary || defaultAlias == "" {
			defaultAlias = firstNonEmpty(field.JSONTag, toSnakeCase(field.Name))
			defaultAlias = strings.TrimSpace(strings.Split(defaultAlias, ",")[0])
		}
	}
	columns := make([]repositoryOrderColumn, 0, len(aliases))
	for alias, column := range aliases {
		columns = append(columns, repositoryOrderColumn{Alias: alias, Column: column})
	}
	sort.Slice(columns, func(i, j int) bool { return columns[i].Alias < columns[j].Alias })
	return columns, defaultAlias
}

func gormColumnName(field modelFieldView) string {
	for _, part := range strings.Split(field.GormTag, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), ":")
		if ok && strings.EqualFold(key, "column") {
			return strings.TrimSpace(value)
		}
	}
	return toSnakeCase(field.Name)
}
