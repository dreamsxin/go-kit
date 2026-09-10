package tools_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateContractSnapshots = flag.Bool("update-contract-snapshots", false, "update reviewed generated contract snapshots")

// contractIndexFile lists the artefacts under review for one source, so that an
// artefact the generator stops emitting is a failure rather than a golden file
// nobody notices.
const contractIndexFile = "files.txt"

// assertGeneratedContractSnapshot compares the public contract a generated project
// exposes against the reviewed copy in testdata.
//
// The reviewed copy is the artefacts themselves. It used to be one SHA-256 per
// file, which made this gate the least useful in the suite: a failure said that
// one of six generated documents had changed, and answering "how" meant running
// the generator into a temporary directory by hand and reading the output. That
// is how the OpenAPI defects fixed in v2.21.0 had to be reviewed — the gate had
// caught them for two releases without anyone being able to see them.
//
// Golden files are the standard answer for a generator, and here they are ~1,900
// lines per source. A template change rewrites them, which is the point: the diff
// is what a reviewer reads to decide whether the change to every generated service
// is the intended one.
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

	generated := make(map[string]string, len(paths))
	for _, relative := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read contract artifact %s: %v", relative, err)
		}
		generated[relative] = normalizeArtifact(data)
	}

	snapshotDir := filepath.Join("testdata", "contract_snapshots", name)
	// Only the flag refreshes a snapshot. An environment variable used to do it
	// too, which meant a value left in a shell profile or leaked into CI made all
	// three of these permanently self-blessing with nothing on any command line to
	// notice. A gate whose refresh can be armed out of band is not a gate.
	if *updateContractSnapshots {
		writeContractGolden(t, snapshotDir, paths, generated)
	}

	reviewed, err := os.ReadFile(filepath.Join(snapshotDir, contractIndexFile))
	if err != nil {
		t.Fatalf("read contract snapshot index for %s: %v (rerun with -args -update-contract-snapshots to create it)", name, err)
	}
	reviewedPaths := strings.Fields(normalizeArtifact(reviewed))

	if added := missingFrom(paths, reviewedPaths); len(added) > 0 {
		t.Errorf("generated %s contract exposes artifacts the reviewed set does not contain: %s\n"+
			"A new public artifact is a contract change: review it, then refresh with: make update-snapshots",
			name, strings.Join(added, ", "))
	}
	if gone := missingFrom(reviewedPaths, paths); len(gone) > 0 {
		t.Errorf("generated %s contract no longer emits reviewed artifacts: %s\n"+
			"Consumers of a generated project read these. Removing one is a breaking change.",
			name, strings.Join(gone, ", "))
	}

	for _, relative := range reviewedPaths {
		got, ok := generated[relative]
		if !ok {
			continue // already reported above
		}
		goldenPath := filepath.Join(snapshotDir, filepath.FromSlash(relative))
		wantData, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Errorf("read reviewed %s artifact %s: %v", name, relative, err)
			continue
		}
		want := normalizeArtifact(wantData)
		if want == got {
			continue
		}
		line, wantLine, gotLine := firstDifference(want, got)
		t.Errorf("generated %s contract changed: %s\n"+
			"  first difference at line %d\n    want: %s\n    got:  %s\n"+
			"The reviewed copy is the artifact, so the diff is the review:\n"+
			"  git diff -- v2/tools/%s\n"+
			"Once the change is intended, refresh with: make update-snapshots",
			name, relative, line, wantLine, gotLine, filepath.ToSlash(goldenPath))
	}
}

