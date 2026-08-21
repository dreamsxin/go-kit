package main

import (
	"reflect"
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

func TestPublishableModulesExcludeRepositoryOnlyModules(t *testing.T) {
	got := sortedModuleNames(publishableModules("root"))
	want := []string{"consul", "core", "grpc", "kit-grpc", "microgen", "otel", "zap"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("publishable modules = %v, want %v", got, want)
	}
}
