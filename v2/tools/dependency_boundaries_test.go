package tools_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const coreModulePath = "github.com/dreamsxin/go-kit/v2"

func TestArchitectureDependencyGates(t *testing.T) {
	root := moduleRoot(t)
	checks := []struct {
		name         string
		pattern      string
		allowedExact []string
		allowedTrees []string
	}{
		{
			name:         "endpoint",
			pattern:      "./endpoint/...",
			allowedExact: []string{coreModulePath + "/apperror"},
			allowedTrees: []string{coreModulePath + "/endpoint"},
		},
		{
			name:         "http transport",
			pattern:      "./transport/http/...",
			allowedExact: []string{coreModulePath + "/apperror", coreModulePath + "/endpoint", coreModulePath + "/transport"},
			allowedTrees: []string{coreModulePath + "/transport/http"},
		},
		{
			name:         "service discovery",
			pattern:      "./sd/...",
			allowedExact: []string{coreModulePath + "/endpoint"},
			allowedTrees: []string{coreModulePath + "/sd"},
		},
		{
			name:         "kit",
			pattern:      "./kit",
			allowedExact: []string{coreModulePath + "/endpoint", coreModulePath + "/health", coreModulePath + "/sd", coreModulePath + "/transport"},
			allowedTrees: []string{coreModulePath + "/kit", coreModulePath + "/transport/http"},
		},
		{
			name:         "health",
			pattern:      "./health",
			allowedTrees: []string{coreModulePath + "/health"},
		},
		{
			// interaction reads the correlation identifiers a transport put in
			// the context, which is what makes a tool call attributable to the
			// request that carried it. endpoint is the stdlib-only layer that
			// owns that contract, so this is the whole allowance.
			name:         "interaction",
			pattern:      "./interaction",
			allowedExact: []string{coreModulePath + "/endpoint"},
			allowedTrees: []string{coreModulePath + "/interaction"},
		},
		{
			name:         "mcp interaction",
			pattern:      "./interaction/mcp",
			allowedExact: []string{coreModulePath + "/interaction"},
			allowedTrees: []string{coreModulePath + "/interaction/mcp"},
		},
		{
			name:         "http security",
			pattern:      "./security/http",
			allowedTrees: []string{coreModulePath + "/security/http"},
		},
		{
			name:         "security",
			pattern:      "./security",
			allowedExact: []string{coreModulePath + "/apperror", coreModulePath + "/endpoint"},
		},
		{
			name:         "slog observability",
			pattern:      "./observability/slog",
			allowedExact: []string{coreModulePath + "/endpoint", coreModulePath + "/transport"},
			allowedTrees: []string{coreModulePath + "/observability/slog"},
		},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			output := commandOutput(t, root, "go", "list", "-f", "{{.ImportPath}}|{{join .Imports \",\"}}", check.pattern)
			for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
				parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
				if len(parts) != 2 {
					t.Fatalf("unexpected go list output %q", line)
				}
				for _, importPath := range strings.Split(parts[1], ",") {
					if importPath == "" || isStandardLibraryImport(importPath) {
						continue
					}
					if !isAllowedImport(importPath, check.allowedExact, check.allowedTrees) {
						t.Errorf("%s imports package outside its dependency gate: %q", parts[0], importPath)
					}
				}
			}
		})
	}
}

func isStandardLibraryImport(importPath string) bool {
	firstSegment := strings.SplitN(importPath, "/", 2)[0]
	return !strings.Contains(firstSegment, ".")
}

func isAllowedImport(importPath string, exact, trees []string) bool {
	for _, allowed := range exact {
		if importPath == allowed {
			return true
		}
	}
	for _, allowed := range trees {
		if importPath == allowed || strings.HasPrefix(importPath, allowed+"/") {
			return true
		}
	}
	return false
}

// TestEndpointHasOnlyStandardLibraryImports keeps the contract layer at the
// bottom of the layering.
//
// endpoint defines what an Endpoint, a Middleware, and a Chain are. Nobody
// should have to adopt a framework policy — error taxonomy included — to use
// those three. It speaks the structural classification contract instead, the
// same one apperror documents for callers that must not depend on it.
func TestEndpointHasOnlyStandardLibraryImports(t *testing.T) {
	root := moduleRoot(t)
	output := commandOutput(t, root, "go", "list", "-f", "{{join .Imports \"\\n\"}}", "./endpoint")
	for _, importPath := range strings.Fields(string(output)) {
		if !isStandardLibraryImport(importPath) {
			t.Errorf("endpoint imports non-standard package %q", importPath)
		}
	}
}

