package tools_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var updateProtocolBehaviour = flag.Bool("update-protocol-behaviour", false,
	"update the reviewed protocol behaviour list")

// reviewedProtocolBehaviour is the reviewed set of protocol promises, each entry
// prefixed with whether it is one.
var reviewedProtocolBehaviour = reviewedList{
	subject:    "set of declared protocol behaviours",
	file:       "protocol_behaviour.txt",
	gained:     "newly declared",
	lost:       "no longer declared",
	updateFlag: "-update-protocol-behaviour",
	consequence: "A stable entry that disappears is a promise withdrawn from consumers who already depend on " +
		"it; one that appears is a promise the freeze holds for the life of v2.",
}

// behaviourMarker matches a declaration of protocol behaviour, written beside the
// code that decides it:
//
//	// Stable: http.error-envelope — the JSON error body is {"code","message"}.
//	// Covered by: TestJSONErrorEncoder_DefaultStatus
//
// Unstable takes the same form and needs no test: it records that the absence of
// a promise is deliberate.
var behaviourMarker = regexp.MustCompile(`^\s*//\s*(Stable|Unstable):\s*([a-z][a-z0-9.\-]*)\s*—\s*(\S.*)$`)

// behaviourCoverage names the tests that fail when a stable behaviour changes.
var behaviourCoverage = regexp.MustCompile(`^\s*//\s*Covered by:\s*(\S.*)$`)

