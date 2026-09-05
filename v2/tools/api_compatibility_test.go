package tools_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestAPICompatibilityWithLastRelease compares the exported surface at HEAD with
// the surface at the newest released tag and fails only on an incompatible
// change.
//
// TestPublicAPISurfaceSnapshot answers "did the surface change" with one digest
// per package, which cannot tell adding an exported function from deleting one:
// both show up as a different hex string, and the reviewer diffs `go doc` by hand
// to find out which happened. Once the compatibility freeze is declared that
// distinction is the whole contract, so it is asked here instead — a removal, a
// renamed symbol, a changed signature, or a changed struct field fails, and an
// addition passes.
//
// The old surface comes from a detached worktree at the tag rather than from the
// module proxy, so the comparison needs no network and no published version.
func TestAPICompatibilityWithLastRelease(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	repoRoot := strings.TrimSpace(string(normalizeCommandOutput(
		commandOutput(t, root, "git", "rev-parse", "--show-toplevel"))))

	tag, ok := newestReleaseTag(t, repoRoot)
	if !ok {
		t.Skip("no v2 release tag exists yet, so there is no published surface to stay compatible with")
	}

	previous := t.TempDir()
	// A detached worktree leaves the checkout alone: this suite runs alongside
	// others that read the working tree.
	commandOutput(t, repoRoot, "git", "worktree", "add", "--detach", "--quiet", previous, tag)
	t.Cleanup(func() {
		cmd := []string{"worktree", "remove", "--force", previous}
		if out, err := runQuiet(repoRoot, "git", cmd...); err != nil {
			t.Logf("remove temporary worktree: %v\n%s", err, out)
		}
	})

	previousRoot := filepath.Join(previous, filepath.Base(root))
	oldSurface := exportedSurface(t, previousRoot)
	newSurface := exportedSurface(t, root)

	problems := incompatibleChanges(oldSurface, newSurface)
	if len(problems) == 0 {
		return
	}
	t.Fatalf("the exported surface is not compatible with %s:\n%s\n\n"+
		"An addition is fine; these are removals or changes. If the break is deliberate, it needs a new major "+
		"module version once the compatibility freeze is declared — see internal/docs/RELEASE.md.",
		tag, strings.Join(problems, "\n"))
}

// TestAPICompatibilityClassifier proves the classifier reports what it claims,
// on inputs written here rather than on the real surface — the real surface is
// compatible with itself, so it cannot demonstrate a failure.
func TestAPICompatibilityClassifier(t *testing.T) {
	t.Parallel()
	const base = `package endpoint

FUNCTIONS

func Chain(outer Middleware, others ...Middleware) Middleware

TYPES

type Table struct {
	// Samples counts what has been recorded.
	Samples uint64
}

func (t *Table) Wrap(strategy Strategy) Strategy

CONSTANTS

const (
	StateReady State = "ready"
	StateDraining State = "draining"
	internalState State = "internal"
)
`
	tests := []struct {
		name    string
		changed string
		want    string
	}{
		{
			name:    "an addition is compatible",
			changed: base + "\nfunc Added() {}\n",
			want:    "",
		},
		{
			name: "rewording a field comment is compatible",
			changed: strings.Replace(base,
				"// Samples counts what has been recorded.",
				"// Samples is how many outcomes this table has seen.", 1),
			want: "",
		},
		{
			name:    "a removed function is incompatible",
			changed: strings.Replace(base, "func Chain(outer Middleware, others ...Middleware) Middleware\n", "", 1),
			want:    "removed: func Chain",
		},
		{
			name:    "a changed signature is incompatible",
			changed: strings.Replace(base, "others ...Middleware", "others []Middleware", 1),
			want:    "changed: func Chain",
		},
		{
			name:    "a removed method is incompatible",
			changed: strings.Replace(base, "func (t *Table) Wrap(strategy Strategy) Strategy\n", "", 1),
			want:    "removed: func Table.Wrap",
		},
		{
			name:    "a removed struct field is incompatible",
			changed: strings.Replace(base, "\tSamples uint64\n", "", 1),
			want:    "changed: type Table",
		},
		{
			name:    "a removed grouped constant is incompatible",
			changed: strings.Replace(base, "\tStateDraining State = \"draining\"\n", "", 1),
			want:    "removed: const StateDraining",
		},
		{
			name:    "removing an unexported name is compatible",
			changed: strings.Replace(base, "\tinternalState State = \"internal\"\n", "", 1),
			want:    "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			old := map[string]map[string]string{"pkg": apiSymbols([]byte(base))}
			updated := map[string]map[string]string{"pkg": apiSymbols([]byte(test.changed))}
			problems := incompatibleChanges(old, updated)
			joined := strings.Join(problems, "\n")
			if test.want == "" {
				if len(problems) != 0 {
					t.Fatalf("expected a compatible verdict, got:\n%s", joined)
				}
				return
			}
			if !strings.Contains(joined, test.want) {
				t.Fatalf("expected a report containing %q, got:\n%s", test.want, joined)
			}
		})
	}
}

