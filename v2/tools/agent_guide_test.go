package tools_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// agentGuides are the two halves of the repository guide an agent reads first.
var agentGuides = []string{"AGENTS.md", "AGENTS_zh.md"}

var (
	backtickedToken     = regexp.MustCompile("`([^`\n]+)`")
	makeTargetReference = regexp.MustCompile(`\bmake ([a-z][a-z0-9-]*)`)
	updateFlagReference = regexp.MustCompile(`-(update-[a-z-]+)`)
	testNameReference   = regexp.MustCompile(`\bTest[A-Za-z0-9_]+`)
)

// TestAgentGuideNamesThingsThatExist keeps AGENTS.md from becoming the most
// confidently wrong file in the repository.
//
// It is the document a coding agent reads before it knows anything else, so its
// mistakes are the expensive kind: a path that moved sends the reader to the
// wrong file, a refresh flag that was renamed makes a gate look bypassable, and a
// `make` target that no longer exists reads as "verification is optional here".
// Nothing else in the tree checks it — the documentation gates walk `v2`, and this
// file sits above it.
//
// What is checked is only what can be checked mechanically: every repository path
// it names exists, every `make` target is in the Makefile, every `-update-*` flag
// is registered, and every Test name is declared. Whether the advice is *good*
// remains a review question.
func TestAgentGuideNamesThingsThatExist(t *testing.T) {
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	repoRoot := filepath.Dir(root)

	makefile := readTextFile(t, filepath.Join(root, "Makefile"))
	declaredFlags := registeredUpdateFlags(t, cwd)
	declaredFuncs := declaredFunctionNames(t, root)

	references := make(map[string]guideReferences, len(agentGuides))
	for _, guide := range agentGuides {
		guidePath := filepath.Join(repoRoot, guide)
		text := readTextFile(t, guidePath)
		if strings.TrimSpace(text) == "" {
			t.Fatalf("%s is empty", guide)
		}
		found := collectGuideReferences(text)

		// A guide that names nothing cannot be wrong, and is also of no use. Each
		// category is required so that deleting a section silently is not a way to
		// pass this test.
		if len(found.paths) == 0 || len(found.makeTargets) == 0 ||
			len(found.updateFlags) == 0 || len(found.testNames) == 0 {
			t.Fatalf("%s names too little to be a guide: %d paths, %d make targets, %d update flags, %d tests",
				guide, len(found.paths), len(found.makeTargets), len(found.updateFlags), len(found.testNames))
		}

		for _, path := range found.paths {
			// Existence is the property worth checking. Whether the guide writes a
			// trailing slash on a directory is prose, not a claim.
			if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(strings.TrimSuffix(path, "/")))); err != nil {
				t.Errorf("%s names %s, which does not exist", guide, path)
			}
		}
		for _, target := range found.makeTargets {
			if !strings.Contains(makefile, "\n"+target+":") {
				t.Errorf("%s tells the reader to run `make %s`, which v2/Makefile does not define", guide, target)
			}
		}
		for _, flagName := range found.updateFlags {
			if _, ok := declaredFlags[flagName]; !ok {
				t.Errorf("%s names -%s, which no test in v2/tools registers", guide, flagName)
			}
		}
		for _, name := range found.testNames {
			if _, ok := declaredFuncs[name]; !ok {
				t.Errorf("%s names %s, which is not declared anywhere under v2", guide, name)
			}
		}
		references[guide] = found
	}

	// The two halves are one document. Naming a path or a flag in one language and
	// not the other is how a translated pair starts drifting, and it is exactly the
	// kind of omission a reader of the other language cannot notice.
	first, second := references[agentGuides[0]], references[agentGuides[1]]
	for _, difference := range []struct {
		what        string
		left, right []string
	}{
		{"paths", languageNeutralPaths(first.paths), languageNeutralPaths(second.paths)},
		{"make targets", first.makeTargets, second.makeTargets},
		{"update flags", first.updateFlags, second.updateFlags},
		{"test names", first.testNames, second.testNames},
	} {
		if missing := missingFrom(difference.left, difference.right); len(missing) > 0 {
			t.Errorf("%s names %s the translation does not: %s",
				agentGuides[0], difference.what, strings.Join(missing, ", "))
		}
		if missing := missingFrom(difference.right, difference.left); len(missing) > 0 {
			t.Errorf("%s names %s the English guide does not: %s",
				agentGuides[1], difference.what, strings.Join(missing, ", "))
		}
	}
}

