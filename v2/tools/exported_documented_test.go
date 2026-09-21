package tools_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// docGateSkips are directories with no public surface to hold to the standard,
// wherever they sit: internal packages are not importable and testdata are
// fixtures. Dotted directories are skipped by skipGateDir, and generated proto
// files are the work of a compiler, not a review.
var docGateSkips = map[string]bool{
	"internal":     true,
	"node_modules": true,
	"testdata":     true,
}

// docGateRootSkips are exempt only directly under the module root: examples are
// sample code and the gates live in their own module. A package that happens to
// carry one of these names deeper in the tree is public surface.
var docGateRootSkips = map[string]bool{
	"examples": true,
	"tools":    true,
}

// docGateFloor is the number of exported declarations the walk must reach
// before a green result means anything. Without it the gate passes by scanning
// nothing — a root that resolved elsewhere or a skip that swallowed the tree
// both read as "no undocumented names". The module currently declares 837, so
// the floor sits below that with room for a package to be retired.
const docGateFloor = 700

// TestExportedDeclarationsAreDocumented holds the public surface of the
// published module to one standard: an exported top-level declaration — a
// function, type, variable, or constant — carries a doc comment, and a
// consumer opening the package never meets a name with no explanation.
//
// The rule is deliberately narrower than golint's. Methods are exempt, because
// their meaning comes from the interface they implement (Error, Unwrap, a
// clock's Now) and forcing prose onto each one is noise, not clarity. Internal
// packages and testdata are exempt, because a non-consumer does not read them.
// What is left is exactly the surface a consumer can name, and it is the part
// that must read coherently.
//
// The framework already meets this; the gate exists to keep a new exported
// name from shipping undocumented. Add a doc comment beside the declaration —
// the group comment that heads a const or var block counts for every member.
func TestExportedDeclarationsAreDocumented(t *testing.T) {
	t.Parallel()
	root := goKitRoot(t)

	var missing []string
	checked := 0
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && skipGateDir(root, path, entry.Name(), docGateSkips, docGateRootSkips) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".pb.go") {
			return nil // generated; the deliverer owns the prose
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return parseErr
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil || d.Name == nil || !d.Name.IsExported() {
					continue
				}
				checked++
				if !hasDocText(d.Doc) {
					missing = append(missing, missingDecl(relative, fset, d.Pos(), d.Name.Name))
				}
			case *ast.GenDecl:
				if d.Tok == token.IMPORT {
					continue
				}
				// A single group comment heads every member of a
				// const/var/type block; a per-spec comment heads that one.
				hasGroup := hasDocText(d.Doc)
				for _, spec := range d.Specs {
					names, specDoc := specNames(spec)
					documented := hasGroup || hasDocText(specDoc)
					for _, name := range names {
						if name == nil || !name.IsExported() {
							continue
						}
						checked++
						if documented {
							continue
						}
						missing = append(missing, missingDecl(relative, fset, spec.Pos(), name.Name))
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if checked < docGateFloor {
		t.Fatalf("only %d exported declarations scanned from %s, fewer than the floor of %d: the walk or the skip set changed, so a green result means nothing",
			checked, root, docGateFloor)
	}
	if len(missing) > 0 {
		t.Fatalf("exported top-level declarations without a doc comment (%d):\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// skipGateDir reports whether a module-wide walk should stop at this directory.
// Dotted directories hold tooling state, names in skips are exempt wherever
// they appear, and names in rootOnly are exempt only directly under root.
func skipGateDir(root, path, name string, skips, rootOnly map[string]bool) bool {
	if strings.HasPrefix(name, ".") || skips[name] {
		return true
	}
	return rootOnly[name] && filepath.Dir(path) == root
}

// specNames returns every name a type or value spec declares and the doc
// attached to that spec itself, if any. A value spec declares more than one
// name often enough — `var A, B = ...` — that reading only the first leaves the
// rest unchecked.
func specNames(spec ast.Spec) ([]*ast.Ident, *ast.CommentGroup) {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return []*ast.Ident{s.Name}, s.Doc
	case *ast.ValueSpec:
		return s.Names, s.Doc
	}
	return nil, nil
}

func hasDocText(group *ast.CommentGroup) bool {
	return group != nil && strings.TrimSpace(group.Text()) != ""
}

func missingDecl(relative string, fset *token.FileSet, pos token.Pos, name string) string {
	return filepath.ToSlash(relative) + ":" + strconv.Itoa(fset.Position(pos).Line) + " " + name
}
