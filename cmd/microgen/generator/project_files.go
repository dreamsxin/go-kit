package generator

import (
	"os"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
)

func (g *Generator) generateMainFileFull(ctx generationContext) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]
	data := map[string]any{
		"Project":      ctx.project,
		"Services":     ctx.services,
		"Models":       ctx.models,
		"GormModels":   ctx.models,
		"SvcRoutes":    g.serviceRoutes(ctx.project),
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

func (g *Generator) generateGeneratedRuntimeFile(ctx generationContext) error {
	data := map[string]any{
		"Project":    ctx.project,
		"GormModels": ctx.models,
		"WithDB":     g.config.WithDB,
		"WithGRPC":   g.config.WithGRPC,
		"WithSwag":   g.config.WithSwag,
		"WithSkill":  g.config.WithSkill,
		"SvcRoutes":  g.serviceRoutes(ctx.project),
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("generated_runtime.tmpl", g.layout.cmdGeneratedRuntime(), data)
}

func (g *Generator) generateGeneratedServicesFile(ctx generationContext) error {
	data := map[string]any{
		"Project":    ctx.project,
		"Services":   ctx.services,
		"GormModels": ctx.models,
		"ImportPath": g.config.ImportPath,
		"WithDB":     g.config.WithDB,
		"WithConfig": g.config.WithConfig,
	}
	return g.executeTemplate("generated_services.tmpl", g.layout.cmdGeneratedServices(), data)
}

func (g *Generator) generateGeneratedRoutesFile(ctx generationContext) error {
	data := map[string]any{
		"Project":     ctx.project,
		"Services":    ctx.services,
		"SvcRoutes":   g.serviceRoutes(ctx.project),
		"ImportPath":  g.config.ImportPath,
		"WithGRPC":    g.config.WithGRPC,
		"RoutePrefix": g.config.RoutePrefix,
	}
	return g.executeTemplate("generated_routes.tmpl", g.layout.cmdGeneratedRoutes(), data)
}

func (g *Generator) generateCustomRoutesFile() error {
	path := g.layout.cmdCustomRoutes()
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	data := map[string]any{
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("custom_routes.tmpl", path, data)
}

func (g *Generator) generateConfigFile(services []*serviceView) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]
	data := map[string]any{
		"Services":              services,
		"DBDriver":              g.config.DBDriver,
		"DBDefaultDSN":          dbMeta.DefaultDSN,
		"WithGRPC":              g.config.WithGRPC,
		"WithSwag":              g.config.WithSwag,
		"WithDB":                g.config.WithDB,
		"ConfigMode":            g.config.ConfigMode,
		"RemoteProvider":        g.config.RemoteProvider,
		"RemoteEnabledDefault":  g.config.ConfigMode == "hybrid" || g.config.ConfigMode == "remote",
		"RemoteFallbackDefault": g.config.ConfigMode != "remote",
	}
	return g.executeTemplate("config.tmpl", g.layout.configYAML(), data)
}

func (g *Generator) generateConfigCodeFile(services []*serviceView) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]
	data := map[string]any{
		"Services":              services,
		"WithDB":                g.config.WithDB,
		"WithGRPC":              g.config.WithGRPC,
		"WithSwag":              g.config.WithSwag,
		"DBDriver":              g.config.DBDriver,
		"DBDefaultDSN":          dbMeta.DefaultDSN,
		"ConfigMode":            g.config.ConfigMode,
		"RemoteProvider":        g.config.RemoteProvider,
		"RemoteEnabledDefault":  g.config.ConfigMode == "hybrid" || g.config.ConfigMode == "remote",
		"RemoteFallbackDefault": g.config.ConfigMode != "remote",
	}
	targets := []struct {
		template string
		path     string
	}{
		{template: "config_types.tmpl", path: g.layout.configCode()},
		{template: "config_local.tmpl", path: g.layout.configLocal()},
		{template: "config_env.tmpl", path: g.layout.configEnv()},
		{template: "config_remote.tmpl", path: g.layout.configRemote()},
		{template: "config_loader.tmpl", path: g.layout.configLoader()},
	}
	for _, target := range targets {
		if err := g.executeTemplate(target.template, target.path, data); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) generateReadme(ctx generationContext) error {
	data := map[string]any{
		"Project":      ctx.project,
		"IsProtoInput": strings.EqualFold(ctx.source, "proto") || strings.HasSuffix(g.config.IDLSrcPath, ".proto"),
	}
	return g.executeTemplate("readme.tmpl", g.layout.readme(), data)
}

func (g *Generator) generateDocsStub(services []*serviceView) error {
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

func (g *Generator) generateSkillFile(ctx generationContext) error {
	data := map[string]any{
		"Project":    ctx.project,
		"ImportPath": g.config.ImportPath,
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
		"WithConfig":  g.config.WithConfig,
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

func (g *Generator) serviceRoutes(project *ir.Project) []SvcRoute {
	if project == nil {
		return nil
	}
	routes := make([]SvcRoute, 0, len(project.Services))
	for _, svc := range project.Services {
		routes = append(routes, SvcRoute{
			Service:    svc,
			FullPrefix: routePrefix(g.config.RoutePrefix, svc.Name),
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
