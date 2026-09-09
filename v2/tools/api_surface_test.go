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

// TestPublicAPISurfaceSnapshot pins the exported declarations of every runtime
// package, so an exported symbol cannot appear, disappear, or change shape
// without someone signing off.
//
// It hashes declarations only, not doc-comment prose. Prose used to be in the
// hash, which made a typo fix in a comment a release-gate event and put every
// documentation improvement behind a snapshot refresh. The guarantee worth
// keeping is the one tests structurally cannot give: no test asserts "these and
// only these symbols are exported", and for a published library an accidental
// export cannot be taken back. Whether a comment is accurate is a review
// question, not a hash question.
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
		declarations := declarationsOnly(normalizeCommandOutput(doc))
		fmt.Fprintf(&snapshot, "%x  %s\n", sha256.Sum256(declarations), pkg.importPath)
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
		t.Fatalf("public API surface changed in %s\n"+
			"A digest says only that something moved, so the packages are named here and the\n"+
			"declarations are yours to read:\n  go doc -all <import path>\n\n"+
			"--- want\n%s--- got\n%s\n"+
			"Review the exported API change, then refresh with: make update-snapshots",
			strings.Join(changedSurfacePackages(wantText, got), ", "), wantText, got)
	}
}

// changedSurfacePackages names the packages whose digest moved.
//
// The snapshot stores one hash per package, so a failure used to print two blocks
// of thirty-five hex strings and leave the reader to spot which line differed
// before they could even begin to work out what changed. Naming the packages does
// not make the digest reviewable — that needs a different stored form — but it
// removes the first, entirely mechanical step from the reviewer's job.
func changedSurfacePackages(want, got string) []string {
	previous := make(map[string]string)
	for _, line := range strings.Split(want, "\n") {
		if digest, pkg, ok := strings.Cut(strings.TrimSpace(line), "  "); ok {
			previous[pkg] = digest
		}
	}
	var changed []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(got, "\n") {
		digest, pkg, ok := strings.Cut(strings.TrimSpace(line), "  ")
		if !ok {
			continue
		}
		seen[pkg] = struct{}{}
		if before, existed := previous[pkg]; !existed {
			changed = append(changed, pkg+" (new package)")
		} else if before != digest {
			changed = append(changed, pkg)
		}
	}
	for pkg := range previous {
		if _, ok := seen[pkg]; !ok {
			changed = append(changed, pkg+" (gone)")
		}
	}
	sort.Strings(changed)
	if len(changed) == 0 {
		// The digests match line for line, so the difference is in the header or
		// in the ordering. Say so rather than printing an empty list.
		return []string{"no package digest differs — the header or package order changed"}
	}
	return changed
}

// declarationsOnly strips doc-comment prose from `go doc -all` output, keeping
// the declarations.
//
// The format it relies on: declarations and section headers sit at column 0,
// the body of a struct, interface, or grouped const/var block is indented with a
// tab, and prose is indented with four spaces. The package clause is kept; the
// package doc paragraph that follows it is at column 0 too, which is why the
// column-0 case matches on keywords rather than on indentation alone.
func declarationsOnly(doc []byte) []byte {
	var kept strings.Builder
	for _, line := range strings.Split(string(doc), "\n") {
		switch {
		case strings.HasPrefix(line, "\t"):
			// Struct field, interface method, or grouped const/var entry.
			kept.WriteString(line)
		case line == "}" || line == ")":
			kept.WriteString(line)
		case isDeclarationLine(line):
			kept.WriteString(line)
		default:
			continue
		}
		kept.WriteString("\n")
	}
	return []byte(kept.String())
}

func isDeclarationLine(line string) bool {
	for _, keyword := range []string{"package ", "type ", "func ", "const ", "var "} {
		if strings.HasPrefix(line, keyword) {
			return true
		}
	}
	// Section headers such as TYPES, CONSTANTS, FUNCTIONS, VARIABLES. Keeping
	// them means a symbol moving between sections is still a visible change.
	return line != "" && line == strings.ToUpper(line) && !strings.ContainsAny(line, " \t")
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