// TestAgentGuideNamesEveryVersionBearingFile keeps the release-candidate checklist
// in the guide complete.
//
// Opening a candidate means writing the new version into every file that carries
// it, and the cost of missing one is paid at the worst moment: the release gate
// fails, or worse, a generated project pins a version that was never published.
// AGENTS.md lists those files, and a list of that kind rots — a new file starts
// carrying the version and nobody remembers the checklist exists.
//
// So the check runs the other way: every non-Markdown file under v2 that contains
// the manifest's version outside a comment has to be named in both guides.
// Markdown is excluded because a changelog and a roadmap talk about past releases
// by name, and a comment is excluded because prose about a past fix — "the defects
// fixed in v2.21.0" — is history, not a value to bump.
func TestAgentGuideNamesEveryVersionBearingFile(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	repoRoot := filepath.Dir(root)
	version := readReleaseManifest(t, root).CoreVersion
	if version == "" {
		t.Fatal("manifest has no coreVersion, so this gate would pass vacuously")
	}

	guides := make(map[string]string, len(agentGuides))
	for _, guide := range agentGuides {
		guides[guide] = readTextFile(t, filepath.Join(repoRoot, guide))
	}

	var carriers []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(withoutComments(entry.Name(), string(data)), version) {
			return nil
		}
		relative, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		carriers = append(carriers, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatalf("walk v2: %v", err)
	}
	if len(carriers) == 0 {
		t.Fatalf("no file under v2 carries %s, so this gate would pass vacuously", version)
	}

	sort.Strings(carriers)
	for _, carrier := range carriers {
		for _, guide := range agentGuides {
			if !strings.Contains(guides[guide], carrier) {
				t.Errorf("%s carries %s but %s does not name it in the release-candidate list.\n"+
					"Add it there, or the next candidate will be opened with this file left behind.",
					carrier, version, guide)
			}
		}
	}
}

// withoutComments removes line comments so that prose naming a past release is not
// mistaken for a version the next candidate has to update.
//
// It cuts at the first marker on a line, which also truncates a line whose URL
// contains one. That can only hide an occurrence, never invent one, and a version
// string inside a URL is not a value anybody bumps.
func withoutComments(name, text string) string {
	marker := "//"
	if name == "Makefile" || strings.HasSuffix(name, ".mk") {
		marker = "#"
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if at := strings.Index(line, marker); at >= 0 {
			lines[i] = line[:at]
		}
	}
	return strings.Join(lines, "\n")
}

type guideReferences struct {
	paths       []string
	makeTargets []string
	updateFlags []string
	testNames   []string
}

// collectGuideReferences pulls the checkable claims out of a guide.
//
// A backticked token counts as a repository path only when it starts with `v2/`.
// That deliberately excludes the module path `github.com/dreamsxin/go-kit/v2`,
// which looks like a path and is not one, and the `X.md` placeholders the guide
// uses to describe the pairing convention rather than a particular file.
func collectGuideReferences(text string) guideReferences {
	var found guideReferences
	for _, match := range backtickedToken.FindAllStringSubmatch(text, -1) {
		token := match[1]
		if strings.ContainsAny(token, " \t") || !strings.HasPrefix(token, "v2/") {
			continue
		}
		found.paths = appendUnique(found.paths, token)
	}
	for _, match := range makeTargetReference.FindAllStringSubmatch(text, -1) {
		found.makeTargets = appendUnique(found.makeTargets, match[1])
	}
	for _, match := range updateFlagReference.FindAllStringSubmatch(text, -1) {
		found.updateFlags = appendUnique(found.updateFlags, match[1])
	}
	for _, name := range testNameReference.FindAllString(text, -1) {
		found.testNames = appendUnique(found.testNames, name)
	}
	sort.Strings(found.paths)
	sort.Strings(found.makeTargets)
	sort.Strings(found.updateFlags)
	sort.Strings(found.testNames)
	return found
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func missingFrom(wanted, available []string) []string {
	present := make(map[string]struct{}, len(available))
	for _, value := range available {
		present[value] = struct{}{}
	}
	var missing []string
	for _, value := range wanted {
		if _, ok := present[value]; !ok {
			missing = append(missing, value)
		}
	}
	return missing
}

// languageNeutralPaths drops the `_zh` marker so the two halves can be compared.
//
// The translation is expected to point at the translated document — the Chinese
// guide sends its reader to ARCHITECTURE_zh.md — and treating that as drift would
// make the pairing check punish the correct behaviour.
func languageNeutralPaths(paths []string) []string {
	neutral := make([]string, 0, len(paths))
	for _, path := range paths {
		neutral = append(neutral, strings.Replace(path, "_zh.md", ".md", 1))
	}
	return neutral
}

// registeredUpdateFlags lists the -update-* flags the gate suite actually
// registers, so a renamed flag cannot keep living in the guide.
func registeredUpdateFlags(t *testing.T, toolsDir string) map[string]struct{} {
	t.Helper()
	flags := make(map[string]struct{})
	pattern := regexp.MustCompile(`flag\.Bool\("(update-[a-z-]+)"`)
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		t.Fatalf("read v2/tools: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		for _, match := range pattern.FindAllStringSubmatch(readTextFile(t, filepath.Join(toolsDir, entry.Name())), -1) {
			flags[match[1]] = struct{}{}
		}
	}
	if len(flags) == 0 {
		t.Fatal("found no -update-* flags in v2/tools: the scan is broken, not the guide")
	}
	return flags
}

// declaredFunctionNames collects every function declared under v2, which is
// enough to tell a live test name from one that was renamed or deleted.
func declaredFunctionNames(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	names := make(map[string]struct{})
	pattern := regexp.MustCompile(`(?m)^func (?:\([^)]*\) )?([A-Za-z0-9_]+)\(`)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
			names[match[1]] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk v2: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("found no function declarations under v2: the scan is broken, not the guide")
	}
	return names
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(normalizeCommandOutput(data))
}