// TestAPICompatibilityClassifierReportsARemovedPackage covers the coarsest break:
// a package that no longer exists cannot be imported at all.
func TestAPICompatibilityClassifierReportsARemovedPackage(t *testing.T) {
	t.Parallel()
	old := map[string]map[string]string{"gone": {"func A": "func A()"}}
	problems := incompatibleChanges(old, map[string]map[string]string{})
	if len(problems) != 1 || !strings.Contains(problems[0], "removed package: gone") {
		t.Fatalf("expected a removed-package report, got %v", problems)
	}
}

// exportedSurface reads the declarations of every reviewed package under root,
// keyed by import path.
func exportedSurface(t *testing.T, root string) map[string]map[string]string {
	t.Helper()
	surface := map[string]map[string]string{}
	for _, pkg := range publicRuntimePackages(t, root) {
		doc := commandOutput(t, pkg.root, "go", "doc", "-all", pkg.importPath)
		surface[pkg.importPath] = apiSymbols(declarationsOnly(normalizeCommandOutput(doc)))
	}
	return surface
}

// apiSymbols splits declaration text into one entry per exported symbol, keyed by
// a stable name so a symbol that changed shape can be told from one that is gone.
//
// A struct or interface body belongs to its type, because a removed field breaks
// a composite literal just as a removed method breaks a call. Entries of a
// grouped const or var block are separate symbols: they are declared together
// only for brevity.
//
// Comment lines inside a body are dropped, and unexported names are ignored.
// Without the first, rewording a field comment reads as an incompatible change,
// which is the reason doc prose was taken out of the surface digest in the first
// place. Without the second, the report names symbols no consumer can reach.
func apiSymbols(declarations []byte) map[string]string {
	symbols := map[string]string{}
	current := ""
	grouped := false
	for _, line := range strings.Split(string(declarations), "\n") {
		switch {
		case line == "" || isSectionHeader(line):
			current, grouped = "", false
		case strings.HasPrefix(line, "\t"):
			entry := strings.TrimSpace(line)
			if strings.HasPrefix(entry, "//") {
				continue
			}
			if grouped {
				if key := groupedEntryKey(current, entry); isExportedKey(key) {
					symbols[key] = entry
				}
				continue
			}
			if current != "" {
				symbols[current] += "\n" + entry
			}
		case line == "}" || line == ")":
			current, grouped = "", false
		default:
			key, isGroup := declarationKey(line)
			if key == "" || !isGroup && !isExportedKey(key) {
				current, grouped = "", false
				continue
			}
			current, grouped = key, isGroup
			if !isGroup {
				symbols[key] = line
			}
		}
	}
	return symbols
}

// isExportedKey reports whether every named part of a symbol key is exported. A
// method on an unexported type is unreachable, so both halves have to qualify.
func isExportedKey(key string) bool {
	space := strings.Index(key, " ")
	if space < 0 {
		return false
	}
	for _, part := range strings.Split(key[space+1:], ".") {
		if part == "" {
			return false
		}
		first := part[0]
		if first < 'A' || first > 'Z' {
			return false
		}
	}
	return true
}

func isSectionHeader(line string) bool {
	return line != "" && line == strings.ToUpper(line) && !strings.ContainsAny(line, " \t")
}

