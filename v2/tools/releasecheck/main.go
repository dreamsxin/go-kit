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

	"github.com/dreamsxin/go-kit-tools/v2/internal/releaseconfig"
)

const publishedModule = "github.com/dreamsxin/go-kit/v2"

func main() {
	publishedVersion := flag.String("published-version", "", "verify a published module version through the public Go proxy")
	requestedScope := flag.String("scope", ".", "repository-relative scope to verify, resolved from the current directory")
	manifestPath := flag.String("manifest", "", "release manifest path, resolved from the current directory")
	checkTags := flag.Bool("check-tags", false, "verify local tag presence matches the manifest phase")
	checkPublished := flag.Bool("check-published", false, "verify the manifest version resolves through the public Go proxy")
	flag.Parse()

	cwd, err := os.Getwd()
	if err != nil {
		fail("resolve working directory: %v", err)
	}
	repoRoot := strings.TrimSpace(run(cwd, "git", "rev-parse", "--show-toplevel"))
	scopeDir, scope, err := resolveScope(repoRoot, cwd, *requestedScope)
	if err != nil {
		fail("resolve release scope: %v", err)
	}
	status := run(repoRoot, "git", "status", "--porcelain", "--untracked-files=all", "--", scope)
	if strings.TrimSpace(status) != "" {
		fail("release scope is not clean:\n%s", status)
	}
	run(repoRoot, "git", "diff", "--check", "HEAD", "--", scope)
	fmt.Printf("release scope is clean: %s\n", filepath.ToSlash(scopeDir))
	if strings.TrimSpace(*publishedVersion) != "" {
		verifyPublishedModule(publishedModule, *publishedVersion)
	}
	if strings.TrimSpace(*manifestPath) == "" {
		return
	}
	resolvedManifest, err := filepath.Abs(filepath.Join(cwd, *manifestPath))
	if err != nil {
		fail("resolve release manifest: %v", err)
	}
	manifest, err := releaseconfig.LoadManifest(resolvedManifest)
	if err != nil {
		fail("load release manifest: %v", err)
	}
	if *checkTags {
		if err := verifyTagState(repoRoot, manifest); err != nil {
			fail("verify release tags: %v", err)
		}
		fmt.Printf("release tag matches phase: %s\n", manifest.Phase)
	}
	if *checkPublished {
		verifyPublishedModule(publishedModule, manifest.CoreVersion)
	}
}

func resolveScope(repoRoot, cwd, requestedScope string) (string, string, error) {
	scopeDir, err := filepath.Abs(filepath.Join(cwd, requestedScope))
	if err != nil {
		return "", "", err
	}
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		return "", "", err
	}
	scope, err := filepath.Rel(repoRoot, scopeDir)
	if err != nil {
		return "", "", err
	}
	if scope == ".." || strings.HasPrefix(scope, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("scope %s is outside repository %s", scopeDir, repoRoot)
	}
	return scopeDir, scope, nil
}

// verifyTagState checks the release tag against the manifest phase. v2 ships as
// one module, so there is one current tag and no historical v2 tags to retain.
func verifyTagState(repoRoot string, manifest releaseconfig.Manifest) error {
	want, err := expectedTagState(manifest.Phase)
	if err != nil {
		return err
	}
	got, err := tagExists(repoRoot, manifest.Tag)
	if err != nil {
		return err
	}
	if got != want {
		state := "absent"
		if want {
			state = "present"
		}
		return fmt.Errorf("tag %s must be %s in phase %s", manifest.Tag, state, manifest.Phase)
	}
	allV2Tags := strings.Fields(run(repoRoot, "git", "tag", "--list", "v2.*", "v2/*"))
	var historical []string
	for _, tag := range allV2Tags {
		if tag != manifest.Tag {
			historical = append(historical, tag)
		}
	}
	if len(historical) > 0 {
		return fmt.Errorf("historical v2 tags must be absent:\n%s", strings.Join(historical, "\n"))
	}
	return nil
}

// expectedTagState reports whether the release tag may exist in the given phase.
func expectedTagState(phase string) (bool, error) {
	switch phase {
	case "candidate":
		return false, nil
	case "released":
		return true, nil
	default:
		return false, fmt.Errorf("unsupported release phase %q", phase)
	}
}

func tagExists(repoRoot, tag string) (bool, error) {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/tags/"+tag)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("inspect tag %s: %w", tag, err)
	}
	return true, nil
}

func verifyPublishedModule(modulePath, version string) {
	tempDir, err := os.MkdirTemp("", "go-kit-published-check-")
	if err != nil {
		fail("create published-module check directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cmd := exec.Command("go", "list", "-m", "-json", modulePath+"@"+version)
	cmd.Dir = tempDir
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://proxy.golang.org",
		"GOWORK=off",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		fail("published module %s@%s is not resolvable: %v\n%s", modulePath, version, err, stderr.String())
	}

	var module struct {
		Path    string
		Version string
	}
	if err := json.Unmarshal(stdout.Bytes(), &module); err != nil {
		fail("decode published module metadata: %v", err)
	}
	if module.Path != modulePath || module.Version != version {
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
