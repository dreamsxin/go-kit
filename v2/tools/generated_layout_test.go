package tools_test

import (
	"flag"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateGeneratedLayout = flag.Bool("update-generated-layout", false, "update the reviewed generated project layout")

// generatedProjectLayout is the reviewed set of files microgen writes.
var generatedProjectLayout = reviewedList{
	subject:     "generated project layout",
	file:        "generated_layout.txt",
	gained:      "newly generated",
	lost:        "no longer generated",
	updateFlag:  "-update-generated-layout",
	consequence: "A path that moves relocates a file the consumer owns and may have edited.",
}

// TestGeneratedLayout pins the set of files microgen writes, so the layout a
// consumer edits is a reviewed decision rather than whatever the templates
// currently happen to emit.
//
// The paths were already asserted, but incidentally: roughly sixty
// mustExistFile calls spread over nine test files, each added because some
// fixture needed that file. Coverage followed the fixtures, which left
// cmd/generated_runtime.go, cmd/generated_services.go and most of config/
// asserted nowhere. The contract snapshot does not close the gap either — its
// globs are not location pins, because a directory that stops emitting shrinks
// the snapshot rather than failing it, and the next `make update-snapshots`
// blesses the smaller set.
//
// One list, generated with every path-affecting flag on, is the upper bound of
// the layout. Turning a feature off removes paths from it, which is subtraction
// this list already describes; the combination below is the one the
// documentation shows.
func TestGeneratedLayout(t *testing.T) {
	root := goKitRoot(t)
	outDir := generatedProjectDir(t, "gen_layout")

	idlFile := filepath.Join(root, "cmd", "microgen", "internal", "parser", "testdata", "basic.go")
	cmd := microgenCommand(t,
		"-idl", idlFile,
		"-out", outDir,
		"-import", "example.com/gen_layout",
		"-protocols", "http,grpc",
		"-openapi",
		"-config",
		"-model",
		"-db",
		"-tests",
		"-interaction",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("microgen failed: %v\n%s", err, out)
	}

	generated := generatedFilePaths(t, outDir)
	if len(generated) < 20 {
		// A generator that silently stopped writing would otherwise leave this
		// test comparing two short lists and passing.
		t.Fatalf("microgen wrote only %d files, so the run did not do its job:\n%s",
			len(generated), strings.Join(generated, "\n"))
	}
	generatedProjectLayout.assert(t, generated, *updateGeneratedLayout)
}

// generatedFilePaths lists every file below root as a slash-separated relative
// path. Directories are left out: an empty directory is not something a consumer
// can edit, and the files inside already name their parents.
func generatedFilePaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatalf("walk generated project: %v", err)
	}
	sort.Strings(paths)
	return paths
}
