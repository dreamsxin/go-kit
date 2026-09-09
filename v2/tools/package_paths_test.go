package tools_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updatePackagePaths = flag.Bool("update-package-paths", false, "update the reviewed exported package path list")

// publishedPackagePaths is the reviewed set of import paths the module offers.
var publishedPackagePaths = reviewedList{
	subject:    "set of published package paths",
	file:       "package_paths.txt",
	gained:     "newly published",
	lost:       "no longer published",
	updateFlag: "-update-package-paths",
	consequence: "A path that stops being published breaks every import of it, and cannot be taken back once " +
		"released.",
}

// TestExportedPackagePaths pins the import paths the module publishes, so moving
// or renaming a package is a reviewed decision rather than a side effect.
//
// An import path is the most breaking thing a library owns: a consumer's import
// statement names it, and no amount of source compatibility saves a build from a
// path that moved. It was protected only incidentally at first — the API snapshot
// happens to name every package it covers, so a rename showed up there with no
// indication that a path was involved.
//
// This list covers cmd/microgen too, which the API snapshot excludes. That
// snapshot leaves it out because a command has no importable surface worth
// reviewing declaration by declaration; its path still matters, because
// `go install` and `go run` name it and the documentation tells readers to.
func TestExportedPackagePaths(t *testing.T) {
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)

	paths := strings.Fields(string(commandOutput(t, root, "go", "list", "./...")))
	publishedPackagePaths.assert(t, paths, *updatePackagePaths)
}
