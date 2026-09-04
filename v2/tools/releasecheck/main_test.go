package main

import (
	"path/filepath"
	"testing"
)

func TestExpectedTagState(t *testing.T) {
	tests := []struct {
		phase string
		want  bool
	}{
		{phase: "candidate", want: false},
		{phase: "released", want: true},
	}
	for _, test := range tests {
		got, err := expectedTagState(test.phase)
		if err != nil {
			t.Fatalf("expectedTagState(%q): %v", test.phase, err)
		}
		if got != test.want {
			t.Errorf("expectedTagState(%q) = %t, want %t", test.phase, got, test.want)
		}
	}
	if _, err := expectedTagState("unknown"); err == nil {
		t.Fatal("expected unsupported phase to fail")
	}
}

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
