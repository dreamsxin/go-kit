package tools_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func microgenMainPath(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	return filepath.Join(root, "cmd", "microgen", "main.go")
}

func TestMicrogenCLIValidation(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	microgenPath := microgenMainPath(t)

	t.Run("FailsWithoutIDLOrFromDB", func(t *testing.T) {
		cmd := exec.Command("go", "run", microgenPath)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("expected microgen to fail without -idl or -from-db")
		}
		if !strings.Contains(string(out), "either -idl or -from-db is required") {
			t.Fatalf("unexpected error output:\n%s", out)
		}
	})

	t.Run("FailsForMissingIDLPath", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_missing_idl")
		os.RemoveAll(outDir)

		cmd := exec.Command("go", "run", microgenPath,
			"-idl", filepath.Join(cwd, "testdata", "does-not-exist.go"),
			"-out", outDir,
			"-import", "example.com/gen_missing_idl",
		)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("expected microgen to fail for missing idl path")
		}
		outText := strings.ToLower(string(out))
		if !strings.Contains(outText, "no such file") && !strings.Contains(outText, "cannot find the file") {
			t.Fatalf("unexpected error output:\n%s", out)
		}
	})

	t.Run("FailsForUnsupportedDriver", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_bad_driver")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_bad_driver",
			"-driver", "oracle",
		)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("expected microgen to fail for unsupported driver")
		}
		if !strings.Contains(string(out), "unsupported db driver") {
			t.Fatalf("unexpected error output:\n%s", out)
		}
	})
}
