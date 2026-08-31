package tools_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// documentedPackageAliases maps the package aliases the v2 docs use to the
// directory owning that package. Only framework packages are listed;
// standard-library and third-party aliases are deliberately absent because
// this test guards the framework's own surface.
var documentedPackageAliases = map[string]string{
	"kit":           "kit",
	"kitgrpc":       filepath.Join("kit", "grpc"),
	"endpoint":      "endpoint",
	"apperror":      "apperror",
	"server":        filepath.Join("transport", "http", "server"),
	"httpserver":    filepath.Join("transport", "http", "server"),
	"grpcserver":    filepath.Join("integrations", "grpc", "server"),
	"transporthttp": filepath.Join("transport", "http"),
	"sdclient":      filepath.Join("sd", "client"),
	"security":      "security",
	"interaction":   "interaction",
	"mcp":           filepath.Join("interaction", "mcp"),
	"slogadapter":   filepath.Join("observability", "slog"),
	"oteladapter":   filepath.Join("observability", "otel"),
}

var frameworkAPIPattern = regexp.MustCompile(`\b(` + documentedAliasAlternation() + `)\.([A-Z]\w+)`)

// historicalDocumentation records past releases and therefore names APIs that
// no longer exist. Documenting removals is the point of these files, so they
// are exempt from the existence check.
var historicalDocumentation = map[string]struct{}{
	"CHANGELOG.md":       {},
	"CHANGELOG_zh.md":    {},
	"MIGRATION.md":       {},
	"MIGRATION_zh.md":    {},
	"ROADMAP.md":         {},
	"ROADMAP_zh.md":      {},
	"RELEASE.md":         {},
	"RELEASE_zh.md":      {},
	"ARCHITECTURE.md":    {},
	"ARCHITECTURE_zh.md": {},
}

// TestDocumentationReferencesExistingAPIs fails when a document recommends a
// framework symbol its own package does not declare. It complements the
// curated removed-API list in documentation_links_test.go: that list catches
// removals someone remembered to record, this catches every removal.
func TestDocumentationReferencesExistingAPIs(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)

	declared := make(map[string]map[string]struct{}, len(documentedPackageAliases))
	for alias, directory := range documentedPackageAliases {
		declared[alias] = exportedDeclarations(t, filepath.Join(root, directory))
	}

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		if _, ok := historicalDocumentation[entry.Name()]; ok {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		missing := make(map[string]struct{})
		for _, match := range frameworkAPIPattern.FindAllStringSubmatch(string(data), -1) {
			if _, ok := declared[match[1]][match[2]]; ok {
				continue
			}
			missing[match[0]] = struct{}{}
		}
		relative, _ := filepath.Rel(root, path)
		for _, reference := range sortedKeys(missing) {
			t.Errorf("%s references %s, which the package does not declare", filepath.ToSlash(relative), reference)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk documentation: %v", err)
	}
}

// exportedDeclarations collects the exported top-level names and method names
// declared by the Go files directly inside directory.
func exportedDeclarations(t *testing.T, directory string) map[string]struct{} {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}

	declared := make(map[string]struct{})
	fileSet := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				recordExported(declared, typed.Name)
			case *ast.GenDecl:
				for _, spec := range typed.Specs {
					switch specTyped := spec.(type) {
					case *ast.TypeSpec:
						recordExported(declared, specTyped.Name)
					case *ast.ValueSpec:
						for _, name := range specTyped.Names {
							recordExported(declared, name)
						}
					}
				}
			}
		}
	}
	if len(declared) == 0 {
		t.Fatalf("%s declares no exported names; documentedPackageAliases is stale", directory)
	}
	return declared
}

func recordExported(declared map[string]struct{}, name *ast.Ident) {
	if name != nil && name.IsExported() {
		declared[name.Name] = struct{}{}
	}
}

func documentedAliasAlternation() string {
	aliases := make([]string, 0, len(documentedPackageAliases))
	for alias := range documentedPackageAliases {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return strings.Join(aliases, "|")
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
