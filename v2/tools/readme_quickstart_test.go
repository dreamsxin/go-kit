package tools_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadmeQuickStartSmoke(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	outDir := t.TempDir()

	idl := `package hello

import "context"

type HelloRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

type HelloResponse struct {
	Message string ` + "`json:\"message\"`" + `
}

type HelloService interface {
	// SayHello returns a greeting.
	SayHello(ctx context.Context, req HelloRequest) (HelloResponse, error)
}
`
	if err := os.WriteFile(filepath.Join(outDir, "idl.go"), []byte(idl), 0o644); err != nil {
		t.Fatalf("write readme smoke idl: %v", err)
	}

	binName := "microgen_readme_smoke"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(t.TempDir(), binName)
	buildMicrogenCmd := exec.Command("go", "build", "-o", binPath, "./cmd/microgen")
	buildMicrogenCmd.Dir = root
	runCommand(t, buildMicrogenCmd)

	generateCmd := exec.Command(binPath,
		"-idl", "idl.go",
		"-out", ".",
		"-import", "example.com/hello-svc",
		"-config=false",
		"-model=false",
		"-db=false",
	)
	generateCmd.Dir = outDir
	runCommand(t, generateCmd)

	replaceCmd := exec.Command("go", "mod", "edit", "-replace", "github.com/dreamsxin/go-kit/v2="+root)
	replaceCmd.Dir = outDir
	runCommand(t, replaceCmd)

	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = outDir
	runCommand(t, tidyCmd)

	testCmd := exec.Command("go", "test", "./...")
	testCmd.Dir = outDir
	runCommand(t, testCmd)

	mustExistFile(t, filepath.Join(outDir, "service", "helloservice", "service.go"))
	mustExistFile(t, filepath.Join(outDir, "skill", "skill.go"))
	mustNotExistFile(t, filepath.Join(outDir, "config", "config.go"))
	mustContainFile(t, filepath.Join(outDir, "README.md"), "## Agent Workflow")
}
