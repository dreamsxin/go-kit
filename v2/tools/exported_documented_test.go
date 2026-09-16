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

// docGateSkips are directories with no public surface to hold to the standard:
// internal packages are not importable, examples are sample code, testdata are
// fixtures, the gates live in their own module, and generated proto files are
// the work of a compiler, not a review.
var docGateSkips = map[string]bool{
	".comate":      true,
	".git":         true,
	".github":      true,
	"examples":     true,
	"internal":     true,
	"node_modules": true,
	"testdata":     true,
	"tools":        true,
}

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
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && docGateSkips[entry.Name()] {
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
					name, specDoc := specName(spec)
					if name == nil || !name.IsExported() {
						continue
					}
					if hasGroup || hasDocText(specDoc) {
						continue
					}
					missing = append(missing, missingDecl(relative, fset, spec.Pos(), name.Name))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(missing) > 0 {
		t.Fatalf("exported top-level declarations without a doc comment (%d):\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// specName returns the name a type or value spec declares and the doc attached
// to that spec itself, if any.
func specName(spec ast.Spec) (*ast.Ident, *ast.CommentGroup) {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return s.Name, s.Doc
	case *ast.ValueSpec:
		if len(s.Names) > 0 {
			return s.Names[0], s.Doc
		}
	}
	return nil, nil
}

func hasDocText(group *ast.CommentGroup) bool {
	return group != nil && strings.TrimSpace(group.Text()) != ""
}

func missingDecl(relative string, fset *token.FileSet, pos token.Pos, name string) string {
	return filepath.ToSlash(relative) + ":" + strconv.Itoa(fset.Position(pos).Line) + " " + name
}
