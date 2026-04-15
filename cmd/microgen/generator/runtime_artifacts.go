package generator

import (
	"os"

	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
)

func (g *Generator) generateServiceFileFull(service *serviceView, models []*modelView, irService *ir.Service, source string) error {
	data := map[string]any{
		"Service":    service,
		"IRService":  irService,
		"Models":     models,
		"WithModel":  g.config.WithModel,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	return g.executeTemplate("service.tmpl", g.layout.serviceFile(service.ServiceName), data)
}

func (g *Generator) generateEndpointsFile(service *serviceView, irService *ir.Service, source string) error {
	data := map[string]any{
		"Service":    service,
		"IRService":  irService,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	return g.executeTemplate("endpoints.tmpl", g.layout.endpointsFile(service.ServiceName), data)
}

func (g *Generator) generateHTTPTransportFile(service *serviceView, irService *ir.Service, source string) error {
	data := map[string]any{
		"Service":     service,
		"IRService":   irService,
		"ImportPath":  g.config.ImportPath,
		"RoutePrefix": routePrefix(g.config.RoutePrefix, service.ServiceName),
		"Source":      source,
	}
	return g.executeTemplate("transport.tmpl", g.layout.httpTransportFile(service.ServiceName), data)
}

func (g *Generator) generateGRPCTransportFile(service *serviceView, irService *ir.Service, source string) error {
	data := map[string]any{
		"Service":    service,
		"IRService":  irService,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	return g.executeTemplate("transport_grpc.tmpl", g.layout.grpcTransportFile(service.ServiceName), data)
}

func (g *Generator) generateProtoFile(service *serviceView, models []*modelView, project *ir.Project) error {
	schema := buildProtoSchema(service, models)
	var irService *ir.Service
	if project != nil {
		for _, candidate := range project.Services {
			if candidate.Name == service.ServiceName {
				irService = candidate
				break
			}
		}
		if irService != nil {
			schema = buildProtoSchemaFromIR(irService, project.Messages)
		}
	}
	data := map[string]any{
		"Service":        service,
		"IRService":      irService,
		"Messages":       schema.Messages,
		"NeedsTimestamp": schema.NeedsTimestamp,
		"NeedsDuration":  schema.NeedsDuration,
	}
	if err := os.MkdirAll(g.layout.protoDir(service.ServiceName), 0o755); err != nil {
		return err
	}
	return g.executeTemplate("proto.tmpl", g.layout.protoFile(service.ServiceName), data)
}

func (g *Generator) generateTestFile(service *serviceView, irService *ir.Service, source string) error {
	data := map[string]any{
		"Service":    service,
		"IRService":  irService,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	if err := os.MkdirAll(g.layout.testDir(), 0o755); err != nil {
		return err
	}
	return g.executeTemplate("service_test.tmpl", g.layout.serviceTestFile(service.ServiceName), data)
}

func (g *Generator) generateClientDemo(service *serviceView, irService *ir.Service, source string) error {
	data := map[string]any{
		"Service":     service,
		"IRService":   irService,
		"ImportPath":  g.config.ImportPath,
		"WithGRPC":    g.config.WithGRPC,
		"RoutePrefix": g.config.RoutePrefix,
		"Source":      source,
	}
	return g.executeTemplate("client.tmpl", g.layout.clientDemoFile(service.ServiceName), data)
}

func (g *Generator) generateSDKFile(service *serviceView, irService *ir.Service, source string) error {
	data := map[string]any{
		"Service":     service,
		"IRService":   irService,
		"ImportPath":  g.config.ImportPath,
		"WithGRPC":    g.config.WithGRPC,
		"Source":      source,
		"RoutePrefix": g.config.RoutePrefix,
	}
	if err := os.MkdirAll(g.layout.sdkDir(service.ServiceName), 0o755); err != nil {
		return err
	}
	return g.executeTemplate("sdk.tmpl", g.layout.sdkFile(service.ServiceName), data)
}
