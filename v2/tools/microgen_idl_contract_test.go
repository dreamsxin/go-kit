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

func TestMicrogenIDLContractIntegration(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	microgenPath := microgenMainPath(t)

	t.Run("IDL", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_integration")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_integration",
			"-prefix", "/api/idl",
			"-openapi",
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
		mustExistFile(t, filepath.Join(outDir, "docs", "openapi.json"))
		mustExistFile(t, filepath.Join(outDir, "docs", "schema.json"))
		mustExistFile(t, filepath.Join(outDir, "sdk", "typescript", "client.ts"))
		mustExistFile(t, filepath.Join(outDir, "sdk", "typescript", "README.md"))
		mustExistFile(t, filepath.Join(outDir, "sdk", "typescript", "tsconfig.json"))
		mustExistFile(t, filepath.Join(outDir, "skill", "skill.go"))
		mustExistFile(t, filepath.Join(outDir, "cmd", "main.go"))
		mustExistFile(t, filepath.Join(outDir, "cmd", "custom_routes.go"))
		mustContainFile(t, filepath.Join(outDir, "cmd", "generated_routes.go"), "/api/idl/userservice")
		mustContainFile(t, filepath.Join(outDir, "client", "userservice", "demo.go"), "/api/idl/userservice")
		mustContainFile(t, filepath.Join(outDir, "sdk", "userservicesdk", "client.go"), "/api/idl/userservice")
		mustContainFile(t, filepath.Join(outDir, "docs", "openapi.json"), `"openapi": "3.1.0"`)
		mustContainFile(t, filepath.Join(outDir, "docs", "schema.json"), `"$schema": "https://json-schema.org/draft/2020-12/schema"`)
		mustContainFile(t, filepath.Join(outDir, "sdk", "typescript", "client.ts"), "/api/idl/userservice")
		mustContainFile(t, filepath.Join(outDir, "sdk", "typescript", "client.ts"), "export class UserServiceClient")

		// Verify generated service package compiles (it only depends on go-kit itself)
		buildCmd := exec.Command("go", "build", "-mod=mod", "./service/...")
		buildCmd.Dir = outDir
		buildCmd.Env = append(os.Environ(), "GONOSUMDB=*")
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Errorf("generated service/ failed to compile: %v\n%s", err, out)
		}
	})

	t.Run("IDL_Rerun_RefreshesGeneratedDocsAndPreservesCustomFiles", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_rerun")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		run := func() {
			cmd := exec.Command("go", "run", microgenPath,
				"-idl", idlFile,
				"-out", outDir,
				"-import", "example.com/gen_idl_rerun",
				"-openapi",
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("microgen idl rerun failed: %v\n%s", err, out)
			}
		}

		run()

		goModPath := filepath.Join(outDir, "go.mod")
		docsPath := filepath.Join(outDir, "docs", "docs.go")
		customRoutesPath := filepath.Join(outDir, "cmd", "custom_routes.go")
		customChainPath := filepath.Join(outDir, "endpoint", "userservice", "custom_chain.go")

		goMod := readFile(t, goModPath)
		goMod += "\nrequire example.com/custom v0.0.0\n"
		if err := os.WriteFile(goModPath, []byte(goMod), 0o644); err != nil {
			t.Fatalf("write go.mod: %v", err)
		}

		staleDocs := `package docs

	// Stale generated docs should be refreshed on reruns.
	`
		if err := os.WriteFile(docsPath, []byte(strings.TrimSpace(staleDocs)+"\n"), 0o644); err != nil {
			t.Fatalf("write stale docs.go: %v", err)
		}

		customRoutes := `package main

	import "net/http"

	func registerCustomRoutes(r *http.ServeMux) []generatedRouteEntry {
		r.HandleFunc("GET /custom/ping", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(204)
		})
		return []generatedRouteEntry{
			{Method: "GET", Path: "/custom/ping", Handler: "custom-ping"},
		}
	}
	`
		if err := os.WriteFile(customRoutesPath, []byte(strings.TrimSpace(customRoutes)+"\n"), 0o644); err != nil {
			t.Fatalf("write custom_routes.go: %v", err)
		}

		customChain := `package userservice

	import (
		"github.com/dreamsxin/go-kit/v2/endpoint"
		kitlog "github.com/dreamsxin/go-kit/v2/log"
	)

	func applyCustomMiddleware(ep endpoint.Endpoint, logger *kitlog.Logger, cfg MiddlewareConfig, name string) endpoint.Endpoint {
		_ = logger
		_ = cfg
		_ = name
		return ep
	}
	`
		if err := os.WriteFile(customChainPath, []byte(strings.TrimSpace(customChain)+"\n"), 0o644); err != nil {
			t.Fatalf("write custom_chain.go: %v", err)
		}

		run()

		mustContainFile(t, goModPath, "require example.com/custom v0.0.0")
		mustContainFile(t, docsPath, "go:embed openapi.json")
		mustContainFile(t, docsPath, "go:embed schema.json")
		mustContainFile(t, filepath.Join(outDir, "docs", "openapi.json"), `"openapi": "3.1.0"`)
		mustContainFile(t, filepath.Join(outDir, "docs", "schema.json"), `"$defs"`)
		mustContainFile(t, filepath.Join(outDir, "sdk", "typescript", "client.ts"), "export class UserServiceClient")
		mustContainFile(t, customRoutesPath, "/custom/ping")
		mustContainFile(t, customChainPath, "applyCustomMiddleware")
	})

	t.Run("IDL_CustomRoutes_ArePreservedAndServed", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_custom_routes")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_custom_routes",
			"-config=false",
			"-docs=false",
			"-model=false",
			"-db=false",
			"-skill=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen custom-routes fixture failed: %v\n%s", err, out)
		}

		customRoutesPath := filepath.Join(outDir, "cmd", "custom_routes.go")
		customRoutes := `package main

	import "net/http"

	func registerCustomRoutes(r *http.ServeMux) []generatedRouteEntry {
		r.HandleFunc("GET /custom/ping", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		return []generatedRouteEntry{
			{Method: "GET", Path: "/custom/ping", Handler: "custom-ping"},
		}
	}
	`
		if err := os.WriteFile(customRoutesPath, []byte(strings.TrimSpace(customRoutes)+"\n"), 0o644); err != nil {
			t.Fatalf("write custom_routes.go: %v", err)
		}

		binName := "microgen_custom_routes_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		buildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		buildCmd.Dir = outDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("custom-routes project build failed: %v\n%s", err, out)
		}
		defer os.Remove(filepath.Join(outDir, binName))

		httpAddr := freeTCPAddr(t)
		baseURL := "http://" + httpAddr
		runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr)
		runCmd.Dir = outDir
		runCmd.Env = os.Environ()
		if err := runCmd.Start(); err != nil {
			t.Fatalf("failed to start custom-routes project: %v", err)
		}
		defer killCmd(t, runCmd)

		waitServer(t, baseURL+"/health")
		resp, err := http.Get(baseURL + "/custom/ping")
		if err != nil {
			t.Fatalf("GET /custom/ping failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("GET /custom/ping: want status %d, got %d", http.StatusNoContent, resp.StatusCode)
		}

		expectStatusContains(t, "GET", baseURL+"/debug/routes", "", http.StatusOK, "/custom/ping")
	})

}
