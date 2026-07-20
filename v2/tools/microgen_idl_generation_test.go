package tools_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMicrogenIDLDefaultFlags(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	outDir := filepath.Join(cwd, "testdata", "gen_idl_default_flags")
	os.RemoveAll(outDir)

	idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
	cmd := exec.Command("go", "run", microgenMainPath(t),
		"-idl", idlFile,
		"-out", outDir,
		"-import", "example.com/gen_idl_default_flags",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("microgen idl default-flags failed: %v\n%s", err, out)
	}

	mustExistFile(t, filepath.Join(outDir, "go.mod"))
	mustExistFile(t, filepath.Join(outDir, "idl.go"))
	mustExistFile(t, filepath.Join(outDir, "service", "userservice", "service.go"))
	mustExistFile(t, filepath.Join(outDir, "endpoint", "userservice", "endpoints.go"))
	mustExistFile(t, filepath.Join(outDir, "endpoint", "userservice", "generated_chain.go"))
	mustExistFile(t, filepath.Join(outDir, "endpoint", "userservice", "custom_chain.go"))
	mustExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"))
	mustExistFile(t, filepath.Join(outDir, "client", "userservice", "demo.go"))
	mustExistFile(t, filepath.Join(outDir, "sdk", "userservicesdk", "client.go"))
	mustExistFile(t, filepath.Join(outDir, "config", "config.yaml"))
	mustExistFile(t, filepath.Join(outDir, "config", "config.go"))
	mustExistFile(t, filepath.Join(outDir, "config", "local.go"))
	mustExistFile(t, filepath.Join(outDir, "config", "env.go"))
	mustExistFile(t, filepath.Join(outDir, "config", "remote.go"))
	mustExistFile(t, filepath.Join(outDir, "config", "loader.go"))
	mustExistFile(t, filepath.Join(outDir, "README.md"))
	mustExistFile(t, filepath.Join(outDir, "model", "generated_user.go"))
	mustExistFile(t, filepath.Join(outDir, "repository", "generated_user_repository.go"))
	mustNotExistFile(t, filepath.Join(outDir, "docs", "docs.go"))
	mustNotExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_grpc.go"))
	mustNotExistFile(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"))
}
