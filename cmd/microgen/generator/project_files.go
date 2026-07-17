package generator

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
)

func (g *Generator) generateMainFileFull(ctx generationContext) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]
	data := mainTemplateData{
		Project:         ctx.project,
		Services:        ctx.services,
		Models:          ctx.models,
		GormModels:      ctx.models,
		SvcRoutes:       g.serviceRoutes(ctx.project),
		ImportPath:      g.config.ImportPath,
		WithDB:          g.config.WithDB,
		DBDriver:        g.config.DBDriver,
		DBImportPkg:     dbMeta.ImportPkg,
		DBOpenCall:      dbMeta.OpenCall,
		DBDefaultDSN:    dbMeta.DefaultDSN,
		WithConfig:      g.config.WithConfig,
		WithGRPC:        g.config.WithGRPC,
		WithSwag:        g.config.WithSwag,
		WithSkill:       g.config.WithSkill,
		WithInteraction: g.config.WithInteraction,
	}
	return g.executeTemplate("main.tmpl", g.layout.cmdMain(), data)
}

func (g *Generator) generateGeneratedRuntimeFile(ctx generationContext) error {
	data := generatedRuntimeTemplateData{
		Project:         ctx.project,
		GormModels:      ctx.models,
		WithDB:          g.config.WithDB,
		WithGRPC:        g.config.WithGRPC,
		WithSwag:        g.config.WithSwag,
		WithSkill:       g.config.WithSkill,
		WithInteraction: g.config.WithInteraction,
		SvcRoutes:       g.serviceRoutes(ctx.project),
		ImportPath:      g.config.ImportPath,
	}
	return g.executeTemplate("generated_runtime.tmpl", g.layout.cmdGeneratedRuntime(), data)
}

func (g *Generator) generateGeneratedServicesFile(ctx generationContext) error {
	data := generatedServicesTemplateData{
		Project:    ctx.project,
		Services:   ctx.services,
		GormModels: ctx.models,
		ImportPath: g.config.ImportPath,
		WithDB:     g.config.WithDB,
		WithConfig: g.config.WithConfig,
	}
	return g.executeTemplate("generated_services.tmpl", g.layout.cmdGeneratedServices(), data)
}

func (g *Generator) generateGeneratedRoutesFile(ctx generationContext) error {
	svcRoutes := g.serviceRoutes(ctx.project)
	data := generatedRoutesTemplateData{
		Project:        ctx.project,
		Services:       ctx.services,
		SvcRoutes:      svcRoutes,
		UnarySvcRoutes: unaryServiceRoutes(svcRoutes),
		ImportPath:     g.config.ImportPath,
		WithGRPC:       g.config.WithGRPC,
		RoutePrefix:    g.config.RoutePrefix,
	}
	return g.executeTemplate("generated_routes.tmpl", g.layout.cmdGeneratedRoutes(), data)
}

func (g *Generator) generateCustomRoutesFile() error {
	path := g.layout.cmdCustomRoutes()
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	data := customRoutesTemplateData{
		ImportPath: g.config.ImportPath,
	}
	return g.executeTemplate("custom_routes.tmpl", path, data)
}

func (g *Generator) generateConfigFile(services []*serviceView) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]
	data := configTemplateData{
		Services:              services,
		DBDriver:              g.config.DBDriver,
		DBDefaultDSN:          dbMeta.DefaultDSN,
		DBConfigDSN:           dbMeta.ConfigDSN,
		WithGRPC:              g.config.WithGRPC,
		WithSwag:              g.config.WithSwag,
		WithDB:                g.config.WithDB,
		ConfigMode:            g.config.ConfigMode,
		RemoteProvider:        g.config.RemoteProvider,
		RemoteEnabledDefault:  g.config.ConfigMode == "hybrid" || g.config.ConfigMode == "remote",
		RemoteFallbackDefault: g.config.ConfigMode != "remote",
	}
	return g.executeTemplate("config.tmpl", g.layout.configYAML(), data)
}

func (g *Generator) generateConfigCodeFile(services []*serviceView) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]
	data := configTemplateData{
		Services:              services,
		WithDB:                g.config.WithDB,
		WithGRPC:              g.config.WithGRPC,
		WithSwag:              g.config.WithSwag,
		DBDriver:              g.config.DBDriver,
		DBDefaultDSN:          dbMeta.DefaultDSN,
		DBConfigDSN:           dbMeta.ConfigDSN,
		ConfigMode:            g.config.ConfigMode,
		RemoteProvider:        g.config.RemoteProvider,
		RemoteEnabledDefault:  g.config.ConfigMode == "hybrid" || g.config.ConfigMode == "remote",
		RemoteFallbackDefault: g.config.ConfigMode != "remote",
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
	data := readmeTemplateData{
		Project:         ctx.project,
		IsProtoInput:    strings.EqualFold(ctx.source, "proto") || strings.HasSuffix(g.config.IDLSrcPath, ".proto"),
		WithSkill:       g.config.WithSkill,
		WithInteraction: g.config.WithInteraction,
		WithConfig:      g.config.WithConfig,
		WithDB:          g.config.WithDB,
		ConfigMode:      g.config.ConfigMode,
		RemoteProvider:  g.config.RemoteProvider,
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
	data := docsTemplateData{
		Services:   services,
		LeftDelim:  "{{",
		RightDelim: "}}",
	}
	return g.executeTemplate("docs.tmpl", path, data)
}

func (g *Generator) generateSkillFile(ctx generationContext) error {
	data := skillTemplateData{
		Project:    ctx.project,
		ImportPath: g.config.ImportPath,
	}
	return g.executeTemplate("skill.tmpl", g.layout.skillFile(), data)
}

func (g *Generator) generateInteractionFile(ctx generationContext) error {
	data := interactionTemplateData{
		Project:    ctx.project,
		Services:   ctx.services,
		ImportPath: g.config.ImportPath,
	}
	return g.executeTemplate("interaction.tmpl", g.layout.cmdGeneratedInteraction(), data)
}

func (g *Generator) generateAIProjectGuide(ctx generationContext) error {
	data := aiProjectGuideTemplateData{
		Project:         ctx.project,
		ImportPath:      g.config.ImportPath,
		WithConfig:      g.config.WithConfig,
		WithDB:          g.config.WithDB,
		WithGRPC:        g.config.WithGRPC,
		WithSwag:        g.config.WithSwag,
		WithSkill:       g.config.WithSkill,
		WithInteraction: g.config.WithInteraction,
	}
	return g.executeTemplate("ai_project_guide.tmpl", g.layout.aiProjectGuide(), data)
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

	data := goModTemplateData{
		ImportPath:   g.config.ImportPath,
		GoKitVersion: g.config.GoKitVersion,
		WithConfig:   g.config.WithConfig,
		RootRelPath:  g.rootRelativePath(),
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
	root := findGoKitModuleRoot(g.outputDir)
	if root == "" {
		return ""
	}
	if !isPathInside(root, g.outputDir) {
		return ""
	}
	rel, err := filepath.Rel(g.outputDir, root)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

func findGoKitModuleRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module github.com/dreamsxin/go-kit") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func isPathInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
