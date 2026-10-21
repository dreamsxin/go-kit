package tools_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// microgenCommand runs the repository generator from the root module. This is
// important when tools tests run with GOWORK=off: the generator imports
// cmd/microgen/internal packages, which Go permits only when the command is
// built from within the owning module.
//
// MICROGEN_GO_KIT_ROOT goes on the child's environment rather than the test
// process's. Only the generator reads it, and setting it with t.Setenv would
// mutate process-global state, which bars every caller from t.Parallel.
func microgenCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	root := goKitRoot(t)
	cmd := exec.Command(microgenBinary(t), args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "MICROGEN_GO_KIT_ROOT="+filepath.ToSlash(root))
	return cmd
}

// microgenBuild is the generator compiled once for the whole test binary.
type microgenBuildState struct {
	once   sync.Once
	path   string
	dir    string
	reason error
}

var microgenBuild microgenBuildState

// microgenBinary builds the generator once and returns the executable.
//
// Every call used to be `go run ./cmd/microgen`, which links a fresh executable
// each time — about two seconds with a warm build cache, and this suite invokes
// the generator forty times. The generator keeps nothing between runs: the tree
// it reads in is MICROGEN_GO_KIT_ROOT and its working directory, both of which
// still differ per caller. One build therefore serves all of them, and the
// callers that need to invoke it twice from one case stop paying twice as well.
func microgenBinary(t *testing.T) string {
	t.Helper()
	microgenBuild.once.Do(func() {
		cwd, err := os.Getwd()
		if err != nil {
			microgenBuild.reason = fmt.Errorf("Getwd: %w", err)
			return
		}
		root := filepath.Dir(cwd)
		dir, err := os.MkdirTemp("", "go-kit-microgen-")
		if err != nil {
			microgenBuild.reason = fmt.Errorf("create generator build directory: %w", err)
			return
		}
		microgenBuild.dir = dir
		binary := filepath.Join(dir, "microgen")
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		build := exec.Command("go", "build", "-o", binary, "./cmd/microgen")
		build.Dir = root
		build.Env = os.Environ()
		if out, err := build.CombinedOutput(); err != nil {
			microgenBuild.reason = fmt.Errorf("build the generator: %w\n%s", err, out)
			return
		}
		microgenBuild.path = binary
	})
	if microgenBuild.reason != nil {
		t.Fatalf("%v", microgenBuild.reason)
	}
	return microgenBuild.path
}

// TestMain removes the shared generator build directory. It cannot be a
// t.TempDir: the first caller's cleanup would delete the binary the rest of the
// suite is still using.
func TestMain(m *testing.M) {
	code := m.Run()
	if microgenBuild.dir != "" {
		_ = os.RemoveAll(microgenBuild.dir)
	}
	os.Exit(code)
}

// goKitRoot is the v2 module root, the parent of the tools module this test
// binary runs in.
func goKitRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return filepath.Dir(cwd)
}

func generatedProjectDir(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

func TestMicrogenCLIValidation(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)

	t.Run("FailsWithoutIDLOrFromDB", func(t *testing.T) {
		cmd := microgenCommand(t)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("expected microgen to fail without -idl or -from-db")
		}
		if !strings.Contains(string(out), "either -idl or -from-db is required") {
			t.Fatalf("unexpected error output:\n%s", out)
		}
	})

	t.Run("FailsForMissingIDLPath", func(t *testing.T) {
		outDir := generatedProjectDir(t, "gen_missing_idl")

		cmd := microgenCommand(t,
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
		outDir := generatedProjectDir(t, "gen_bad_driver")

		idlFile := filepath.Join(root, "cmd", "microgen", "internal", "parser", "testdata", "basic.go")
		cmd := microgenCommand(t,
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
