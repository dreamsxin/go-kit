package tools_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
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
				{method: "GET", path: "/skill?format=openai", want: "\"function\""},
				{method: "GET", path: "/skill?format=mcp", want: "inputSchema"},
				{method: "GET", path: "/skill?format=unknown", want: "\"function\""},
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

func expectJSONStatusContains(t *testing.T, method, url, body string, wantStatus int, want string) {
	t.Helper()

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("%s %s: build request: %v", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: want status %d, got %d", method, url, wantStatus, resp.StatusCode)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("%s %s: decode json body: %v", method, url, err)
	}
	encoded, _ := json.Marshal(data)
	if !strings.Contains(string(encoded), want) {
		t.Fatalf("%s %s: body %s does not contain %q", method, url, encoded, want)
	}
}

func expectStatusContains(t *testing.T, method, url, body string, wantStatus int, want string) {
	t.Helper()

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("%s %s: build request: %v", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: want status %d, got %d", method, url, wantStatus, resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", method, url, err)
	}
	if !strings.Contains(string(respBody), want) {
		t.Fatalf("%s %s: body %s does not contain %q", method, url, respBody, want)
	}
}

func runCommand(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", strings.Join(cmd.Args, " "), err, out)
	}
	return string(out)
}

func resolveCommandPath(name string, envVar string, extraDirs ...string) string {
	if envVar != "" {
		if candidate := strings.TrimSpace(os.Getenv(envVar)); candidate != "" {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}

	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	candidates := []string{
		filepath.Join("D:\\gowork\\bin", name),
		filepath.Join(os.Getenv("USERPROFILE"), "go", "bin", name),
	}
	candidates = append(candidates, extraDirs...)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func requireProtoToolchain(t *testing.T) (string, string, string) {
	t.Helper()

	protoc := resolveCommandPath("protoc.exe", "PROTOC", filepath.Join("D:\\EasyDiffusion\\installer_files\\env\\Lib\\site-packages\\torch\\bin", "protoc.exe"))
	protocGenGo := resolveCommandPath("protoc-gen-go.exe", "PROTOC_GEN_GO")
	protocGenGoGRPC := resolveCommandPath("protoc-gen-go-grpc.exe", "PROTOC_GEN_GO_GRPC")

	var missing []string
	if protoc == "" {
		missing = append(missing, "protoc")
	}
	if protocGenGo == "" {
		missing = append(missing, "protoc-gen-go")
	}
	if protocGenGoGRPC == "" {
		missing = append(missing, "protoc-gen-go-grpc")
	}
	if len(missing) > 0 {
		t.Skipf("protobuf toolchain incomplete; missing %s", strings.Join(missing, ", "))
	}
	return protoc, protocGenGo, protocGenGoGRPC
}

func writeIDLSvcEndpointTransportProbe(t *testing.T, outDir, probeDirName, importPath string) string {
	t.Helper()

	probeDir := filepath.Join(outDir, "testdata", probeDirName)
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", probeDirName, err)
	}

	probePath := filepath.Join(probeDir, "main.go")
	probe := `package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	idl "` + importPath + `"
	userserviceendpoint "` + importPath + `/endpoint/userservice"
	userservicesvc "` + importPath + `/service/userservice"
	userservicetransport "` + importPath + `/transport/userservice"
	kitlog "github.com/dreamsxin/go-kit/log"
)

func main() {
	logger, err := kitlog.NewDevelopment()
	if err != nil {
		panic(err)
	}

	svc := userservicesvc.NewService(nil)
	endpoints := userserviceendpoint.MakeServerEndpoints(svc, logger)
	handler := userservicetransport.NewHTTPHandler(endpoints)

	reqBody := []byte(` + "`" + `{"username":"component-user","email":"component@example.com"}` + "`" + `)
	req := httptest.NewRequest(http.MethodPost, "/createuser", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		panic("unexpected status")
	}

	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "CreateUser: not implemented") {
		panic("unexpected body")
	}

	_ = idl.CreateUserRequest{}
}
`
	if err := os.WriteFile(probePath, []byte(probe), 0o644); err != nil {
		t.Fatalf("write %s: %v", probeDirName, err)
	}
	return "./testdata/" + probeDirName
}

func writeProtoSvcEndpointTransportProbe(t *testing.T, outDir, probeDirName, importPath string) string {
	t.Helper()

	probeDir := filepath.Join(outDir, "testdata", probeDirName)
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", probeDirName, err)
	}

	probePath := filepath.Join(probeDir, "main.go")
	probe := `package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	idl "` + importPath + `/pb"
	userserviceendpoint "` + importPath + `/endpoint/userservice"
	userservicesvc "` + importPath + `/service/userservice"
	userservicetransport "` + importPath + `/transport/userservice"
	kitlog "github.com/dreamsxin/go-kit/log"
)

func main() {
	logger, err := kitlog.NewDevelopment()
	if err != nil {
		panic(err)
	}

	svc := userservicesvc.NewService(nil)
	endpoints := userserviceendpoint.MakeServerEndpoints(svc, logger)
	handler := userservicetransport.NewHTTPHandler(endpoints)

	reqBody := []byte(` + "`" + `{"name":"proto-user","email":"proto@example.com"}` + "`" + `)
	req := httptest.NewRequest(http.MethodPost, "/createuser", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		panic("unexpected status")
	}

	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "CreateUser: not implemented") {
		panic("unexpected body")
	}

	_ = idl.CreateUserRequest{}
}
`
	if err := os.WriteFile(probePath, []byte(probe), 0o644); err != nil {
		t.Fatalf("write %s: %v", probeDirName, err)
	}
	return "./testdata/" + probeDirName
}