var testFunctionDeclaration = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]*)\(`)

// behaviourScanSkips are directories with no protocol surface of their own:
// generated fixtures, sample programs, the gates themselves, and the templates,
// whose promises belong to the generated project rather than to this module.
var behaviourScanSkips = map[string]bool{
	".git":         true,
	".github":      true,
	"examples":     true,
	"node_modules": true,
	"templates":    true,
	"testdata":     true,
	"tools":        true,
}

type protocolBehaviour struct {
	kind  string
	id    string
	dir   string
	where string
	tests []string
}

// TestStableProtocolBehaviour pins which protocol behaviours v2 promises.
//
// The compatibility contract in internal/docs/RELEASE.md ends with "protocol
// behavior documented as stable". Nothing said which behaviours those were: the
// transports answer with particular status codes, error codes, header names, body
// shapes and JSON-RPC codes, and a reader had no way to tell a promise from an
// implementation detail that happens to work today. A freeze over an unnamed set
// is a freeze nobody can check.
//
// A promise is declared next to the code that keeps it, because that is where it
// gets broken. This gate reads those declarations and enforces three things: the
// set as a whole is reviewed, each promise names a test in its own package, and
// the named test exists.
func TestStableProtocolBehaviour(t *testing.T) {
	t.Parallel()
	root := goKitRoot(t)

	behaviours := scanProtocolBehaviours(t, root)
	stable := make([]string, 0, len(behaviours))
	entries := make([]string, 0, len(behaviours))
	declared := map[string]protocolBehaviour{}
	for _, behaviour := range behaviours {
		if previous, ok := declared[behaviour.id]; ok {
			t.Errorf("%s is declared twice, at %s and %s, so one of them can change unnoticed",
				behaviour.id, previous.where, behaviour.where)
			continue
		}
		declared[behaviour.id] = behaviour
		entries = append(entries, strings.ToLower(behaviour.kind)+"/"+behaviour.id)
		if behaviour.kind == "Stable" {
			stable = append(stable, behaviour.id)
		}
	}
	if len(stable) < 20 {
		// The transports promise far more than twenty behaviours between them, so
		// a short scan means the marker shape moved and this gate is comparing
		// almost nothing.
		t.Fatalf("found only %d stable protocol behaviours, so the declaration shape changed", len(stable))
	}

	for _, id := range stable {
		behaviour := declared[id]
		if len(behaviour.tests) == 0 {
			t.Errorf("%s (%s) names no test\n\nA promise nothing tests is a promise that breaks quietly.",
				id, behaviour.where)
			continue
		}
		known := packageTestFunctions(t, behaviour.dir)
		for _, name := range behaviour.tests {
			if !known[name] {
				t.Errorf("%s (%s) names %s, which no test in %s declares", id, behaviour.where, name,
					filepath.Base(behaviour.dir))
			}
		}
	}

	reviewedProtocolBehaviour.assert(t, entries, *updateProtocolBehaviour)
}

// scanProtocolBehaviours reads every behaviour declaration in the module.
func scanProtocolBehaviours(t *testing.T, root string) []protocolBehaviour {
	t.Helper()
	var behaviours []protocolBehaviour
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (behaviourScanSkips[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			relative = path
		}
		lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		for i, line := range lines {
			match := behaviourMarker.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			behaviours = append(behaviours, protocolBehaviour{
				kind:  match[1],
				id:    match[2],
				dir:   filepath.Dir(path),
				where: fmt.Sprintf("%s:%d", filepath.ToSlash(relative), i+1),
				tests: coveringTests(lines[i+1:]),
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan for protocol behaviour declarations: %v", err)
	}
	sort.Slice(behaviours, func(i, j int) bool { return behaviours[i].id < behaviours[j].id })
	return behaviours
}

// coveringTests reads the Covered by line belonging to a declaration, which is
// the rest of the same comment block.
func coveringTests(rest []string) []string {
	var names []string
	for _, line := range rest {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "//") {
			return names
		}
		match := behaviourCoverage.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		for _, name := range strings.Split(match[1], ",") {
			if name = strings.TrimSpace(name); name != "" {
				names = append(names, name)
			}
		}
		return names
	}
	return names
}

// packageTestFunctions reads the test functions declared beside an
// implementation, which is where a promise about it is proven.
func packageTestFunctions(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	names := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, match := range testFunctionDeclaration.FindAllStringSubmatch(string(data), -1) {
			names[match[1]] = true
		}
	}
	return names
}

// compatibilityContractDocs are the release documents that state the contract,
// each with the line the six covered surfaces follow.
var compatibilityContractDocs = []struct{ name, heading string }{
	{filepath.Join("internal", "docs", "RELEASE.md"), "compatibility contract at that point covers"},
	{filepath.Join("internal", "docs", "RELEASE_zh.md"), "届时的兼容性契约覆盖"},
}

var contractGateName = regexp.MustCompile("`(Test[A-Za-z0-9_]+)`")

// TestCompatibilityContractNamesItsGates keeps the contract and the gate suite in
// step: every surface the freeze covers names a test that exists.
//
// The contract used to be six bare nouns. Two of them were enforced by prose
// only and four in part, and nothing said so — the document read as a complete
// promise either way. Naming the gate is what makes the claim falsifiable, and a
// renamed or deleted gate now fails here instead of quietly leaving a surface
// unguarded.
func TestCompatibilityContractNamesItsGates(t *testing.T) {
	t.Parallel()
	root := goKitRoot(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	gates := packageTestFunctions(t, cwd)

	for _, doc := range compatibilityContractDocs {
		data, err := os.ReadFile(filepath.Join(root, doc.name))
		if err != nil {
			t.Fatalf("read %s: %v", doc.name, err)
		}
		surfaces := contractSurfaces(string(data), doc.heading)
		if len(surfaces) != 6 {
			t.Errorf("%s lists %d covered surfaces, want the 6 the contract names", doc.name, len(surfaces))
			continue
		}
		for _, surface := range surfaces {
			named := contractGateName.FindAllStringSubmatch(surface, -1)
			if len(named) == 0 {
				t.Errorf("%s names no gate for %q\n\nA surface with no gate is a promise the tooling "+
					"cannot hold.", doc.name, surface)
				continue
			}
			for _, match := range named {
				if !gates[match[1]] {
					t.Errorf("%s names %s for %q, which the tools package does not declare",
						doc.name, match[1], surface)
				}
			}
		}
	}
}

// contractSurfaces reads the bullet list that follows heading. Blank lines and
// the rest of the heading's own sentence come first; the list ends at the next
// line that is neither.
func contractSurfaces(text, heading string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if !strings.Contains(line, heading) {
			continue
		}
		var surfaces []string
		for _, candidate := range lines[i+1:] {
			trimmed := strings.TrimSpace(candidate)
			if strings.HasPrefix(trimmed, "- ") {
				surfaces = append(surfaces, strings.TrimPrefix(trimmed, "- "))
				continue
			}
			if len(surfaces) > 0 {
				return surfaces
			}
		}
		return surfaces
	}
	return nil
}

// TestProtocolBehaviourMarkerShape proves the scan reads a declaration the way
// the comments beside the transports are written, and rejects a malformed one.
// The real markers all parse, so they cannot show a rejection.
func TestProtocolBehaviourMarkerShape(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		"// Stable: http.error-envelope — the body is {\"code\",\"message\"}.",
		"\t// Unstable: http.access-log — the access line is a diagnostic.",
	} {
		if behaviourMarker.FindStringSubmatch(line) == nil {
			t.Errorf("a declaration was not read: %s", line)
		}
	}
	for _, line := range []string{
		"// Stable: http.error-envelope",              // no description
		"// Stable: HTTP.ErrorEnvelope — shouting.",   // not an id
		"// stable: http.error-envelope — lowercase.", // not a declaration
		"// See also: Stable behaviour lists.",
	} {
		if match := behaviourMarker.FindStringSubmatch(line); match != nil {
			t.Errorf("%q was read as a declaration of %q", line, match[2])
		}
	}

	block := []string{
		"// Covered by: TestOne, TestTwo",
		"// and some more prose.",
		"func Something() {}",
		"// Covered by: TestThree",
	}
	got := coveringTests(block)
	if len(got) != 2 || got[0] != "TestOne" || got[1] != "TestTwo" {
		t.Fatalf("covering tests = %v, want the two names in the same comment block", got)
	}
	if names := coveringTests(block[2:]); len(names) != 0 {
		t.Fatalf("covering tests = %v, want none once the comment block ends", names)
	}
}
