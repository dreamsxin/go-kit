package tools_test

import (
	"bytes"
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
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	packages := publicRuntimePackages(t, root)

	var snapshot strings.Builder
	snapshot.WriteString("go-kit-v2 public API\n")
	for _, pkg := range packages {
		doc := commandOutput(t, pkg.root, "go", "doc", "-all", pkg.importPath)
		fmt.Fprintf(&snapshot, "%x  %s\n", sha256.Sum256(normalizeCommandOutput(doc)), pkg.importPath)
	}

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
	got := string(normalizeCommandOutput([]byte(snapshot.String())))
	wantText := string(normalizeCommandOutput(want))
	if got != wantText {
		t.Fatalf("public API surface changed\n--- want\n%s--- got\n%s\nreview the exported API change, then refresh with: make update-snapshots", wantText, got)
	}
}

type publicPackage struct {
	root       string
	importPath string
}

// publicRuntimePackages lists the packages whose exported surface is reviewed.
//
// Everything ships from one module now, so the list comes from one go list. The
// microgen tree is excluded: it is a command, and its behaviour is pinned by the
// generator contract tests rather than by a doc snapshot.
func publicRuntimePackages(t *testing.T, root string) []publicPackage {
	t.Helper()
	const microgenPrefix = "github.com/dreamsxin/go-kit/v2/cmd/microgen"

	output := commandOutput(t, root, "go", "list", "./...")
	var packages []publicPackage
	for _, importPath := range strings.Fields(string(output)) {
		if importPath == microgenPrefix || strings.HasPrefix(importPath, microgenPrefix+"/") {
			continue
		}
		packages = append(packages, publicPackage{root: root, importPath: importPath})
	}
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].importPath < packages[j].importPath
	})
	return packages
}

func commandOutput(t *testing.T, dir, name string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s%s", name, strings.Join(args, " "), err, stdout.Bytes(), stderr.Bytes())
	}
	return stdout.Bytes()
}

func normalizeCommandOutput(data []byte) []byte {
	return []byte(strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n")) + "\n")
}
