package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestChangedModuleFiles(t *testing.T) {
	base := moduleFilesState{
		"go.mod": []byte("module example.com/x\n\ngo 1.26.0\n"),
		"go.sum": []byte("example.com/y v1.0.0 h1:abc\n"),
	}

	tests := []struct {
		name   string
		after  moduleFilesState
		change []string
	}{
		{
			name:   "identical",
			after:  moduleFilesState{"go.mod": base["go.mod"], "go.sum": base["go.sum"]},
			change: nil,
		},
		{
			// A Windows checkout holds CRLF while the toolchain writes LF.
			// That is not a module change and must not fail the suite.
			name:   "line endings only",
			after:  moduleFilesState{"go.mod": []byte("module example.com/x\r\n\r\ngo 1.26.0\r\n"), "go.sum": base["go.sum"]},
			change: nil,
		},
		{
			name:   "requirement removed",
			after:  moduleFilesState{"go.mod": []byte("module example.com/x\n\ngo 1.26.0\n"), "go.sum": []byte{}},
			change: []string{"go.sum"},
		},
		{
			name:   "created",
			after:  moduleFilesState{"go.mod": base["go.mod"], "go.sum": base["go.sum"], "extra": []byte("x")},
			change: nil,
		},
		{
			name:   "go.sum appeared",
			after:  moduleFilesState{"go.mod": base["go.mod"]},
			change: []string{"go.sum"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := changedModuleFiles(base, tt.after); !reflect.DeepEqual(got, tt.change) {
				t.Fatalf("changedModuleFiles = %v, want %v", got, tt.change)
			}
		})
	}
}

func TestReadAndRestoreModuleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	before, err := readModuleFiles(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, ok := before["go.sum"]; ok {
		t.Fatal("a missing go.sum must not be recorded")
	}

	// tidy would create go.sum and rewrite go.mod; restore must undo both.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte("example.com/y v1.0.0 h1:abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := restoreModuleFiles(dir, before); err != nil {
		t.Fatalf("restore: %v", err)
	}

	restored, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != "module example.com/x\n" {
		t.Fatalf("go.mod = %q, want the original contents", restored)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.sum")); !os.IsNotExist(err) {
		t.Fatalf("go.sum should have been removed, stat err = %v", err)
	}
}
