package tools_test

import (
	"encoding/json"
	"fmt"
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

// TestMicrogenIntegration tests the full microgen workflow.
func TestMicrogenIntegration(t *testing.T) {
	cwd, _ := os.Getwd()
	root := filepath.Dir(cwd)
	microgenPath := filepath.Join(root, "cmd", "microgen", "main.go")

	t.Run("Proto", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_proto_integration")
		os.RemoveAll(outDir)

		protoFile := filepath.Join(cwd, "testdata", "service.proto")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", protoFile,
			"-out", outDir,
			"-import", "example.com/gen_proto_integration",
			"-skill",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen proto failed: %v\n%s", err, out)
		}

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
			"-skill",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen idl failed: %v\n%s", err, out)
		}

		// Verify key files were generated
		mustExistFile(t, filepath.Join(outDir, "service", "userservice", "service.go"))
		mustExistFile(t, filepath.Join(outDir, "endpoint", "userservice", "endpoints.go"))
		mustExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"))
		mustExistFile(t, filepath.Join(outDir, "skill", "skill.go"))
		mustExistFile(t, filepath.Join(outDir, "cmd", "main.go"))

		// Verify generated service package compiles (it only depends on go-kit itself)
		buildCmd := exec.Command("go", "build", "-mod=mod", "./service/...")
		buildCmd.Dir = outDir
		buildCmd.Env = append(os.Environ(), "GONOSUMDB=*")
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Errorf("generated service/ failed to compile: %v\n%s", err, out)
		}
	})
}

func mustExistFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", path)
	}
}
