package tools_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func validateGeneratedContracts(t *testing.T, root string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	cmd := exec.Command("go", "run", ".",
		"-openapi", filepath.Join(root, "docs", "openapi.json"),
		"-schema", filepath.Join(root, "docs", "schema.json"),
	)
	cmd.Dir = filepath.Join(cwd, "contractcheck")
	runCommand(t, cmd)
}
