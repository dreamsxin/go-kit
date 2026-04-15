package generator

import (
	"fmt"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
)

func (g *Generator) prepareProject(ctx generationContext) error {
	if err := g.createDirStructure(ctx.services); err != nil {
		return fmt.Errorf("create dir structure failed: %w", err)
	}

	if g.config.ImportPath != "" {
		if err := g.generateGoModFile(); err != nil {
			return fmt.Errorf("generate go.mod failed: %w", err)
		}
	}

	if g.shouldCopyIDLSource() {
		if err := g.copyIDLFile(g.config.IDLSrcPath); err != nil {
			return fmt.Errorf("copy idl file failed: %w", err)
		}
	}

	return nil
}

func (g *Generator) generateModelArtifacts(ctx generationContext) error {
	if !g.config.WithModel {
		return nil
	}

	var hasModels bool
	for _, model := range ctx.models {
		if !model.HasGormTags {
			continue
		}
		hasModels = true
		if err := g.generateModelFile(model); err != nil {
			return fmt.Errorf("generate model[%s] failed: %w", model.Name, err)
		}
		if err := g.generateRepositoryFile(model); err != nil {
			return fmt.Errorf("generate repository[%s] failed: %w", model.Name, err)
		}
	}
	if hasModels {
		if err := g.generateRepositoryBaseFile(); err != nil {
			return fmt.Errorf("generate repository base failed: %w", err)
		}
	}
	return nil
}

func (g *Generator) generateServiceArtifacts(ctx generationContext) error {
	for _, service := range ctx.services {
		var irService *ir.Service
		for _, candidate := range ctx.project.Services {
			if candidate.Name == service.ServiceName {
				irService = candidate
				break
			}
		}
		if err := g.generateServiceFileFull(service, ctx.models, irService, ctx.source); err != nil {
			return fmt.Errorf("generate service[%s] failed: %w", service.ServiceName, err)
		}
		if err := g.generateEndpointsFile(service, irService, ctx.source); err != nil {
			return fmt.Errorf("generate endpoints[%s] failed: %w", service.ServiceName, err)
		}
		if err := g.generateHTTPTransportFile(service, irService, ctx.source); err != nil {
			return fmt.Errorf("generate http transport[%s] failed: %w", service.ServiceName, err)
		}
		if g.config.WithGRPC {
			if err := g.generateGRPCTransportFile(service, irService, ctx.source); err != nil {
				return fmt.Errorf("generate grpc transport[%s] failed: %w", service.ServiceName, err)
			}
			if err := g.generateProtoFile(service, ctx.models, ctx.project); err != nil {
				return fmt.Errorf("generate proto[%s] failed: %w", service.ServiceName, err)
			}
		}
		if g.config.WithTests {
			if err := g.generateTestFile(service, irService, ctx.source); err != nil {
				return fmt.Errorf("generate test[%s] failed: %w", service.ServiceName, err)
			}
		}
		if err := g.generateClientDemo(service, irService, ctx.source); err != nil {
			return fmt.Errorf("generate client[%s] failed: %w", service.ServiceName, err)
		}
		if err := g.generateSDKFile(service, irService, ctx.source); err != nil {
			return fmt.Errorf("generate sdk[%s] failed: %w", service.ServiceName, err)
		}
	}

	return nil
}

func (g *Generator) generateFinalProjectArtifacts(ctx generationContext) error {
	if err := g.generateMainFileFull(ctx); err != nil {
		return fmt.Errorf("generate main failed: %w", err)
	}

	if g.config.WithConfig {
		if err := g.generateConfigFile(ctx.services); err != nil {
			return fmt.Errorf("generate config.yaml failed: %w", err)
		}
		if err := g.generateConfigCodeFile(ctx.services); err != nil {
			return fmt.Errorf("generate config.go failed: %w", err)
		}
	}

	if g.config.WithDocs {
		if err := g.generateReadme(ctx); err != nil {
			return fmt.Errorf("generate readme failed: %w", err)
		}
	}

	if g.config.WithSwag {
		if err := g.generateDocsStub(ctx.services); err != nil {
			return fmt.Errorf("generate docs stub failed: %w", err)
		}
	}

	if g.config.WithSkill {
		if err := g.generateSkillFile(ctx); err != nil {
			return fmt.Errorf("generate skill file failed: %w", err)
		}
	}

	return nil
}

func (g *Generator) shouldCopyIDLSource() bool {
	return g.config.IDLSrcPath != "" && !strings.HasSuffix(g.config.IDLSrcPath, ".proto")
}
