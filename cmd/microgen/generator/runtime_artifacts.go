package generator

import (
	"os"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

func (g *Generator) generateServiceFileFull(service *parser.Service, models []*parser.Model, source parser.SourceType) error {
	data := map[string]any{
		"Service":    service,
		"Models":     models,
		"WithModel":  g.config.WithModel,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	return g.executeTemplate("service.tmpl", g.layout.serviceFile(service.ServiceName), data)
}

func (g *Generator) generateEndpointsFile(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	return g.executeTemplate("endpoints.tmpl", g.layout.endpointsFile(service.ServiceName), data)
}

func (g *Generator) generateHTTPTransportFile(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":     service,
		"ImportPath":  g.config.ImportPath,
		"RoutePrefix": routePrefix(g.config.RoutePrefix, service.ServiceName),
		"Source":      source,
	}
	return g.executeTemplate("transport.tmpl", g.layout.httpTransportFile(service.ServiceName), data)
}

func (g *Generator) generateGRPCTransportFile(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	return g.executeTemplate("transport_grpc.tmpl", g.layout.grpcTransportFile(service.ServiceName), data)
}

func (g *Generator) generateProtoFile(service *parser.Service) error {
	data := map[string]any{
		"Service": service,
	}
	if err := os.MkdirAll(g.layout.protoDir(service.ServiceName), 0o755); err != nil {
		return err
	}
	return g.executeTemplate("proto.tmpl", g.layout.protoFile(service.ServiceName), data)
}

func (g *Generator) generateTestFile(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	if err := os.MkdirAll(g.layout.testDir(), 0o755); err != nil {
		return err
	}
	return g.executeTemplate("service_test.tmpl", g.layout.serviceTestFile(service.ServiceName), data)
}

func (g *Generator) generateClientDemo(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
		"WithGRPC":   g.config.WithGRPC,
		"Source":     source,
	}
	return g.executeTemplate("client.tmpl", g.layout.clientDemoFile(service.ServiceName), data)
}

func (g *Generator) generateSDKFile(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":     service,
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
