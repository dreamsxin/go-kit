package main

import (
	"path/filepath"
	"testing"
)

func TestTagRequirementForPhase(t *testing.T) {
	tests := []struct {
		phase string
		want  tagRequirement
	}{
		{phase: "candidate", want: tagMustBeAbsent},
		{phase: "released", want: tagUnconstrained},
	}
	for _, test := range tests {
		got, err := tagRequirementFor(test.phase)
		if err != nil {
			t.Fatalf("tagRequirementFor(%q): %v", test.phase, err)
		}
		if got != test.want {
			t.Errorf("tagRequirementFor(%q) = %d, want %d", test.phase, got, test.want)
		}
	}
	if _, err := tagRequirementFor("unknown"); err == nil {
		t.Fatal("expected unsupported phase to fail")
	}
}

// The released phase deliberately says nothing about the local tag: recording
// the release is a commit, that commit runs this check, and requiring the tag
// would make the record commit and the tag push require each other.
func TestReleasedPhaseDoesNotRequireTheTag(t *testing.T) {
	requirement, err := tagRequirementFor("released")
	if err != nil {
		t.Fatalf("tagRequirementFor: %v", err)
	}
	if requirement == tagMustBeAbsent {
		t.Fatal("the released phase must not constrain the tag; -check-published proves publication")
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
