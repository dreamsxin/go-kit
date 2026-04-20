package generator

import "os"

func (g *Generator) generateModelFile(model *modelView) error {
	data := map[string]any{
		"Model":      model,
		"ImportPath": g.config.ImportPath,
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
	hooksData := map[string]any{
		"Name": model.Name,
	}
	return g.executeTemplate("model_hooks.tmpl", path, hooksData)
}

func (g *Generator) generateRepositoryBaseFile() error {
	data := map[string]any{
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("repository_base.tmpl", g.layout.repositoryBaseFile(), data)
}

func (g *Generator) generateRepositoryFile(model *modelView) error {
	data := map[string]any{
		"Model":      model,
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("repository.tmpl", g.layout.repositoryFile(model.Name), data)
}
