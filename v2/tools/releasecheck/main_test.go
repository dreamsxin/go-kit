package main

import (
	"path/filepath"
	"testing"
)

func TestResolveScopeFromNestedToolModule(t *testing.T) {
	repoRoot := t.TempDir()
	cwd := filepath.Join(repoRoot, "v2", "tools")

	scopeDir, scope, err := resolveScope(repoRoot, cwd, "..")
	if err != nil {
		t.Fatalf("resolveScope: %v", err)
	}
	wantDir := filepath.Join(repoRoot, "v2")
	if scopeDir != wantDir {
		t.Fatalf("scope dir = %q, want %q", scopeDir, wantDir)
	}
	if scope != "v2" {
		t.Fatalf("scope = %q, want v2", scope)
	}
}

func TestResolveScopeRejectsPathOutsideRepository(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	cwd := filepath.Join(repoRoot, "v2", "tools")

	if _, _, err := resolveScope(repoRoot, cwd, filepath.Join("..", "..", "..")); err == nil {
		t.Fatal("expected outside-repository scope to fail")
	}
}
