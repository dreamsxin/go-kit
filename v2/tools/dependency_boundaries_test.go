package tools_test

import (
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

func TestTransportPackagesDoNotCrossProtocolBoundaries(t *testing.T) {
	root := moduleRoot(t)
	checks := []struct {
		pattern   string
		forbidden []string
	}{
		{
			pattern: "./transport",
			forbidden: []string{
				"net/http",
				"github.com/dreamsxin/go-kit/v2/transport/http",
				"github.com/dreamsxin/go-kit/v2/transport/grpc",
			},
		},
		{
			pattern: "./transport/http/...",
			forbidden: []string{
				"github.com/dreamsxin/go-kit/v2/transport/grpc",
				"google.golang.org/grpc",
			},
		},
		{
			pattern: "./transport/grpc/...",
			forbidden: []string{
				"github.com/dreamsxin/go-kit/v2/transport/http",
				"net/http",
			},
		},
	}

	for _, check := range checks {
		output := commandOutput(t, root, "go", "list", "-f", "{{.ImportPath}}|{{join .Imports \",\"}}", check.pattern)
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
	return goMod[:index]
}