// writeContractGolden replaces the reviewed copy of one source's artefacts.
//
// It writes the index first and prunes afterwards, rather than clearing the
// directory and rebuilding it. Clearing first was simpler and wrong: the three
// integration tests that own these directories run in parallel with
// TestEveryContractSnapshotHasALiveCaller, which reads the directory listing and
// requires each source to have a files.txt. A refresh that deletes the directory
// leaves a window where that is briefly false, so `go test ./... -args
// -update-contract-snapshots` could fail on a snapshot it was in the middle of
// writing correctly.
//
// Pruning still has to happen: an artefact the generator no longer emits must
// disappear from the reviewed set, or the golden tree describes a generator that no
// longer exists.
func writeContractGolden(t *testing.T, snapshotDir string, paths []string, generated map[string]string) {
	t.Helper()
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		t.Fatalf("create contract snapshot directory: %v", err)
	}
	index := strings.Join(paths, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(snapshotDir, contractIndexFile), []byte(index), 0o644); err != nil {
		t.Fatalf("write contract snapshot index: %v", err)
	}

	keep := map[string]struct{}{contractIndexFile: {}}
	for _, relative := range paths {
		local := filepath.FromSlash(relative)
		keep[local] = struct{}{}
		goldenPath := filepath.Join(snapshotDir, local)
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("create contract snapshot directory: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(generated[relative]), 0o644); err != nil {
			t.Fatalf("write reviewed artifact %s: %v", relative, err)
		}
	}
	pruneContractGolden(t, snapshotDir, keep)
}

// pruneContractGolden deletes reviewed files the generator no longer writes, and
// the directories left empty behind them.
func pruneContractGolden(t *testing.T, snapshotDir string, keep map[string]struct{}) {
	t.Helper()
	var stale, directories []string
	err := filepath.WalkDir(snapshotDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(snapshotDir, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative != "." {
				directories = append(directories, path)
			}
			return nil
		}
		if _, ok := keep[relative]; !ok {
			stale = append(stale, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan contract snapshot directory: %v", err)
	}
	for _, path := range stale {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove stale reviewed artifact %s: %v", path, err)
		}
	}
	// Deepest first, and os.Remove refuses a directory that still has contents, so
	// this clears exactly the ones the pruning emptied.
	sort.Sort(sort.Reverse(sort.StringSlice(directories)))
	for _, path := range directories {
		_ = os.Remove(path)
	}
}

// normalizeArtifact makes the comparison independent of the checkout's line
// endings. This working copy is CRLF and the golden files are stored LF, so
// without it every artefact would differ on every line on Windows.
func normalizeArtifact(data []byte) string {
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// firstDifference locates the first line where two artefacts diverge, so the
// failure can quote it instead of the file.
func firstDifference(want, got string) (int, string, string) {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] != gotLines[i] {
			return i + 1, quoteArtifactLine(wantLines[i]), quoteArtifactLine(gotLines[i])
		}
	}
	switch {
	case len(gotLines) > len(wantLines):
		return len(wantLines) + 1, "(end of file)", quoteArtifactLine(gotLines[len(wantLines)])
	case len(wantLines) > len(gotLines):
		return len(gotLines) + 1, quoteArtifactLine(wantLines[len(gotLines)]), "(end of file)"
	}
	return 0, "", ""
}

// artifactLineQuota keeps one long generated line from burying the message.
const artifactLineQuota = 120

func quoteArtifactLine(line string) string {
	line = strings.TrimRight(line, " \t")
	if len(line) > artifactLineQuota {
		return line[:artifactLineQuota] + "…"
	}
	return line
}

// TestEveryContractSnapshotHasALiveCaller refuses a snapshot nobody reads.
//
// assertGeneratedContractSnapshot is a helper, not a Test, so the gate over the
// generated public contract exists only where somebody calls it. Deleting one call
// line produced no failure anywhere: the reviewed files stayed in the tree as files
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
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, err := os.Stat(filepath.Join(snapshotDir, name, contractIndexFile)); err != nil {
			t.Errorf("%s/%s has no %s, so nothing states which artifacts it reviews", snapshotDir, name, contractIndexFile)
			continue
		}
		snapshots++
		call := fmt.Sprintf("assertGeneratedContractSnapshot(t, %q", name)
		if !strings.Contains(haystack, call) {
			t.Errorf("%s/%s is stored but nothing calls %s.\n"+
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
