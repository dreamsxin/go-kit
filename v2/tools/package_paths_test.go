package tools_test

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updatePackagePaths = flag.Bool("update-package-paths", false, "update the reviewed exported package path list")

// TestExportedPackagePaths pins the import paths the module publishes, so moving
// or renaming a package is a reviewed decision rather than a side effect.
//
// An import path is the most breaking thing a library owns: a consumer's import
// statement names it, and no amount of source compatibility saves a build from a
// path that moved. It was protected only incidentally until now — the second
// column of api_surface.sha256 happens to list the paths, so a rename showed up
// as a changed digest with no indication that a path was involved.
//
// This list covers cmd/microgen too, which the API digest excludes. The digest
// leaves it out because a command has no importable surface worth reviewing
// declaration by declaration; its path still matters, because `go install` and
// `go run` name it and the documentation tells readers to.
func TestExportedPackagePaths(t *testing.T) {
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)

	paths := strings.Fields(string(commandOutput(t, root, "go", "list", "./...")))
	sort.Strings(paths)
	listing := strings.Join(paths, "\n") + "\n"

	listingPath := filepath.Join(cwd, "testdata", "package_paths.txt")
	if *updatePackagePaths {
		if err := os.WriteFile(listingPath, []byte(listing), 0o644); err != nil {
			t.Fatalf("update package path list: %v", err)
		}
	}
	want, err := os.ReadFile(listingPath)
	if err != nil {
		t.Fatalf("read package path list: %v (rerun with -args -update-package-paths)", err)
	}

	added, removed := pathSetDifference(reviewedPaths(want), paths)
	if len(added) == 0 && len(removed) == 0 {
		return
	}
	var report strings.Builder
	for _, path := range removed {
		report.WriteString("\n  no longer published: " + path)
	}
	for _, path := range added {
		report.WriteString("\n  newly published:     " + path)
	}
	t.Fatalf("the set of published package paths changed:%s\n\n"+
		"A path that stops being published breaks every import of it, and cannot be taken back once released. "+
		"Refresh with: make update-snapshots", report.String())
}

// TestExportedPackagePathsReportsBothDirections proves the report names what
// moved, on inputs written here: the real list matches itself, so it cannot show
// a failure.
func TestExportedPackagePathsReportsBothDirections(t *testing.T) {
	t.Parallel()
	previous := []string{"example.com/a", "example.com/b"}
	current := []string{"example.com/a", "example.com/c"}
	added, removed := pathSetDifference(previous, current)
	if len(removed) != 1 || removed[0] != "example.com/b" {
		t.Fatalf("removed = %v, want example.com/b", removed)
	}
	if len(added) != 1 || added[0] != "example.com/c" {
		t.Fatalf("added = %v, want example.com/c", added)
	}

	added, removed = pathSetDifference(previous, previous)
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("an unchanged list reported added=%v removed=%v", added, removed)
	}
}

// reviewedPaths reads the stored list, tolerating the line endings a Windows
// checkout would otherwise introduce.
func reviewedPaths(listing []byte) []string {
	var paths []string
	for _, line := range strings.Fields(strings.ReplaceAll(string(listing), "\r\n", "\n")) {
		if line != "" {
			paths = append(paths, line)
		}
	}
	sort.Strings(paths)
	return paths
}

// pathSetDifference reports what current gained and lost relative to previous.
func pathSetDifference(previous, current []string) (added, removed []string) {
	have := make(map[string]struct{}, len(current))
	for _, path := range current {
		have[path] = struct{}{}
	}
	had := make(map[string]struct{}, len(previous))
	for _, path := range previous {
		had[path] = struct{}{}
	}
	for _, path := range previous {
		if _, ok := have[path]; !ok {
			removed = append(removed, path)
		}
	}
	for _, path := range current {
		if _, ok := had[path]; !ok {
			added = append(added, path)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}
