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
