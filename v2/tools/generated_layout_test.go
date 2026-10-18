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
// Two runs, every path-affecting flag on, are the upper bound of the layout:
// gRPC is a protobuf contract and model output needs a gorm-tagged Go IDL, so no
// single source mode can carry both. Turning a feature off removes paths from
// the union, which is subtraction this list already describes.
func TestGeneratedLayout(t *testing.T) {
	root := goKitRoot(t)

	// The HTTP project carries the database and interaction output.
	httpDir := generatedProjectDir(t, "gen_layout_http")
	httpIDL := filepath.Join(root, "cmd", "microgen", "internal", "parser", "testdata", "basic.go")
	httpCmd := microgenCommand(t,
		"-idl", httpIDL,
		"-out", httpDir,
		"-import", "example.com/gen_layout",
		"-protocols", "http",
		"-openapi",
		"-config",
		"-model",
		"-db",
		"-tests",
		"-interaction",
	)
	if out, err := httpCmd.CombinedOutput(); err != nil {
		t.Fatalf("microgen HTTP run failed: %v\n%s", err, out)
	}

	// The gRPC project carries the protobuf artifacts.
	grpcDir := generatedProjectDir(t, "gen_layout_grpc")
	grpcIDL := filepath.Join(root, "tools", "testdata", "service.proto")
	grpcCmd := microgenCommand(t,
		"-idl", grpcIDL,
		"-out", grpcDir,
		"-import", "example.com/gen_layout_grpc",
		"-protocols", "http,grpc",
		"-openapi",
		"-config",
		"-tests",
		"-interaction",
	)
	if out, err := grpcCmd.CombinedOutput(); err != nil {
		t.Fatalf("microgen gRPC run failed: %v\n%s", err, out)
	}

	generated := unionPaths(generatedFilePaths(t, httpDir), generatedFilePaths(t, grpcDir))
	if len(generated) < 20 {
		// A generator that silently stopped writing would otherwise leave this
		// test comparing two short lists and passing.
		t.Fatalf("microgen wrote only %d files, so the run did not do its job:\n%s",
			len(generated), strings.Join(generated, "\n"))
	}
	generatedProjectLayout.assert(t, generated, *updateGeneratedLayout)
}

// unionPaths merges path lists and returns them sorted with duplicates removed.
func unionPaths(lists ...[]string) []string {
	seen := make(map[string]struct{})
	var union []string
	for _, list := range lists {
		for _, path := range list {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			union = append(union, path)
		}
	}
	sort.Strings(union)
	return union
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
