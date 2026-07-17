package tools_test

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateContractSnapshots = flag.Bool("update-contract-snapshots", false, "update reviewed generated contract snapshots")

func assertGeneratedContractSnapshot(t *testing.T, name, root string) {
	t.Helper()

	paths := []string{
		".microgen/manifest.json",
		"docs/openapi.json",
		"docs/schema.json",
		"sdk/typescript/client.ts",
	}
	paths = append(paths, matchingContractFiles(t, root, filepath.Join("sdk", "*sdk", "client.go"))...)
	paths = append(paths, matchingContractFiles(t, root, filepath.Join("pb", "*", "*.proto"))...)
	if _, err := os.Stat(filepath.Join(root, "idl.go")); err == nil {
		paths = append(paths, "idl.go")
	}

	paths = uniqueSortedPaths(paths)
	var snapshot strings.Builder
	fmt.Fprintf(&snapshot, "source %s\n", name)
	for _, relative := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read contract artifact %s: %v", relative, err)
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&snapshot, "%x  %s\n", sum, filepath.ToSlash(relative))
	}

	wantPath := filepath.Join("testdata", "contract_snapshots", name+".sha256")
	if *updateContractSnapshots || os.Getenv("UPDATE_CONTRACT_SNAPSHOTS") == "1" {
		if err := os.MkdirAll(filepath.Dir(wantPath), 0o755); err != nil {
			t.Fatalf("create contract snapshot directory: %v", err)
		}
		if err := os.WriteFile(wantPath, []byte(snapshot.String()), 0o644); err != nil {
			t.Fatalf("update contract snapshot %s: %v", name, err)
		}
	}

	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read contract snapshot %s: %v (rerun with -args -update-contract-snapshots to create it)", name, err)
	}
	if got := snapshot.String(); got != string(want) {
		t.Fatalf("generated %s contract changed\n--- want\n%s--- got\n%s\nreview the public contract, then rerun with -args -update-contract-snapshots", name, want, got)
	}
}

func matchingContractFiles(t *testing.T, root, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil {
		t.Fatalf("glob contract artifacts %q: %v", pattern, err)
	}
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		relative, err := filepath.Rel(root, match)
		if err != nil {
			t.Fatalf("resolve contract artifact %s: %v", match, err)
		}
		result = append(result, filepath.ToSlash(relative))
	}
	return result
}

func uniqueSortedPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}
