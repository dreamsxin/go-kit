package generator

import (
	"os"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

func (g *Generator) generateMainFileFull(services []*parser.Service, models []*parser.Model) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]

	data := map[string]any{
		"Services":     services,
		"Models":       models,
		"GormModels":   models,
		"SvcRoutes":    g.serviceRoutes(services),
		"ImportPath":   g.config.ImportPath,
		"WithDB":       g.config.WithDB,
		"DBDriver":     g.config.DBDriver,
		"DBImportPkg":  dbMeta.ImportPkg,
		"DBOpenCall":   dbMeta.OpenCall,
		"DBDefaultDSN": dbMeta.DefaultDSN,
		"WithConfig":   g.config.WithConfig,
		"WithGRPC":     g.config.WithGRPC,
		"WithSwag":     g.config.WithSwag,
		"WithSkill":    g.config.WithSkill,
	}
	return g.executeTemplate("main.tmpl", g.layout.cmdMain(), data)
}

func (g *Generator) generateConfigFile(services []*parser.Service) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]
	data := map[string]any{
		"Services":     services,
		"DBDriver":     g.config.DBDriver,
		"DBDefaultDSN": dbMeta.DefaultDSN,
		"WithGRPC":     g.config.WithGRPC,
		"WithSwag":     g.config.WithSwag,
		"WithDB":       g.config.WithDB,
	}
	return g.executeTemplate("config.tmpl", g.layout.configYAML(), data)
}

func (g *Generator) generateConfigCodeFile(services []*parser.Service) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]
	data := map[string]any{
		"Services":     services,
		"WithDB":       g.config.WithDB,
		"WithGRPC":     g.config.WithGRPC,
		"WithSwag":     g.config.WithSwag,
		"DBDriver":     g.config.DBDriver,
		"DBDefaultDSN": dbMeta.DefaultDSN,
	}
	return g.executeTemplate("config_code.tmpl", g.layout.configCode(), data)
}

func (g *Generator) generateReadme(services []*parser.Service) error {
	data := map[string]any{
		"Services": services,
	}
	return g.executeTemplate("readme.tmpl", g.layout.readme(), data)
}

func (g *Generator) generateDocsStub(services []*parser.Service) error {
	if err := os.MkdirAll(g.layout.docsDir(), 0o755); err != nil {
		return err
	}
	path := g.layout.docsStub()
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	data := map[string]any{
		"Services": services,
	}
	return g.executeTemplate("docs.tmpl", path, data)
}

func (g *Generator) generateSkillFile(result *parser.ParseResult) error {
	data := map[string]any{
		"Services":   result.Services,
		"Models":     result.Models,
		"ImportPath": g.config.ImportPath,
		"Source":     result.Source,
	}
	return g.executeTemplate("skill.tmpl", g.layout.skillFile(), data)
}

func (g *Generator) generateGoModFile() error {
	path := g.layout.goMod()
	if data, err := os.ReadFile(path); err == nil {
		content := string(data)
		if strings.Contains(content, "module "+g.config.ImportPath) {
			return nil
		}
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "module ") {
				lines[i] = "module " + g.config.ImportPath
				break
			}
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	}

	data := map[string]any{
		"ImportPath":  g.config.ImportPath,
		"RootRelPath": g.rootRelativePath(),
	}
	return g.executeTemplate("go_mod.tmpl", g.layout.goMod(), data)
}

func (g *Generator) copyIDLFile(idlSrcPath string) error {
	src, err := os.ReadFile(idlSrcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(g.layout.idlCopy(), src, 0o644)
}

func (g *Generator) serviceRoutes(services []*parser.Service) []SvcRoute {
	routes := make([]SvcRoute, 0, len(services))
	for _, svc := range services {
		routes = append(routes, SvcRoute{
			Service:    svc,
			FullPrefix: routePrefix(g.config.RoutePrefix, svc.ServiceName),
		})
	}
	return routes
}

func (g *Generator) rootRelativePath() string {
	if strings.Contains(g.outputDir, "testdata") {
		return "../../../"
	}
	if strings.Contains(g.outputDir, "examples") {
		return "../../"
	}
	return "../../"
}
