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

	// Generated projects are ephemeral; compile their TypeScript SDK while the
	// fixture is still available instead of relying on checked-in output.
	typecheck := exec.Command("go", "run", "./typecheck", filepath.Join(root, "sdk", "typescript", "tsconfig.json"))
	typecheck.Dir = cwd
	runCommand(t, typecheck)
}
