package tools_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRepositoryTextFilesAreUTF8(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	repositoryRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))

	err = filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !repositoryTextFile(path) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			relative = path
		}
		if !utf8.Valid(data) {
			t.Errorf("%s is not valid UTF-8", relative)
		}
		if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
			t.Errorf("%s contains a UTF-8 BOM", relative)
		}
		if bytes.Contains(data, []byte("\uFFFD")) {
			t.Errorf("%s contains the Unicode replacement character", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
}

func repositoryTextFile(path string) bool {
	if strings.EqualFold(filepath.Base(path), "Makefile") {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".json", ".md", ".mod", ".proto", ".ps1", ".sh", ".sum", ".tmpl", ".toml", ".ts", ".txt", ".yaml", ".yml":
		return true
	default:
		return false
	}
}
