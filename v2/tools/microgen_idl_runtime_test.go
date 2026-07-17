package tools_test

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMicrogenIDLRuntimeIntegration(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	microgenPath := microgenMainPath(t)

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
		expectStatusContains(t, "GET", baseURL+"/skill", "", http.StatusOK, "microgen.skill.v1")
		expectStatusContains(t, "GET", baseURL+"/skill?format=openai", "", http.StatusOK, "\"function\"")
		expectStatusContains(t, "GET", baseURL+"/debug/routes", "", http.StatusOK, "/createuser")
		expectStatusContains(t, "GET", baseURL+"/skill?format=mcp", "", http.StatusOK, "\"inputSchema\"")
		expectStatusContains(t, "GET", baseURL+"/skill?format=mcp", "", http.StatusOK, "microgen-ir")
		expectStatusContains(t, "GET", baseURL+"/skill?format=unknown", "", http.StatusOK, "\"function\"")
		expectJSONStatusContains(t, "POST", baseURL+"/createuser", `{"username":"alice","email":"alice@example.com"}`, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
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
		mustNotExistFile(t, filepath.Join(outDir, "model", "generated_user.go"))
		mustNotExistFile(t, filepath.Join(outDir, "repository", "generated_user_repository.go"))
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
		mustContainFile(t, filepath.Join(outDir, "cmd", "generated_routes.go"), "/api/runtime/userservice")

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
		expectJSONStatusContains(t, "POST", baseURL+"/api/runtime/userservice/createuser", `{"username":"alice","email":"alice@example.com"}`, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))

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
		if !strings.Contains(sdkOut, http.StatusText(http.StatusInternalServerError)) {
			t.Fatalf("sdk probe output did not contain redacted error message:\n%s", sdkOut)
		}
	})

}
