package generator

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

type projectLayout struct {
	root string
}

func newProjectLayout(root string) projectLayout {
	return projectLayout{root: root}
}

func (l projectLayout) cmdMain() string    { return filepath.Join(l.root, "cmd", "main.go") }
func (l projectLayout) configYAML() string { return filepath.Join(l.root, "config", "config.yaml") }
func (l projectLayout) configCode() string { return filepath.Join(l.root, "config", "config.go") }
func (l projectLayout) readme() string     { return filepath.Join(l.root, "README.md") }
func (l projectLayout) docsDir() string    { return filepath.Join(l.root, "docs") }
func (l projectLayout) docsStub() string   { return filepath.Join(l.docsDir(), "docs.go") }
func (l projectLayout) skillFile() string  { return filepath.Join(l.root, "skill", "skill.go") }
func (l projectLayout) goMod() string      { return filepath.Join(l.root, "go.mod") }
func (l projectLayout) idlCopy() string    { return filepath.Join(l.root, "idl.go") }
func (l projectLayout) modelFile() string  { return filepath.Join(l.root, "model", "model.go") }
func (l projectLayout) repositoryBaseFile() string {
	return filepath.Join(l.root, "repository", "base.go")
}
func (l projectLayout) repositoryFile() string {
	return filepath.Join(l.root, "repository", "repository.go")
}
func (l projectLayout) testDir() string                          { return filepath.Join(l.root, "test") }
func (l projectLayout) servicePackage(serviceName string) string { return strings.ToLower(serviceName) }

func (l projectLayout) modelHooksFile(modelName string) string {
	return filepath.Join(l.root, "model", strings.ToLower(modelName)+".go")
}

func (l projectLayout) serviceFile(serviceName string) string {
	return filepath.Join(l.root, "service", l.servicePackage(serviceName), "service.go")
}

func (l projectLayout) endpointsFile(serviceName string) string {
	return filepath.Join(l.root, "endpoint", l.servicePackage(serviceName), "endpoints.go")
}

func (l projectLayout) httpTransportFile(serviceName string) string {
	return filepath.Join(l.root, "transport", l.servicePackage(serviceName), "transport_http.go")
}

func (l projectLayout) grpcTransportFile(serviceName string) string {
	return filepath.Join(l.root, "transport", l.servicePackage(serviceName), "transport_grpc.go")
}

func (l projectLayout) protoDir(serviceName string) string {
	return filepath.Join(l.root, "pb", l.servicePackage(serviceName))
}

func (l projectLayout) protoFile(serviceName string) string {
	return filepath.Join(l.protoDir(serviceName), l.servicePackage(serviceName)+".proto")
}

func (l projectLayout) serviceTestFile(serviceName string) string {
	return filepath.Join(l.testDir(), l.servicePackage(serviceName)+"_test.go")
}

func (l projectLayout) clientDemoFile(serviceName string) string {
	return filepath.Join(l.root, "client", l.servicePackage(serviceName), "demo.go")
}

func (l projectLayout) sdkDir(serviceName string) string {
	return filepath.Join(l.root, "sdk", l.servicePackage(serviceName)+"sdk")
}

func (l projectLayout) sdkFile(serviceName string) string {
	return filepath.Join(l.sdkDir(serviceName), "client.go")
}

func (l projectLayout) requiredDirs(result *parser.ParseResult, opts Options) []string {
	dirs := []string{
		filepath.Join(l.root, "cmd"),
	}

	for _, svc := range result.Services {
		dirs = append(dirs,
			filepath.Join(l.root, "service", l.servicePackage(svc.ServiceName)),
			filepath.Join(l.root, "endpoint", l.servicePackage(svc.ServiceName)),
			filepath.Join(l.root, "transport", l.servicePackage(svc.ServiceName)),
			filepath.Join(l.root, "client", l.servicePackage(svc.ServiceName)),
			l.sdkDir(svc.ServiceName),
		)
	}

	if opts.WithConfig {
		dirs = append(dirs, filepath.Join(l.root, "config"))
	}
	if opts.WithModel {
		dirs = append(dirs,
			filepath.Join(l.root, "model"),
			filepath.Join(l.root, "repository"),
		)
	}
	if opts.WithSkill {
		dirs = append(dirs, filepath.Join(l.root, "skill"))
	}

	return dirs
}

func (l projectLayout) ensureDirs(result *parser.ParseResult, opts Options) error {
	for _, dir := range l.requiredDirs(result, opts) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func routePrefix(basePrefix, serviceName string) string {
	if basePrefix == "" {
		return ""
	}
	prefix := basePrefix
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix + strings.ToLower(serviceName)
}
