package tools_test

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateAPISnapshot = flag.Bool("update-api-snapshot", false, "update the reviewed public API snapshot")

func TestPublicAPISurfaceSnapshot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	packages := publicRuntimePackages(t, root)

	var snapshot strings.Builder
	snapshot.WriteString("go-kit-v2 public API\n")
	for _, packagePath := range packages {
		doc := commandOutput(t, root, "go", "doc", "-all", packagePath)
		fmt.Fprintf(&snapshot, "%x  %s\n", sha256.Sum256(normalizeCommandOutput(doc)), packagePath)
	}
	otelRoot := filepath.Join(root, "observability", "otel")
	otelModule := "github.com/dreamsxin/go-kit/v2/observability/otel"
	otelDoc := commandOutput(t, otelRoot, "go", "doc", "-all", ".")
	fmt.Fprintf(&snapshot, "%x  %s\n", sha256.Sum256(normalizeCommandOutput(otelDoc)), otelModule)

	snapshotPath := filepath.Join(cwd, "testdata", "api_surface.sha256")
	if *updateAPISnapshot {
		if err := os.WriteFile(snapshotPath, []byte(snapshot.String()), 0o644); err != nil {
			t.Fatalf("update API snapshot: %v", err)
		}
	}
	want, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read API snapshot: %v (rerun with -args -update-api-snapshot)", err)
	}
	if got := snapshot.String(); got != string(want) {
		t.Fatalf("public API surface changed\n--- want\n%s--- got\n%s\nreview exported API changes, then rerun with -args -update-api-snapshot", want, got)
	}
}

func publicRuntimePackages(t *testing.T, root string) []string {
	t.Helper()
	output := commandOutput(t, root, "go", "list", "./...")
	const modulePrefix = "github.com/dreamsxin/go-kit/v2/"
	var packages []string
	for _, packagePath := range strings.Fields(string(output)) {
		relative := strings.TrimPrefix(packagePath, modulePrefix)
		if relative == packagePath || strings.HasPrefix(relative, "cmd/") ||
			strings.HasPrefix(relative, "examples/") || relative == "tools" ||
			strings.HasPrefix(relative, "tools/") {
			continue
		}
		packages = append(packages, packagePath)
	}
	sort.Strings(packages)
	return packages
}

func commandOutput(t *testing.T, dir, name string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return output
}

func normalizeCommandOutput(data []byte) []byte {
	return []byte(strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n")) + "\n")
}
