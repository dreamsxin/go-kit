package tools_test

import (
	"bytes"
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

// apiSurfaceHeader opens the snapshot so a stray file cannot be mistaken for it.
const apiSurfaceHeader = "go-kit-v2 public API"

// apiSurfaceSectionPrefix separates the per-package sections.
const apiSurfaceSectionPrefix = "## "

// TestPublicAPISurfaceSnapshot pins the exported declarations of every runtime
// package, so an exported symbol cannot appear, disappear, or change shape
// without someone signing off.
//
// It hashes nothing. The snapshot used to store one digest per package, which
// made the gate cheap to store and impossible to review: a failure said that
// something in some package had moved, the refresh command made it green again,
// and no diff a human read ever contained the declaration that changed. The
// reviewed form is now the declarations themselves, so `git diff` on the snapshot
// answers the only question the gate exists to ask — what changed, and was it
// meant to.
//
// Declarations only, not doc-comment prose. Prose used to be in the hash, which
// made a typo fix in a comment a release-gate event. The guarantee worth keeping
// is the one tests structurally cannot give: no test asserts "these and only
// these symbols are exported", and for a published library an accidental export
// cannot be taken back. Whether a comment is accurate is a review question.
func TestPublicAPISurfaceSnapshot(t *testing.T) {
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	packages := publicRuntimePackages(t, root)

	var snapshot strings.Builder
	snapshot.WriteString(apiSurfaceHeader + "\n")
	for _, pkg := range packages {
		doc := commandOutput(t, pkg.root, "go", "doc", "-all", pkg.importPath)
		declarations := declarationsOnly(normalizeCommandOutput(doc))
		fmt.Fprintf(&snapshot, "\n%s%s\n%s", apiSurfaceSectionPrefix, pkg.importPath, declarations)
	}

	snapshotPath := filepath.Join(cwd, "testdata", "api_surface.txt")
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
	if got == wantText {
		return
	}

	changed := changedSurfacePackages(parseSurfaceSections(wantText), parseSurfaceSections(got))
	if len(changed) == 0 {
		t.Fatalf("the public API snapshot differs but no package section does: the header or the package order changed\n" +
			"Read: git diff -- v2/tools/testdata/api_surface.txt")
	}
	t.Fatalf("public API surface changed in %d package(s):\n%s\n"+
		"The snapshot holds the declarations, so the diff is the review:\n"+
		"  git diff -- v2/tools/testdata/api_surface.txt\n\n"+
		"Once the change is intended, refresh with: make update-snapshots",
		len(changed), strings.Join(changed, "\n"))
}

// parseSurfaceSections splits a snapshot into its per-package declaration text.
// The first block is the header, which belongs to no package.
func parseSurfaceSections(snapshot string) map[string]string {
	sections := make(map[string]string)
	blocks := strings.Split(snapshot, "\n"+apiSurfaceSectionPrefix)
	for _, block := range blocks[1:] {
		if importPath, declarations, ok := strings.Cut(block, "\n"); ok {
			sections[importPath] = declarations
		}
	}
	return sections
}

// changedSurfacePackages reports, per package, which declaration lines appeared
// and which disappeared.
//
// The failure message quotes the lines rather than the whole snapshot. Printing
// both versions of a file this size is what made the digest failure useless in a
// different way — a reader who has to scroll past three thousand lines is not
// reviewing either.
func changedSurfacePackages(want, got map[string]string) []string {
	var changed []string
	for importPath, declarations := range got {
		previous, existed := want[importPath]
		if !existed {
			changed = append(changed, fmt.Sprintf("  %s (new package, %d declaration line(s))",
				importPath, len(nonEmptyLines(declarations))))
			continue
		}
		if previous == declarations {
			continue
		}
		added := linesMissingFrom(declarations, previous)
		removed := linesMissingFrom(previous, declarations)
		changed = append(changed, fmt.Sprintf("  %s\n%s%s", importPath,
			quoteSurfaceLines("+ ", added), quoteSurfaceLines("- ", removed)))
	}
	for importPath := range want {
		if _, ok := got[importPath]; !ok {
			changed = append(changed, fmt.Sprintf("  %s (gone)", importPath))
		}
	}
	sort.Strings(changed)
	return changed
}

// surfaceLineQuota caps how many lines of one package's change are quoted. A
// change larger than this is a rewrite, and the file diff is the right place to
// read it.
const surfaceLineQuota = 20

func quoteSurfaceLines(marker string, lines []string) string {
	var quoted strings.Builder
	for i, line := range lines {
		if i == surfaceLineQuota {
			fmt.Fprintf(&quoted, "    %s... and %d more\n", marker, len(lines)-i)
			break
		}
		fmt.Fprintf(&quoted, "    %s%s\n", marker, strings.TrimSpace(line))
	}
	return quoted.String()
}

func nonEmptyLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// linesMissingFrom reports the lines of subject that other does not contain.
//
// It compares as sets, which is enough for the question asked: a declaration
// that moved within a package reads as one removal and one addition, and that is
// a change worth someone's eyes anyway.
func linesMissingFrom(subject, other string) []string {
	present := make(map[string]int)
	for _, line := range nonEmptyLines(other) {
		present[line]++
	}
	var missing []string
	for _, line := range nonEmptyLines(subject) {
		if present[line] > 0 {
			present[line]--
			continue
		}
		missing = append(missing, line)
	}
	return missing
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
