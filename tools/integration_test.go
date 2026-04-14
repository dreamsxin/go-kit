package tools_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestAllExamples builds and runs all examples to ensure they still work.
func TestAllExamples(t *testing.T) {
	cwd, _ := os.Getwd()
	root := filepath.Dir(cwd)
	examplesDir := filepath.Join(root, "examples")

	examples := []struct {
		name       string
		path       string
		port       int
		run        bool // whether to run and smoke test
		smokeTests []smokeTest
	}{
		{
			name: "quickstart",
			path: "quickstart",
			port: 8082,
			run:  true,
			smokeTests: []smokeTest{
				{method: "POST", path: "/hello", body: `{"name":"world"}`, want: "Hello, world!"},
			},
		},
		{
			name: "best_practice",
			path: "best_practice",
			port: 8083,
			run:  true,
			smokeTests: []smokeTest{
				{method: "POST", path: "/hello", body: `{"name":"Alice"}`, want: "Hello, Alice!"},
			},
		},
		{
			name: "microgen_skill",
			path: "microgen_skill",
			port: 8084,
			run:  true,
			smokeTests: []smokeTest{
				{method: "POST", path: "/sayhello", body: `{"name":"Bob", "tags":["test"]}`, want: "Hello, Bob!"},
				{method: "GET", path: "/skill", want: "SayHello"},
				{method: "GET", path: "/skill?format=mcp", want: "inputSchema"},
			},
		},
	}

	for _, tc := range examples {
		t.Run(tc.name, func(t *testing.T) {
			pkgPath := filepath.Join(examplesDir, tc.path)

			// 1. Build check
			t.Logf("Building %s...", tc.name)
			binName := "test_bin_" + tc.name
			if runtime.GOOS == "windows" {
				binName += ".exe"
			}
			
			buildPath := "."
			if tc.name == "microgen_skill" {
				buildPath = "./cmd"
			}
			
			// Ensure dependencies are tidy
			tidyCmd := exec.Command("go", "mod", "tidy")
			tidyCmd.Dir = pkgPath
			tidyCmd.Run()

			cmd := exec.Command("go", "build", "-o", binName, buildPath)
			cmd.Dir = pkgPath
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build failed: %v\n%s", err, out)
			}
			defer os.Remove(filepath.Join(pkgPath, binName))

			if !tc.run {
				return
			}

			// 2. Run and smoke test
			t.Logf("Running %s on port %d...", tc.name, tc.port)
			addr := fmt.Sprintf(":%d", tc.port)
			baseURL := fmt.Sprintf("http://localhost:%d", tc.port)
			
			runCmd := exec.Command("./" + binName)
			if tc.name == "microgen_skill" {
				runCmd.Args = append(runCmd.Args, "-http.addr="+addr, "-grpc.addr=:8091")
			} else {
				runCmd.Args = append(runCmd.Args, "-http.addr="+addr)
			}
			runCmd.Dir = pkgPath
			runCmd.Env = os.Environ()

			if err := runCmd.Start(); err != nil {
				t.Fatalf("failed to start %s: %v", tc.name, err)
			}
			
			killProcess := func() {
				if runCmd.Process != nil {
					if runtime.GOOS == "windows" {
						exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", runCmd.Process.Pid)).Run()
					} else {
						runCmd.Process.Kill()
					}
				}
			}
			defer killProcess()

			// Wait for server to start
			waitServer(t, baseURL+"/health")

			for _, st := range tc.smokeTests {
				st.run(t, baseURL)
			}
		})
	}
}

type smokeTest struct {
	method string
	path   string
	body   string
	want   string
}

func (st smokeTest) run(t *testing.T, baseUrl string) {
	url := baseUrl + st.path
	req, _ := http.NewRequest(st.method, url, strings.NewReader(st.body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("%s %s failed: %v", st.method, st.path, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("%s %s: want 200, got %d", st.method, st.path, resp.StatusCode)
	}

	var data map[string]any
	json.NewDecoder(resp.Body).Decode(&data)
	body, _ := json.Marshal(data)
	if !strings.Contains(string(body), st.want) {
		t.Errorf("%s %s: body %s does not contain %q", st.method, st.path, body, st.want)
	}
}

func waitServer(t *testing.T, url string) {
	for i := 0; i < 20; i++ {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server at %s failed to start in time", url)
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()
	return l.Addr().String()
}

func killCmd(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid)).Run()
		return
	}
	_ = cmd.Process.Kill()
}

