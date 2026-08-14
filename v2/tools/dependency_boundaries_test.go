package tools_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndpointHasOnlyStandardLibraryImports(t *testing.T) {
	root := moduleRoot(t)
	output := commandOutput(t, root, "go", "list", "-f", "{{join .Imports \"\\n\"}}", "./endpoint")
	for _, importPath := range strings.Fields(string(output)) {
		firstSegment := strings.SplitN(importPath, "/", 2)[0]
		if strings.Contains(firstSegment, ".") {
			t.Errorf("endpoint imports non-standard package %q", importPath)
		}
	}
}

func TestCoreModuleHasNoThirdPartyRequirements(t *testing.T) {
	root := moduleRoot(t)
	type editJSON struct {
		Require []struct {
			Path string
		}
	}

	output := commandOutput(t, root, "go", "mod", "edit", "-json")
	var edit editJSON
	if err := json.Unmarshal(output, &edit); err != nil {
		t.Fatalf("decode core go.mod: %v", err)
	}
	for _, requirement := range edit.Require {
		t.Errorf("core module requires third-party module %q", requirement.Path)
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

func TestKitHTTPAssemblyDoesNotResolveOptionalDependencies(t *testing.T) {
	root := moduleRoot(t)
	output := commandOutput(t, root, "go", "list", "-deps", "-f", "{{.ImportPath}}", "./kit")
	forbidden := []string{
		"github.com/dreamsxin/go-kit/v2/kit/grpc",
		"github.com/dreamsxin/go-kit/v2/integrations/zap",
		"github.com/go-sql-driver/mysql",
		"github.com/hashicorp/consul",
		"github.com/lib/pq",
		"github.com/mattn/go-sqlite3",
		"github.com/sony/gobreaker",
		"go.uber.org/zap",
		"google.golang.org/grpc",
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

func TestPublishableModulesDoNotUseLocalReplacements(t *testing.T) {
	root := moduleRoot(t)
	moduleRoots := []string{
		root,
		filepath.Join(root, "cmd", "microgen"),
		filepath.Join(root, "integrations", "circuitbreaker"),
		filepath.Join(root, "integrations", "consul"),
		filepath.Join(root, "integrations", "grpc"),
		filepath.Join(root, "integrations", "ratelimit"),
		filepath.Join(root, "integrations", "zap"),
		filepath.Join(root, "kit", "grpc"),
		filepath.Join(root, "observability", "otel"),
	}
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

	for _, moduleRoot := range moduleRoots {
		output := commandOutput(t, moduleRoot, "go", "mod", "edit", "-json")
		var edit editJSON
		if err := json.Unmarshal(output, &edit); err != nil {
			t.Fatalf("decode %s/go.mod: %v", moduleRoot, err)
		}
		for _, replacement := range edit.Replace {
			if replacement.New.Version == "" {
				t.Errorf("publishable module %s uses local replace %s => %s", moduleRoot, replacement.Old.Path, replacement.New.Path)
			}
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
