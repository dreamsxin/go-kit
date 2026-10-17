package tools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMarkdownDocumentsAreBilingualPairs enforces the guide's writing rule
// mechanically: every document is a pair, X.md and X_zh.md, and updating one
// without the other is a defect. Nothing else checked that — the agent-guide
// symmetry test compares what the two guides name, not whether other documents
// have a counterpart — so a one-sided doc addition passed until somebody read
// the directory.
//
// The pairing is same-directory: the translation lives beside the original.
// Generated fixtures are not documentation and are skipped.
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
	// Guard against a scan that quietly walks nothing: this tree carries dozens
	// of pairs, so a floor that low failing means the walk root or skip rules
	// broke, not that the documentation disappeared.
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