// TestComponentsDoNotDependOnAssembly keeps the dependency direction one-way.
//
// The layering is: the contract (endpoint), then components that build on it,
// then the assembly that wires components into a service. A component reaching
// back into the assembly layer is what turns a set of libraries into a
// framework you cannot take pieces of.
func TestComponentsDoNotDependOnAssembly(t *testing.T) {
	root := moduleRoot(t)
	assembly := []string{
		coreModulePath + "/kit",
		coreModulePath + "/cmd/microgen",
	}
	isAssembly := func(importPath string) bool {
		for _, prefix := range assembly {
			if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
				return true
			}
		}
		return false
	}

	packages := strings.Fields(string(commandOutput(t, root, "go", "list", "./...")))
	for _, pkg := range packages {
		if isAssembly(pkg) || !strings.HasPrefix(pkg, coreModulePath) {
			continue
		}
		deps := strings.Fields(string(commandOutput(t, root, "go", "list", "-deps", pkg)))
		for _, dep := range deps {
			if isAssembly(dep) {
				t.Errorf("component %s depends on assembly package %s", pkg, dep)
			}
		}
	}
}

func TestMicrogenImplementationPackagesAreInternal(t *testing.T) {

	root := moduleRoot(t)
	microgenRoot := filepath.Join(root, "cmd", "microgen")
	output := commandOutput(t, microgenRoot, "go", "list", "-f", "{{.ImportPath}}", "./...")
	modulePath := "github.com/dreamsxin/go-kit/v2/cmd/microgen"
	for _, importPath := range strings.Fields(string(output)) {
		if importPath != modulePath && !strings.HasPrefix(importPath, modulePath+"/internal/") {
			t.Errorf("microgen exposes implementation package %q outside internal", importPath)
		}
	}
}

func TestGenericServiceDiscoveryDoesNotDependOnProviders(t *testing.T) {
	root := moduleRoot(t)
	packages := []string{
		"./sd",
		"./sd/balancer",
		"./sd/client",
		"./sd/endpointer",
		"./sd/instance",
		"./sd/retry",
	}
	args := []string{"list", "-deps", "-f", "{{.ImportPath}}"}
	args = append(args, packages...)
	output := commandOutput(t, root, "go", args...)
	for _, importPath := range strings.Fields(string(output)) {
		if strings.HasPrefix(importPath, "google.golang.org/grpc") ||
			strings.HasPrefix(importPath, "github.com/hashicorp/consul") {
			t.Errorf("generic service discovery resolves provider package %q", importPath)
		}
	}
}

// TestOptionalServiceDiscoveryLayersStayOptional pins the direction of the sd
// dependency graph. Feedback accounting and active probing are decorations a
// caller opts into, so the discovery, endpoint, and selection layers must not
// resolve them — otherwise "freely assemblable" would be true of the API and
// false of the build.
func TestOptionalServiceDiscoveryLayersStayOptional(t *testing.T) {
	root := moduleRoot(t)
	packages := []string{
		"./sd",
		"./sd/balancer",
		"./sd/client",
		"./sd/endpointer",
		"./sd/instance",
		"./sd/retry",
		"./sd/selector",
	}
	forbidden := []string{
		coreModulePath + "/sd/feedback",
		coreModulePath + "/sd/health",
	}

	args := []string{"list", "-deps", "-f", "{{.ImportPath}}"}
	args = append(args, packages...)
	output := commandOutput(t, root, "go", args...)
	for _, importPath := range strings.Fields(string(output)) {
		for _, optional := range forbidden {
			if importPath == optional {
				t.Errorf("core service discovery assemblies resolve optional package %q", optional)
			}
		}
	}
}

