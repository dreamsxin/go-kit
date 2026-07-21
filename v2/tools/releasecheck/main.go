package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const publishedModule = "github.com/dreamsxin/go-kit/v2"

func main() {
	publishedVersion := flag.String("published-version", "", "verify a published module version through the public Go proxy")
	flag.Parse()

	cwd, err := os.Getwd()
	if err != nil {
		fail("resolve working directory: %v", err)
	}
	repoRoot := strings.TrimSpace(run(cwd, "git", "rev-parse", "--show-toplevel"))
	scope, err := filepath.Rel(repoRoot, cwd)
	if err != nil || strings.HasPrefix(scope, "..") {
		fail("working directory %s is outside repository %s", cwd, repoRoot)
	}
	status := run(repoRoot, "git", "status", "--porcelain", "--untracked-files=all", "--", scope)
	if strings.TrimSpace(status) != "" {
		fail("release scope is not clean:\n%s", status)
	}
	run(repoRoot, "git", "diff", "--check", "HEAD", "--", scope)
	fmt.Printf("release scope is clean: %s\n", filepath.ToSlash(scope))
	if strings.TrimSpace(*publishedVersion) != "" {
		verifyPublished(*publishedVersion)
	}
}

func verifyPublished(version string) {
	tempDir, err := os.MkdirTemp("", "go-kit-published-check-")
	if err != nil {
		fail("create published-module check directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cmd := exec.Command("go", "list", "-m", "-json", publishedModule+"@"+version)
	cmd.Dir = tempDir
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://proxy.golang.org",
		"GOWORK=off",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		fail("published module %s is not resolvable: %v\n%s", version, err, stderr.String())
	}

	var module struct {
		Path    string
		Version string
	}
	if err := json.Unmarshal(stdout.Bytes(), &module); err != nil {
		fail("decode published module metadata: %v", err)
	}
	if module.Path != publishedModule || module.Version != version {
		fail("published module metadata mismatch: path=%q version=%q", module.Path, module.Version)
	}
	fmt.Printf("published module is resolvable: %s@%s\n", module.Path, module.Version)
}

func run(dir, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		fail("%s %s: %v\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String()
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