// TestMicrogenIntegration tests the full microgen workflow.
func TestMicrogenIntegration(t *testing.T) {
	cwd, _ := os.Getwd()
	root := filepath.Dir(cwd)
	microgenPath := filepath.Join(root, "cmd", "microgen", "main.go")

	t.Run("CLI_FailsWithoutIDLOrFromDB", func(t *testing.T) {
		cmd := exec.Command("go", "run", microgenPath)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("expected microgen to fail without -idl or -from-db")
		}
		if !strings.Contains(string(out), "either -idl or -from-db is required") {
			t.Fatalf("unexpected error output:\n%s", out)
		}
	})

	t.Run("CLI_FailsForMissingIDLPath", func(t *testing.T) {
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

	t.Run("CLI_FailsForUnsupportedDriver", func(t *testing.T) {
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

	t.Run("IDL_DefaultFlags", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_default_flags")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
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
		mustExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"))
		mustExistFile(t, filepath.Join(outDir, "client", "userservice", "demo.go"))
		mustExistFile(t, filepath.Join(outDir, "sdk", "userservicesdk", "client.go"))
		mustExistFile(t, filepath.Join(outDir, "config", "config.yaml"))
		mustExistFile(t, filepath.Join(outDir, "config", "config.go"))
		mustExistFile(t, filepath.Join(outDir, "README.md"))
		mustExistFile(t, filepath.Join(outDir, "model", "model.go"))
		mustExistFile(t, filepath.Join(outDir, "repository", "repository.go"))
		mustExistFile(t, filepath.Join(outDir, "skill", "skill.go"))
		mustNotExistFile(t, filepath.Join(outDir, "docs", "docs.go"))
		mustNotExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_grpc.go"))
		mustNotExistFile(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"))
	})

	t.Run("IDL_GeneratedProject_BuildsAndRuns", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_runnable")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_runnable",
			"-config=false",
			"-docs=false",
			"-model=false",
			"-db=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen idl runnable failed: %v\n%s", err, out)
		}

		binName := "microgen_runnable_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}

		buildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		buildCmd.Dir = outDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("generated project build failed: %v\n%s", err, out)
		}
		binPath := filepath.Join(outDir, binName)
		defer os.Remove(binPath)

		httpAddr := freeTCPAddr(t)
		baseURL := "http://" + httpAddr
		runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr)
		runCmd.Dir = outDir
		runCmd.Env = os.Environ()
		if err := runCmd.Start(); err != nil {
			t.Fatalf("failed to start generated project: %v", err)
		}
		defer killCmd(t, runCmd)

		waitServer(t, baseURL+"/health")
		smokeTest{method: "GET", path: "/health", want: "ok"}.run(t, baseURL)
		smokeTest{method: "GET", path: "/skill", want: "CreateUser"}.run(t, baseURL)
	})

	t.Run("IDL_MinimalProject_BuildsAndRunsWithoutOptionalFeatures", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_minimal_runtime")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_minimal_runtime",
			"-config=false",
			"-docs=false",
			"-model=false",
			"-db=false",
			"-skill=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen idl minimal failed: %v\n%s", err, out)
		}

		mustExistFile(t, filepath.Join(outDir, "go.mod"))
		mustExistFile(t, filepath.Join(outDir, "cmd", "main.go"))
		mustExistFile(t, filepath.Join(outDir, "service", "userservice", "service.go"))
		mustExistFile(t, filepath.Join(outDir, "endpoint", "userservice", "endpoints.go"))
		mustExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"))
		mustNotExistFile(t, filepath.Join(outDir, "config", "config.yaml"))
		mustNotExistFile(t, filepath.Join(outDir, "README.md"))
		mustNotExistFile(t, filepath.Join(outDir, "model", "model.go"))
		mustNotExistFile(t, filepath.Join(outDir, "repository", "repository.go"))
		mustNotExistFile(t, filepath.Join(outDir, "skill", "skill.go"))
		mustNotExistFile(t, filepath.Join(outDir, "docs", "docs.go"))

		binName := "microgen_minimal_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}

		buildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		buildCmd.Dir = outDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("generated minimal project build failed: %v\n%s", err, out)
		}
		binPath := filepath.Join(outDir, binName)
		defer os.Remove(binPath)

		httpAddr := freeTCPAddr(t)
		baseURL := "http://" + httpAddr
		runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr)
		runCmd.Dir = outDir
		runCmd.Env = os.Environ()
		if err := runCmd.Start(); err != nil {
			t.Fatalf("failed to start generated minimal project: %v", err)
		}
		defer killCmd(t, runCmd)

		waitServer(t, baseURL+"/health")
		smokeTest{method: "GET", path: "/health", want: "ok"}.run(t, baseURL)

		resp, err := http.Get(baseURL + "/skill")
		if err != nil {
			t.Fatalf("GET /skill: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("expected /skill to be disabled, got status %d", resp.StatusCode)
		}
	})

	t.Run("Proto", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_proto_integration")
		os.RemoveAll(outDir)

		protoFile := filepath.Join(cwd, "testdata", "service.proto")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", protoFile,
			"-out", outDir,
			"-import", "example.com/gen_proto_integration",
			"-protocols", "http,grpc",
			"-prefix", "/api/proto",
			"-swag",
			"-skill",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen proto failed: %v\n%s", err, out)
		}

		mustExistFile(t, filepath.Join(outDir, "go.mod"))
		mustExistFile(t, filepath.Join(outDir, "service", "userservice", "service.go"))
		mustExistFile(t, filepath.Join(outDir, "endpoint", "userservice", "endpoints.go"))
		mustExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"))
		mustExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_grpc.go"))
		mustExistFile(t, filepath.Join(outDir, "client", "userservice", "demo.go"))
		mustExistFile(t, filepath.Join(outDir, "sdk", "userservicesdk", "client.go"))
		mustExistFile(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"))
		mustExistFile(t, filepath.Join(outDir, "docs", "docs.go"))
		mustExistFile(t, filepath.Join(outDir, "skill", "skill.go"))
		mustExistFile(t, filepath.Join(outDir, "cmd", "main.go"))
		mustNotExistFile(t, filepath.Join(outDir, "idl.go"))
		mustContainFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"), "/api/proto/userservice")
		mustContainFile(t, filepath.Join(outDir, "cmd", "main.go"), "/api/proto/userservice")

		// Build check for the generated code
		buildCmd := exec.Command("go", "build", "./...")
		buildCmd.Dir = outDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Logf("Note: Generated proto code requires protoc, so full build might fail without it. Build output: %s", out)
		}
	})

	t.Run("IDL", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_integration")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_integration",
			"-prefix", "/api/idl",
			"-swag",
			"-skill",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen idl failed: %v\n%s", err, out)
		}

		// Verify key files were generated
		mustExistFile(t, filepath.Join(outDir, "go.mod"))
		mustExistFile(t, filepath.Join(outDir, "idl.go"))
		mustExistFile(t, filepath.Join(outDir, "service", "userservice", "service.go"))
		mustExistFile(t, filepath.Join(outDir, "endpoint", "userservice", "endpoints.go"))
		mustExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"))
		mustExistFile(t, filepath.Join(outDir, "client", "userservice", "demo.go"))
		mustExistFile(t, filepath.Join(outDir, "sdk", "userservicesdk", "client.go"))
		mustExistFile(t, filepath.Join(outDir, "docs", "docs.go"))
		mustExistFile(t, filepath.Join(outDir, "skill", "skill.go"))
		mustExistFile(t, filepath.Join(outDir, "cmd", "main.go"))
		mustContainFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"), "/api/idl/userservice")
		mustContainFile(t, filepath.Join(outDir, "cmd", "main.go"), "/api/idl/userservice")

		// Verify generated service package compiles (it only depends on go-kit itself)
		buildCmd := exec.Command("go", "build", "-mod=mod", "./service/...")
		buildCmd.Dir = outDir
		buildCmd.Env = append(os.Environ(), "GONOSUMDB=*")
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Errorf("generated service/ failed to compile: %v\n%s", err, out)
		}
	})

	t.Run("IDL_Rerun_PreservesCustomizedGoModAndDocs", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_rerun")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		run := func() {
			cmd := exec.Command("go", "run", microgenPath,
				"-idl", idlFile,
				"-out", outDir,
				"-import", "example.com/gen_idl_rerun",
				"-swag",
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("microgen idl rerun failed: %v\n%s", err, out)
			}
		}

		run()

		goModPath := filepath.Join(outDir, "go.mod")
		docsPath := filepath.Join(outDir, "docs", "docs.go")

		goMod := readFile(t, goModPath)
		goMod += "\nrequire example.com/custom v0.0.0\n"
		if err := os.WriteFile(goModPath, []byte(goMod), 0o644); err != nil {
			t.Fatalf("write go.mod: %v", err)
		}

		realDocs := `package docs

// Real Docs should survive reruns.
var SwaggerInfo = struct{}{}
`
		if err := os.WriteFile(docsPath, []byte(realDocs), 0o644); err != nil {
			t.Fatalf("write docs.go: %v", err)
		}

		run()

		mustContainFile(t, goModPath, "require example.com/custom v0.0.0")
		mustContainFile(t, docsPath, "Real Docs should survive reruns.")
	})
}

func mustExistFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", path)
	}
}

func mustNotExistFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("expected file not to exist: %s", path)
	}
}

func mustContainFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Errorf("expected file %s to contain %q", path, want)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(data)
}