// declarationKey names the symbol a `go doc` declaration line declares. The
// second result reports a grouped const or var block, whose entries are named by
// their own lines rather than by this one.
func declarationKey(line string) (string, bool) {
	switch {
	case strings.HasPrefix(line, "package "):
		return "", false
	case strings.HasPrefix(line, "const (") || strings.HasPrefix(line, "var ("):
		return strings.TrimSuffix(strings.Fields(line)[0], " ("), true
	case strings.HasPrefix(line, "func "):
		return "func " + funcName(line), false
	case strings.HasPrefix(line, "type "):
		return "type " + identifier(strings.TrimPrefix(line, "type ")), false
	case strings.HasPrefix(line, "const "):
		return "const " + identifier(strings.TrimPrefix(line, "const ")), false
	case strings.HasPrefix(line, "var "):
		return "var " + identifier(strings.TrimPrefix(line, "var ")), false
	}
	return "", false
}

// funcName renders a function as Name and a method as Receiver.Name, so a method
// moving to a different receiver reads as one removal and one addition rather
// than as a change.
func funcName(line string) string {
	rest := strings.TrimPrefix(line, "func ")
	if !strings.HasPrefix(rest, "(") {
		return identifier(rest)
	}
	closing := strings.Index(rest, ")")
	if closing < 0 {
		return identifier(rest)
	}
	receiver := rest[1:closing]
	fields := strings.Fields(receiver)
	receiverType := strings.TrimPrefix(fields[len(fields)-1], "*")
	// Drop any type parameters: Table[T] and Table are the same receiver.
	if bracket := strings.IndexAny(receiverType, "["); bracket > 0 {
		receiverType = receiverType[:bracket]
	}
	return receiverType + "." + identifier(strings.TrimSpace(rest[closing+1:]))
}

// groupedEntryKey names one entry of a grouped const or var block.
func groupedEntryKey(keyword, entry string) string {
	name := identifier(entry)
	if name == "" {
		return ""
	}
	return keyword + " " + name
}

// identifier reads the leading Go identifier of a declaration fragment.
func identifier(text string) string {
	text = strings.TrimSpace(text)
	end := strings.IndexFunc(text, func(r rune) bool {
		return !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	})
	if end < 0 {
		return text
	}
	return text[:end]
}

// incompatibleChanges reports every way the new surface breaks a consumer of the
// old one. Additions are absent from the result by design.
func incompatibleChanges(old, updated map[string]map[string]string) []string {
	var problems []string
	for importPath, oldSymbols := range old {
		newSymbols, ok := updated[importPath]
		if !ok {
			problems = append(problems, "removed package: "+importPath)
			continue
		}
		for name, declaration := range oldSymbols {
			current, ok := newSymbols[name]
			switch {
			case !ok:
				problems = append(problems, fmt.Sprintf("%s: removed: %s", importPath, name))
			case current != declaration:
				problems = append(problems, fmt.Sprintf("%s: changed: %s\n    was: %s\n    now: %s",
					importPath, name, oneLine(declaration), oneLine(current)))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func oneLine(declaration string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(declaration, "\n", " ")), " ")
}

// runQuiet runs a command and returns its combined output with the error, for
// cleanup that should report a problem rather than fail the test.
func runQuiet(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// newestReleaseTag reports the highest vMAJOR.MINOR.PATCH tag in the repository.
func newestReleaseTag(t *testing.T, repoRoot string) (string, bool) {
	t.Helper()
	output := commandOutput(t, repoRoot, "git", "tag", "--list", "v2.*")
	best := ""
	var bestParts [3]int
	for _, tag := range strings.Fields(string(output)) {
		parts, ok := semverParts(tag)
		if !ok {
			continue
		}
		if best == "" || parts[0] > bestParts[0] ||
			parts[0] == bestParts[0] && parts[1] > bestParts[1] ||
			parts[0] == bestParts[0] && parts[1] == bestParts[1] && parts[2] > bestParts[2] {
			best, bestParts = tag, parts
		}
	}
	return best, best != ""
}

// semverParts parses vMAJOR.MINOR.PATCH, rejecting anything else so a
// pre-release or a stray tag cannot become the comparison baseline.
func semverParts(tag string) ([3]int, bool) {
	var parts [3]int
	fields := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if len(fields) != 3 {
		return parts, false
	}
	for i, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil {
			return parts, false
		}
		parts[i] = value
	}
	return parts, true
}
