package tools_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestMarkdownDocumentsAreBilingualPairs enforces the guide's writing rule
// mechanically: every document is a pair, X.md and X_zh.md, and updating one
// without the other is a defect. The pairing is same-directory: the
// translation lives beside the original. Generated fixtures are not
// documentation and are skipped.
func TestMarkdownDocumentsAreBilingualPairs(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)

	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			relative, _ := filepath.Rel(root, path)
			if relative == filepath.Join("tools", "testdata") || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk documentation: %v", err)
	}
	if len(files) < 40 {
		t.Fatalf("scanned %d markdown files, want a full documentation tree; the walk is broken", len(files))
	}

	present := make(map[string]bool, len(files))
	for _, path := range files {
		present[filepath.ToSlash(path)] = true
	}
	for _, path := range files {
		name := filepath.Base(path)
		var counterpartName string
		switch {
		case strings.HasSuffix(name, "_zh.md"):
			counterpartName = strings.TrimSuffix(name, "_zh.md") + ".md"
		default:
			counterpartName = strings.TrimSuffix(name, ".md") + "_zh.md"
		}
		// Normalize after joining: filepath.Join re-inserts OS separators on
		// Windows, which would miss every key in a slash-built map.
		counterpart := filepath.ToSlash(filepath.Join(filepath.Dir(path), counterpartName))
		if !present[counterpart] {
			relative, _ := filepath.Rel(root, path)
			counterRelative, _ := filepath.Rel(root, filepath.FromSlash(counterpart))
			t.Errorf("%s has no bilingual counterpart %s",
				filepath.ToSlash(relative), filepath.ToSlash(counterRelative))
		}
	}
}

// walkDocPairs collects the bilingual pairs under the module, skipping
// fixtures, dot directories, and the two historical documents whose halves
// legitimately diverge release by release. The two gates below share it so
// their promises stay about the same corpus.
func walkDocPairs(t *testing.T, root string) map[string][2]string {
	t.Helper()
	pairs := map[string][2]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			relative, _ := filepath.Rel(root, path)
			if relative == filepath.Join("tools", "testdata") || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		relative = filepath.ToSlash(relative)
		base := strings.TrimSuffix(strings.TrimSuffix(relative, ".md"), "_zh")
		if base == "CHANGELOG" || base == filepath.ToSlash(filepath.Join("internal", "docs", "ROADMAP")) {
			return nil
		}
		current := pairs[base]
		if strings.HasSuffix(relative, "_zh.md") {
			current[1] = relative
		} else {
			current[0] = relative
		}
		pairs[base] = current
		return nil
	})
	if err != nil {
		t.Fatalf("walk documentation pairs: %v", err)
	}
	return pairs
}

// TestBilingualPairsKeepTheSectionStructure strengthens the pairing rule
// from "both files exist" to "both files tell the same story at the same
// granularity": outside fenced code blocks, the sequence of heading levels
// must match between the two halves. Heading text is the translation and is
// not compared; the structure is, because a section added to one language
// and not the other is exactly how a translation starts lying.
func TestBilingualPairsKeepTheSectionStructure(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	scanned := 0
	for _, pair := range walkDocPairs(t, root) {
		if pair[0] == "" || pair[1] == "" {
			continue // a missing half is the existence gate's finding
		}
		scanned++
		enLevels := headingLevels(filepath.Join(root, filepath.FromSlash(pair[0])))
		zhLevels := headingLevels(filepath.Join(root, filepath.FromSlash(pair[1])))
		if slices.Equal(enLevels, zhLevels) {
			continue
		}
		t.Errorf("%s and %s have different section structures: %d headings vs %d\n"+
			"A section in one language and not the other is a promise half-translated; "+
			"align the headings and retranslate the missing part.",
			pair[0], pair[1], len(enLevels), len(zhLevels))
	}
	if scanned < 40 {
		t.Fatalf("compared %d bilingual pairs, want the full documentation tree; the walk is broken", scanned)
	}
}

// headingLevels lists the ATX heading depths of a markdown file, skipping
// fenced code blocks where a # is a comment.
func headingLevels(path string) []int {
	var levels []int
	fenced := false
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if depth := len(trimmed) - len(strings.TrimLeft(trimmed, "#")); depth >= 2 && depth <= 6 &&
			strings.HasPrefix(trimmed, strings.Repeat("#", depth)) && strings.HasPrefix(strings.TrimLeft(trimmed, "#"), " ") {
			levels = append(levels, depth)
		}
	}
	return levels
}