func writeProtoGRPCE2EProbe(t *testing.T, outDir, probeDirName, importPath, grpcAddr string) string {
	t.Helper()

	probeDir := filepath.Join(outDir, "testdata", probeDirName)
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", probeDirName, err)
	}

	probePath := filepath.Join(probeDir, "main.go")
	probe := `package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	idl "` + importPath + `/pb"
	genTransport "` + importPath + `/transport/userservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "` + grpcAddr + `",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		panic(fmt.Sprintf("dial grpc: %v", err))
	}
	defer conn.Close()

	createUser := genTransport.NewGRPCCreateUserClient(conn)
	_, err = createUser(context.Background(), idl.CreateUserRequest{
		Name:  "grpc-e2e",
		Email: "grpc-e2e@example.com",
	})
	if err == nil {
		panic("expected scaffold grpc error")
	}
	if !strings.Contains(err.Error(), "CreateUser") {
		panic(fmt.Sprintf("unexpected grpc error: %v", err))
	}
	fmt.Println(err.Error())
}
`
	if err := os.WriteFile(probePath, []byte(probe), 0o644); err != nil {
		t.Fatalf("write %s: %v", probeDirName, err)
	}
	return "./testdata/" + probeDirName
}

func createSQLiteSchema(t *testing.T, dbPath string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir sqlite dir: %v", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	schema := `
CREATE TABLE users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO users (username, email) VALUES ('seed-user', 'seed@example.com');
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("init sqlite schema: %v", err)
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
		expectStatusContains(t, "GET", baseURL+"/skill?format=openai", "", http.StatusOK, "\"function\"")
		expectStatusContains(t, "GET", baseURL+"/debug/routes", "", http.StatusOK, "/createuser")
		expectStatusContains(t, "GET", baseURL+"/skill?format=mcp", "", http.StatusOK, "\"inputSchema\"")
		expectStatusContains(t, "GET", baseURL+"/skill?format=unknown", "", http.StatusOK, "\"function\"")
		expectJSONStatusContains(t, "POST", baseURL+"/createuser", `{"username":"alice","email":"alice@example.com"}`, http.StatusInternalServerError, "CreateUser: not implemented")
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

		minimalProbePkg := writeIDLSvcEndpointTransportProbe(t, outDir, "minimalcomponentprobe", "example.com/gen_idl_minimal_runtime")
		minimalProbeCmd := exec.Command("go", "run", "-mod=mod", minimalProbePkg)
		minimalProbeCmd.Dir = outDir
		runCommand(t, minimalProbeCmd)

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
		expectStatusContains(t, "GET", baseURL+"/debug/routes", "", http.StatusOK, "/createuser")

		resp, err := http.Get(baseURL + "/skill")
		if err != nil {
			t.Fatalf("GET /skill: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("expected /skill to be disabled, got status %d", resp.StatusCode)
		}

		mcpResp, err := http.Get(baseURL + "/skill?format=mcp")
		if err != nil {
			t.Fatalf("GET /skill?format=mcp: %v", err)
		}
		defer mcpResp.Body.Close()
		if mcpResp.StatusCode == http.StatusOK {
			t.Fatalf("expected MCP /skill to be disabled, got status %d", mcpResp.StatusCode)
		}
	})

	t.Run("IDL_PrefixedProject_BuildsAndServesPrefixedBusinessRoute", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_prefixed_runtime")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_prefixed_runtime",
			"-config=false",
			"-docs=false",
			"-model=false",
			"-db=false",
			"-skill=false",
			"-prefix", "/api/runtime",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen idl prefixed runtime failed: %v\n%s", err, out)
		}

		mustContainFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"), "/api/runtime/userservice")
		mustContainFile(t, filepath.Join(outDir, "cmd", "main.go"), "/api/runtime/userservice")

		binName := "microgen_prefixed_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}

		buildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		buildCmd.Dir = outDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("generated prefixed project build failed: %v\n%s", err, out)
		}
		binPath := filepath.Join(outDir, binName)
		defer os.Remove(binPath)

		httpAddr := freeTCPAddr(t)
		baseURL := "http://" + httpAddr
		runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr)
		runCmd.Dir = outDir
		runCmd.Env = os.Environ()
		if err := runCmd.Start(); err != nil {
			t.Fatalf("failed to start generated prefixed project: %v", err)
		}
		defer killCmd(t, runCmd)

		waitServer(t, baseURL+"/health")
		smokeTest{method: "GET", path: "/health", want: "ok"}.run(t, baseURL)
		expectStatusContains(t, "GET", baseURL+"/debug/routes", "", http.StatusOK, "/api/runtime/userservice/createuser")
		expectJSONStatusContains(t, "POST", baseURL+"/api/runtime/userservice/createuser", `{"username":"alice","email":"alice@example.com"}`, http.StatusInternalServerError, "CreateUser: not implemented")

		resp, err := http.Post(baseURL+"/createuser", "application/json", strings.NewReader(`{"username":"alice","email":"alice@example.com"}`))
		if err != nil {
			t.Fatalf("POST /createuser: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected unprefixed route to be disabled, got status %d", resp.StatusCode)
		}
	})

	t.Run("IDL_FullGeneratedComponents_AreUsable", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_components")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_components",
			"-docs=false",
			"-model=false",
			"-db=false",
			"-swag",
			"-skill",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen idl components failed: %v\n%s", err, out)
		}

		mustExistFile(t, filepath.Join(outDir, "cmd", "main.go"))
		mustExistFile(t, filepath.Join(outDir, "service", "userservice", "service.go"))
		mustExistFile(t, filepath.Join(outDir, "endpoint", "userservice", "endpoints.go"))
		mustExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"))
		mustExistFile(t, filepath.Join(outDir, "client", "userservice", "demo.go"))
		mustExistFile(t, filepath.Join(outDir, "sdk", "userservicesdk", "client.go"))
		mustExistFile(t, filepath.Join(outDir, "skill", "skill.go"))

		buildTargets := []string{
			"./cmd",
			"./service/...",
			"./endpoint/...",
			"./transport/...",
			"./client/...",
			"./sdk/...",
			"./skill/...",
		}
		buildCmd := exec.Command("go", append([]string{"build", "-mod=mod"}, buildTargets...)...)
		buildCmd.Dir = outDir
		runCommand(t, buildCmd)

		componentProbePkg := writeIDLSvcEndpointTransportProbe(t, outDir, "componentprobe", "example.com/gen_idl_components")
		componentCmd := exec.Command("go", "run", "-mod=mod", componentProbePkg)
		componentCmd.Dir = outDir
		runCommand(t, componentCmd)

		binName := "microgen_components_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		serverBuildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		serverBuildCmd.Dir = outDir
		runCommand(t, serverBuildCmd)
		binPath := filepath.Join(outDir, binName)
		defer os.Remove(binPath)

		httpAddr := freeTCPAddr(t)
		baseURL := "http://" + httpAddr
		runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr)
		runCmd.Dir = outDir
		runCmd.Env = os.Environ()
		if err := runCmd.Start(); err != nil {
			t.Fatalf("failed to start generated components project: %v", err)
		}
		defer killCmd(t, runCmd)

		waitServer(t, baseURL+"/health")
		smokeTest{method: "GET", path: "/health", want: "ok"}.run(t, baseURL)
		smokeTest{method: "GET", path: "/skill", want: "CreateUser"}.run(t, baseURL)
		expectStatusContains(t, "GET", baseURL+"/skill?format=openai", "", http.StatusOK, "\"function\"")
		expectStatusContains(t, "GET", baseURL+"/skill?format=mcp", "", http.StatusOK, "\"inputSchema\"")
		expectStatusContains(t, "GET", baseURL+"/skill?format=unknown", "", http.StatusOK, "\"function\"")

		demoCmd := exec.Command("go", "run", "./client/userservice/demo.go", "-mode=http", "-http.addr="+baseURL)
		demoCmd.Dir = outDir
		demoOut := runCommand(t, demoCmd)
		if !strings.Contains(demoOut, "Demo completed") {
			t.Fatalf("demo output did not show completion:\n%s", demoOut)
		}
		if !strings.Contains(demoOut, "CreateUser") {
			t.Fatalf("demo output did not exercise generated methods:\n%s", demoOut)
		}

		sdkProbeDir := filepath.Join(outDir, "testdata", "sdkprobe")
		if err := os.MkdirAll(sdkProbeDir, 0o755); err != nil {
			t.Fatalf("mkdir sdkprobe: %v", err)
		}
		sdkProbePath := filepath.Join(sdkProbeDir, "main.go")
		sdkProbe := `package main

import (
	"context"
	"fmt"

	idl "example.com/gen_idl_components"
	userservicesdk "example.com/gen_idl_components/sdk/userservicesdk"
)

func main() {
	client := userservicesdk.New("` + baseURL + `")
	_, err := client.CreateUser(context.Background(), idl.CreateUserRequest{
		Username: "sdk-user",
		Email:    "sdk@example.com",
	})
	if err == nil {
		panic("expected generated sdk call to surface scaffold error")
	}
	fmt.Println(err.Error())
}
`
		if err := os.WriteFile(sdkProbePath, []byte(sdkProbe), 0o644); err != nil {
			t.Fatalf("write sdk probe: %v", err)
		}

		sdkCmd := exec.Command("go", "run", "-mod=mod", "./testdata/sdkprobe")
		sdkCmd.Dir = outDir
		sdkOut := runCommand(t, sdkCmd)
		if !strings.Contains(sdkOut, "server returned 500") {
			t.Fatalf("sdk probe output did not surface http api error:\n%s", sdkOut)
		}
		if !strings.Contains(sdkOut, "CreateUser: not implemented") {
			t.Fatalf("sdk probe output did not contain scaffold error:\n%s", sdkOut)
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
		mustContainFile(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"), "string id = 1;")
		mustContainFile(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"), "string name = 2;")
		mustContainFile(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"), "string email = 3;")
		mustContainFile(t, filepath.Join(outDir, "README.md"), "protoc --go_out=. --go-grpc_out=.")
		mustContainFile(t, filepath.Join(outDir, "README.md"), "pb/userservice/userservice.proto")
		mustContainFile(t, filepath.Join(outDir, "README.md"), "Review the generated proto contract before generating stubs")
		mustContainFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"), "/api/proto/userservice")
		mustContainFile(t, filepath.Join(outDir, "cmd", "main.go"), "/api/proto/userservice")

		// Build check for the generated code
		buildCmd := exec.Command("go", "build", "./...")
		buildCmd.Dir = outDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Logf("Note: Generated proto code requires protoc, so full build might fail without it. Build output: %s", out)
		}
	})

	t.Run("Proto_ComponentFlow_WhenProtocAvailable", func(t *testing.T) {
		protoc, protocGenGo, protocGenGoGRPC := requireProtoToolchain(t)

		outDir := filepath.Join(cwd, "testdata", "gen_proto_component_flow")
		os.RemoveAll(outDir)

		protoFile := filepath.Join(cwd, "testdata", "service.proto")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", protoFile,
			"-out", outDir,
			"-import", "example.com/gen_proto_component_flow",
			"-protocols", "http,grpc",
			"-model=false",
			"-db=false",
			"-skill",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen proto component flow failed: %v\n%s", err, out)
		}

		protocCmd := exec.Command(protoc,
			"--go_out=.",
			"--go-grpc_out=.",
			"pb/userservice/userservice.proto",
		)
		protocCmd.Dir = outDir
		protocCmd.Env = append(os.Environ(),
			"PATH="+strings.Join([]string{
				filepath.Dir(protoc),
				filepath.Dir(protocGenGo),
				filepath.Dir(protocGenGoGRPC),
				os.Getenv("PATH"),
			}, string(os.PathListSeparator)),
		)
		runCommand(t, protocCmd)

		buildTargets := []string{
			"./service/...",
			"./endpoint/...",
			"./transport/...",
			"./client/...",
			"./sdk/...",
			"./skill/...",
		}
		buildCmd := exec.Command("go", append([]string{"build", "-mod=mod"}, buildTargets...)...)
		buildCmd.Dir = outDir
		runCommand(t, buildCmd)

		componentProbePkg := writeProtoSvcEndpointTransportProbe(t, outDir, "protocomponentprobe", "example.com/gen_proto_component_flow")
		componentCmd := exec.Command("go", "run", "-mod=mod", componentProbePkg)
		componentCmd.Dir = outDir
		runCommand(t, componentCmd)
	})

	t.Run("Proto_GRPC_GeneratedProject_BuildsAndServesRequests", func(t *testing.T) {
		protoc, protocGenGo, protocGenGoGRPC := requireProtoToolchain(t)

		outDir := filepath.Join(cwd, "testdata", "gen_proto_grpc_runtime")
		os.RemoveAll(outDir)

		protoFile := filepath.Join(cwd, "testdata", "service.proto")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", protoFile,
			"-out", outDir,
			"-import", "example.com/gen_proto_grpc_runtime",
			"-protocols", "http,grpc",
			"-config=false",
			"-docs=false",
			"-model=false",
			"-db=false",
			"-skill=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen proto grpc runtime failed: %v\n%s", err, out)
		}

		protocCmd := exec.Command(protoc,
			"--go_out=.",
			"--go-grpc_out=.",
			"pb/userservice/userservice.proto",
		)
		protocCmd.Dir = outDir
		protocCmd.Env = append(os.Environ(),
			"PATH="+strings.Join([]string{
				filepath.Dir(protoc),
				filepath.Dir(protocGenGo),
				filepath.Dir(protocGenGoGRPC),
				os.Getenv("PATH"),
			}, string(os.PathListSeparator)),
		)
		runCommand(t, protocCmd)

		buildTargets := []string{
			"./cmd",
			"./service/...",
			"./endpoint/...",
			"./transport/...",
			"./client/...",
			"./sdk/...",
		}
		buildCmd := exec.Command("go", append([]string{"build", "-mod=mod"}, buildTargets...)...)
		buildCmd.Dir = outDir
		runCommand(t, buildCmd)

		binName := "microgen_proto_grpc_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		serverBuildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		serverBuildCmd.Dir = outDir
		runCommand(t, serverBuildCmd)
		defer os.Remove(filepath.Join(outDir, binName))

		httpAddr := freeTCPAddr(t)
		grpcAddr := freeTCPAddr(t)
		baseURL := "http://" + httpAddr
		runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr, "-grpc.addr="+grpcAddr)
		runCmd.Dir = outDir
		runCmd.Env = os.Environ()
		if err := runCmd.Start(); err != nil {
			t.Fatalf("failed to start generated proto grpc project: %v", err)
		}
		defer killCmd(t, runCmd)

		waitServer(t, baseURL+"/health")
		smokeTest{method: "GET", path: "/health", want: "ok"}.run(t, baseURL)
		expectJSONStatusContains(t, "POST", baseURL+"/createuser", `{"name":"http-user","email":"http@example.com"}`, http.StatusInternalServerError, "CreateUser: not implemented")

		grpcProbePkg := writeProtoGRPCE2EProbe(t, outDir, "protogrpce2eprobe", "example.com/gen_proto_grpc_runtime", grpcAddr)
		grpcProbeCmd := exec.Command("go", "run", "-mod=mod", grpcProbePkg)
		grpcProbeCmd.Dir = outDir
		grpcOut := runCommand(t, grpcProbeCmd)
		if !strings.Contains(grpcOut, "CreateUser") {
			t.Fatalf("grpc probe output did not contain CreateUser scaffold error:\n%s", grpcOut)
		}

		demoCmd := exec.Command("go", "run", "./client/userservice/demo.go", "-mode=grpc", "-grpc.addr="+grpcAddr)
		demoCmd.Dir = outDir
		demoOut := runCommand(t, demoCmd)
		if !strings.Contains(demoOut, "Demo completed") {
			t.Fatalf("grpc demo output did not show completion:\n%s", demoOut)
		}
		if !strings.Contains(demoOut, "CreateUser") {
			t.Fatalf("grpc demo output did not exercise generated grpc methods:\n%s", demoOut)
		}
	})

	t.Run("FromDB_SQLite_GeneratedProject_BuildsAndRuns", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_fromdb_sqlite")
		os.RemoveAll(outDir)

		dbPath := filepath.Join(cwd, "testdata", "fromdb_runtime.sqlite")
		_ = os.Remove(dbPath)
		defer os.Remove(dbPath)
		createSQLiteSchema(t, dbPath)

		cmd := exec.Command("go", "run", microgenPath,
			"-from-db",
			"-driver", "sqlite",
			"-dsn", dbPath,
			"-service", "CatalogService",
			"-out", outDir,
			"-import", "example.com/gen_fromdb_sqlite",
			"-config=false",
			"-docs=false",
			"-db=false",
			"-skill=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen from-db sqlite failed: %v\n%s", err, out)
		}

		mustExistFile(t, filepath.Join(outDir, "go.mod"))
		mustExistFile(t, filepath.Join(outDir, "idl.go"))
		mustExistFile(t, filepath.Join(outDir, "cmd", "main.go"))
		mustExistFile(t, filepath.Join(outDir, "service", "catalogservice", "service.go"))
		mustExistFile(t, filepath.Join(outDir, "endpoint", "catalogservice", "endpoints.go"))
		mustExistFile(t, filepath.Join(outDir, "transport", "catalogservice", "transport_http.go"))
		mustExistFile(t, filepath.Join(outDir, "model", "model.go"))
		mustContainFile(t, filepath.Join(outDir, "idl.go"), "type CreateUserRequest struct")
		mustContainFile(t, filepath.Join(outDir, "idl.go"), "type ListUsersRequest struct")
		mustContainFile(t, filepath.Join(outDir, "transport", "catalogservice", "transport_http.go"), "/user")
		mustContainFile(t, filepath.Join(outDir, "transport", "catalogservice", "transport_http.go"), "/users")

		buildTargets := []string{
			"./cmd",
			"./service/...",
			"./endpoint/...",
			"./transport/...",
			"./model/...",
		}
		buildCmd := exec.Command("go", append([]string{"build", "-mod=mod"}, buildTargets...)...)
		buildCmd.Dir = outDir
		runCommand(t, buildCmd)

		binName := "microgen_fromdb_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		serverBuildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		serverBuildCmd.Dir = outDir
		runCommand(t, serverBuildCmd)
		defer os.Remove(filepath.Join(outDir, binName))

		httpAddr := freeTCPAddr(t)
		baseURL := "http://" + httpAddr
		runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr)
		runCmd.Dir = outDir
		runCmd.Env = os.Environ()
		if err := runCmd.Start(); err != nil {
			t.Fatalf("failed to start generated from-db project: %v", err)
		}
		defer killCmd(t, runCmd)

		waitServer(t, baseURL+"/health")
		smokeTest{method: "GET", path: "/health", want: "ok"}.run(t, baseURL)
		expectStatusContains(t, "GET", baseURL+"/debug/routes", "", http.StatusOK, "/user")
		expectStatusContains(t, "GET", baseURL+"/debug/routes", "", http.StatusOK, "/users")
		expectJSONStatusContains(t, "POST", baseURL+"/user", `{"username":"db-user","email":"db@example.com"}`, http.StatusInternalServerError, "CreateUser: not implemented")
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
