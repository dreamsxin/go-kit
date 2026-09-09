package tools_test

import (
	"bytes"
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
		sum := contractArtifactDigest(data)
		fmt.Fprintf(&snapshot, "%x  %s\n", sum, filepath.ToSlash(relative))
	}

	wantPath := filepath.Join("testdata", "contract_snapshots", name+".sha256")
	// Only the flag refreshes a snapshot. An environment variable used to do it
	// too, which meant a value left in a shell profile or leaked into CI made all
	// three of these permanently self-blessing with nothing on any command line to
	// notice. A gate whose refresh can be armed out of band is not a gate.
	if *updateContractSnapshots {
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
	got := string(normalizeCommandOutput([]byte(snapshot.String())))
	wantText := string(normalizeCommandOutput(want))
	if got != wantText {
		t.Fatalf("generated %s contract changed\n--- want\n%s--- got\n%s\nreview the public contract, then refresh with: make update-snapshots", name, wantText, got)
	}
}

func contractArtifactDigest(data []byte) [sha256.Size]byte {
	return sha256.Sum256(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
}

func TestContractArtifactDigestNormalizesLineEndings(t *testing.T) {
	lf := contractArtifactDigest([]byte("package sdk\n\nfunc Call() {}\n"))
	crlf := contractArtifactDigest([]byte("package sdk\r\n\r\nfunc Call() {}\r\n"))
	if lf != crlf {
		t.Fatalf("contract artifact digest differs by line ending: LF=%x CRLF=%x", lf, crlf)
	}
}

// TestEveryContractSnapshotHasALiveCaller refuses a snapshot nobody reads.
//
// assertGeneratedContractSnapshot is a helper, not a Test, so the gate over the
// generated public contract exists only where somebody calls it. Deleting one call
// line produced no failure anywhere: the .sha256 file stayed in the tree as a file
// nobody read, and the integration tests that host these calls are not among the
// gates RELEASE.md names, so the contract test that checks gates still exist could
// not see it either.
//
// This closes the hole from the other end — it walks the snapshots rather than the
// callers, so a stored snapshot has to be claimed by a call in this package.
func TestEveryContractSnapshotHasALiveCaller(t *testing.T) {
	t.Parallel()

	snapshotDir := filepath.Join("testdata", "contract_snapshots")
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatalf("read %s: %v", snapshotDir, err)
	}

	sources, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("list test sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("found no test sources to search, so this gate would pass vacuously")
	}
	var corpus strings.Builder
	for _, source := range sources {
		data, readErr := os.ReadFile(source)
		if readErr != nil {
			t.Fatalf("read %s: %v", source, readErr)
		}
		corpus.Write(data)
	}
	haystack := corpus.String()

	var snapshots int
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".sha256")
		if entry.IsDir() || !ok {
			continue
		}
		snapshots++
		call := fmt.Sprintf("assertGeneratedContractSnapshot(t, %q", name)
		if !strings.Contains(haystack, call) {
			t.Errorf("%s/%s.sha256 is stored but nothing calls %s.\n"+
				"Either the call was deleted, in which case that generated contract is no longer gated, "+
				"or the snapshot is stale and should be removed.", snapshotDir, name, call)
		}
	}
	if snapshots == 0 {
		t.Fatal("found no contract snapshots, so this gate would pass vacuously")
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
