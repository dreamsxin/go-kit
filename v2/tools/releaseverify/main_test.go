package main

import (
	"reflect"
	"sort"
	"testing"
)

func TestParseSuites(t *testing.T) {
	got, err := parseSuites("test, race, test")
	if err != nil {
		t.Fatalf("parseSuites: %v", err)
	}
	if want := []string{"test", "race"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("suites = %v, want %v", got, want)
	}
	if _, err := parseSuites("unknown"); err == nil {
		t.Fatal("expected unknown suite to fail")
	}
	if _, err := parseSuites(" , "); err == nil {
		t.Fatal("expected empty suite list to fail")
	}
}

// v2 ships as one module, so the standalone suite covers exactly one directory.
// examples and tools exercise the release; they are never required by anyone.
func TestPublishableModulesIsOnlyTheRootModule(t *testing.T) {
	if got := sortedModuleNames(publishableModules("root")); !reflect.DeepEqual(got, []string{"core"}) {
		t.Fatalf("publishable modules = %v, want [core]", got)
	}
	want := []string{"contractcheck", "core", "examples", "tools"}
	if got := sortedModuleNames(maintainedModules("root")); !reflect.DeepEqual(got, want) {
		t.Fatalf("maintained modules = %v, want %v", got, want)
	}
}

func sortedModuleNames(modules []module) []string {
	names := make([]string, 0, len(modules))
	for _, module := range modules {
		names = append(names, module.name)
	}
	sort.Strings(names)
	return names
}