// TestKitHTTPAssemblyDoesNotResolveOptionalDependencies is the surviving form of
// the old module-level gate.
//
// Everything ships from one module now, so "the core module has no third-party
// requirements" is no longer a statement anyone can make. The goal it protected
// is still checkable, and at a more useful granularity: an HTTP-only assembly
// must not resolve gRPC, a discovery provider, a database driver, or a telemetry
// SDK. go list -deps answers that per package, which is what an application
// actually links.
func TestKitHTTPAssemblyDoesNotResolveOptionalDependencies(t *testing.T) {
	root := moduleRoot(t)
	output := commandOutput(t, root, "go", "list", "-deps", "-f", "{{.ImportPath}}", "./kit")
	forbidden := []string{
		coreModulePath + "/cmd/microgen",
		coreModulePath + "/integrations/consul",
		coreModulePath + "/integrations/etcd",
		coreModulePath + "/integrations/grpc",
		coreModulePath + "/integrations/zap",
		coreModulePath + "/kit/grpc",
		coreModulePath + "/observability/otel",
		"github.com/emicklei/proto",
		"github.com/go-sql-driver/mysql",
		"github.com/hashicorp/consul",
		"github.com/lib/pq",
		"github.com/mattn/go-sqlite3",
		"github.com/sony/gobreaker",
		"go.etcd.io/etcd",
		"go.opentelemetry.io/otel",
		"go.uber.org/zap",
		"google.golang.org/grpc",
		"modernc.org/sqlite",
	}
	for _, importPath := range strings.Fields(string(output)) {
		for _, prefix := range forbidden {
			if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
				t.Errorf("kit HTTP assembly resolves optional package %q", importPath)
			}
		}
	}
}

func TestTransportPackagesDoNotCrossProtocolBoundaries(t *testing.T) {
	root := moduleRoot(t)
	checks := []struct {
		dir       string
		pattern   string
		forbidden []string
	}{
		{
			dir:     root,
			pattern: "./transport",
			forbidden: []string{
				"net/http",
				"github.com/dreamsxin/go-kit/v2/transport/http",
				"github.com/dreamsxin/go-kit/v2/integrations/grpc",
			},
		},
		{
			dir:     root,
			pattern: "./transport/http/...",
			forbidden: []string{
				"github.com/dreamsxin/go-kit/v2/integrations/grpc",
				"google.golang.org/grpc",
			},
		},
		{
			dir:     filepath.Join(root, "integrations", "grpc"),
			pattern: "./...",
			forbidden: []string{
				"github.com/dreamsxin/go-kit/v2/transport/http",
				"net/http",
			},
		},
	}

	for _, check := range checks {
		output := commandOutput(t, check.dir, "go", "list", "-f", "{{.ImportPath}}|{{join .Imports \",\"}}", check.pattern)
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
			if len(parts) != 2 {
				t.Fatalf("unexpected go list output %q", line)
			}
			for _, importPath := range strings.Split(parts[1], ",") {
				for _, forbidden := range check.forbidden {
					if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
						t.Errorf("%s imports forbidden package %q", parts[0], importPath)
					}
				}
			}
		}
	}
}

// TestPublishableModulesDoNotUseLocalReplacements guards the one module that is
// published. examples and tools are workspace-only and may replace freely.
func TestPublishableModulesDoNotUseLocalReplacements(t *testing.T) {
	root := moduleRoot(t)
	type moduleVersion struct {
		Path    string
		Version string
	}
	type editJSON struct {
		Replace []struct {
			Old moduleVersion
			New moduleVersion
		}
	}

	output := commandOutput(t, root, "go", "mod", "edit", "-json")
	var edit editJSON
	if err := json.Unmarshal(output, &edit); err != nil {
		t.Fatalf("decode %s/go.mod: %v", root, err)
	}
	for _, replacement := range edit.Replace {
		if replacement.New.Version == "" {
			t.Errorf("published module uses local replace %s => %s", replacement.Old.Path, replacement.New.Path)
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	cwd := commandOutput(t, ".", "go", "env", "GOMOD")
	goMod := strings.TrimSpace(string(cwd))
	if goMod == "" || goMod == "/dev/null" || strings.EqualFold(goMod, "NUL") {
		t.Fatal("cannot locate go.mod")
	}
	index := strings.LastIndexAny(goMod, "/\\")
	if index < 0 {
		t.Fatalf("invalid go.mod path %q", goMod)
	}
	return filepath.Dir(filepath.Dir(goMod))
}
