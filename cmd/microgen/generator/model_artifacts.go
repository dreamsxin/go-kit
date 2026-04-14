package generator

import "github.com/dreamsxin/go-kit/cmd/microgen/parser"

func (g *Generator) generateModelFile(model *parser.Model) error {
	data := map[string]any{
		"Models":     []*parser.Model{model},
		"ImportPath": g.config.ImportPath,
	}
	if err := g.executeTemplate("model.tmpl", g.layout.modelFile(), data); err != nil {
		return err
	}
	hooksData := map[string]any{
		"Name": model.Name,
	}
	return g.executeTemplate("model_hooks.tmpl", g.layout.modelHooksFile(model.Name), hooksData)
}

func (g *Generator) generateRepositoryBaseFile() error {
	data := map[string]any{
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("repository_base.tmpl", g.layout.repositoryBaseFile(), data)
}

func (g *Generator) generateRepositoryFile(model *parser.Model) error {
	data := map[string]any{
		"Model":      model,
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("repository.tmpl", g.layout.repositoryFile(), data)
}
