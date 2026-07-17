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

func TestMicrogenExtendIntegration(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	microgenPath := microgenMainPath(t)

	t.Run("IDL_Extend_AppendService_PreservesExistingFilesAndServesNewRoute", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_extend_append")
		os.RemoveAll(outDir)

		baseIDL := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", baseIDL,
			"-out", outDir,
			"-import", "example.com/gen_idl_extend_append",
			"-config=false",
			"-docs=false",
			"-model=false",
			"-db=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("initial microgen idl failed: %v\n%s", err, out)
		}

		userServicePath := filepath.Join(outDir, "service", "userservice", "service.go")
		originalUserService, err := os.ReadFile(userServicePath)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", userServicePath, err)
		}
		customMarker := "// preserved customization"
		if err := os.WriteFile(userServicePath, append(originalUserService, []byte("\n"+customMarker+"\n")...), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", userServicePath, err)
		}

		baseContent, err := os.ReadFile(baseIDL)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", baseIDL, err)
		}
		fullIDL := string(baseContent) + `

	type PlaceOrderRequest struct {
		UserID uint ` + "`json:\"user_id\"`" + `
	}
	type PlaceOrderResponse struct {
		OrderID uint   ` + "`json:\"order_id\"`" + `
		Error   string ` + "`json:\"error\"`" + `
	}

	type OrderService interface {
		PlaceOrder(ctx context.Context, req PlaceOrderRequest) (PlaceOrderResponse, error)
	}
	`
		fullIDLPath := filepath.Join(t.TempDir(), "combined_extend.go")
		if err := os.WriteFile(fullIDLPath, []byte(fullIDL), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", fullIDLPath, err)
		}

		extendCmd := exec.Command("go", "run", microgenPath, "extend",
			"-idl", fullIDLPath,
			"-out", outDir,
			"-append-service", "OrderService",
		)
		if out, err := extendCmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen extend append-service failed: %v\n%s", err, out)
		}

		mustExistFile(t, filepath.Join(outDir, "service", "orderservice", "service.go"))
		mustContainFile(t, filepath.Join(outDir, "cmd", "generated_routes.go"), "/placeorder")
		mustContainFile(t, filepath.Join(outDir, "skill", "skill.go"), "PlaceOrder")
		mustContainFile(t, userServicePath, customMarker)

		binName := "microgen_extend_append_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}

		buildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		buildCmd.Dir = outDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("extended project build failed: %v\n%s", err, out)
		}
		binPath := filepath.Join(outDir, binName)
		defer os.Remove(binPath)

		httpAddr := freeTCPAddr(t)
		baseURL := "http://" + httpAddr
		runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr)
		runCmd.Dir = outDir
		runCmd.Env = os.Environ()
		if err := runCmd.Start(); err != nil {
			t.Fatalf("failed to start extended project: %v", err)
		}
		defer killCmd(t, runCmd)

		waitServer(t, baseURL+"/health")
		expectJSONStatusContains(t, "POST", baseURL+"/createuser", `{"username":"alice","email":"alice@example.com"}`, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		expectJSONStatusContains(t, "POST", baseURL+"/placeorder", `{"user_id":1}`, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		expectStatusContains(t, "GET", baseURL+"/skill", "", http.StatusOK, "PlaceOrder")
	})

	t.Run("IDL_Extend_AppendModel_PreservesExistingHooksAndBuilds", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_extend_append_model")
		os.RemoveAll(outDir)

		baseIDL := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", baseIDL,
			"-out", outDir,
			"-import", "example.com/gen_idl_extend_append_model",
			"-config=false",
			"-docs=false",
			"-model=true",
			"-db=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("initial microgen idl for append-model failed: %v\n%s", err, out)
		}

		userHooksPath := filepath.Join(outDir, "model", "user.go")
		originalHooks, err := os.ReadFile(userHooksPath)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", userHooksPath, err)
		}
		customMarker := "// preserved model customization"
		if err := os.WriteFile(userHooksPath, append(originalHooks, []byte("\n"+customMarker+"\n")...), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", userHooksPath, err)
		}

		baseContent, err := os.ReadFile(baseIDL)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", baseIDL, err)
		}
		fullIDL := string(baseContent) + `

	type Product struct {
		ID   uint   ` + "`json:\"id\" gorm:\"primaryKey\"`" + `
		Name string ` + "`json:\"name\" gorm:\"not null\"`" + `
	}
	`
		fullIDLPath := filepath.Join(t.TempDir(), "combined_model_extend.go")
		if err := os.WriteFile(fullIDLPath, []byte(fullIDL), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", fullIDLPath, err)
		}

		extendCmd := exec.Command("go", "run", microgenPath, "extend",
			"-idl", fullIDLPath,
			"-out", outDir,
			"-append-model", "Product",
		)
		if out, err := extendCmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen extend append-model failed: %v\n%s", err, out)
		}

		mustExistFile(t, filepath.Join(outDir, "model", "generated_product.go"))
		mustExistFile(t, filepath.Join(outDir, "model", "product.go"))
		mustExistFile(t, filepath.Join(outDir, "repository", "generated_product_repository.go"))
		mustContainFile(t, filepath.Join(outDir, "service", "userservice", "generated_repos.go"), "ProductRepo *repository.ProductRepository")
		mustContainFile(t, userHooksPath, customMarker)

		buildTargets := []string{
			"./cmd",
			"./service/...",
			"./model/...",
			"./repository/...",
		}
		buildCmd := exec.Command("go", append([]string{"build", "-mod=mod"}, buildTargets...)...)
		buildCmd.Dir = outDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("append-model project build failed: %v\n%s", err, out)
		}
	})

	t.Run("IDL_Extend_AppendMiddleware_PreservesCustomChainAndServesWrappedErrors", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_extend_append_middleware")
		os.RemoveAll(outDir)

		baseIDL := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", baseIDL,
			"-out", outDir,
			"-import", "example.com/gen_idl_extend_append_middleware",
			"-config=false",
			"-docs=false",
			"-model=false",
			"-db=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("initial microgen idl for append-middleware failed: %v\n%s", err, out)
		}

		customChainPath := filepath.Join(outDir, "endpoint", "userservice", "custom_chain.go")
		originalChain, err := os.ReadFile(customChainPath)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", customChainPath, err)
		}
		customMarker := "// preserved custom middleware"
		if err := os.WriteFile(customChainPath, append(originalChain, []byte("\n"+customMarker+"\n")...), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", customChainPath, err)
		}

		extendCmd := exec.Command("go", "run", microgenPath, "extend",
			"-idl", baseIDL,
			"-out", outDir,
			"-append-middleware", "tracing,error-handling,metrics",
		)
		if out, err := extendCmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen extend append-middleware failed: %v\n%s", err, out)
		}

		generatedChainPath := filepath.Join(outDir, "endpoint", "userservice", "generated_chain.go")
		mustContainFile(t, generatedChainPath, "endpoint.TracingMiddleware()")
		mustContainFile(t, generatedChainPath, "endpoint.ErrorHandlingMiddleware(name)")
		mustContainFile(t, generatedChainPath, "endpoint.MetricsMiddleware(generatedMetrics(name))")
		mustContainFile(t, customChainPath, customMarker)

		buildTargets := []string{
			"./cmd",
			"./endpoint/...",
			"./service/...",
		}
		buildCmd := exec.Command("go", append([]string{"build", "-mod=mod"}, buildTargets...)...)
		buildCmd.Dir = outDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("append-middleware project build failed: %v\n%s", err, out)
		}

		binName := "microgen_append_middleware_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		serverBuildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		serverBuildCmd.Dir = outDir
		if out, err := serverBuildCmd.CombinedOutput(); err != nil {
			t.Fatalf("append-middleware server build failed: %v\n%s", err, out)
		}
		defer os.Remove(filepath.Join(outDir, binName))

		httpAddr := freeTCPAddr(t)
		baseURL := "http://" + httpAddr
		runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr)
		runCmd.Dir = outDir
		runCmd.Env = os.Environ()
		if err := runCmd.Start(); err != nil {
			t.Fatalf("failed to start append-middleware project: %v", err)
		}
		defer killCmd(t, runCmd)

		waitServer(t, baseURL+"/health")
		expectJSONStatusContains(t, "POST", baseURL+"/createuser", `{"username":"mw-user","email":"mw@example.com"}`, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
	})

	t.Run("IDL_Extend_Check_ReportsCompatibility", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_extend_check")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_extend_check",
			"-config=false",
			"-docs=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("initial microgen idl for extend-check failed: %v\n%s", err, out)
		}

		checkCmd := exec.Command("go", "run", microgenPath, "extend",
			"-check",
			"-out", outDir,
		)
		checkOut, err := checkCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("microgen extend -check failed: %v\n%s", err, checkOut)
		}

		report := string(checkOut)
		for _, want := range []string{
			"Extend compatibility for",
			"Module: example.com/gen_idl_extend_check",
			"Generated services seam: cmd/generated_services.go",
			"Generated routes seam: cmd/generated_routes.go",
			"append-service: ready",
			"append-model: ready",
			"append-middleware: ready",
			"Recommended Actions:",
			"append-service: ready; use `microgen extend -out <project> -idl <full-combined.go> -append-service <Name>`",
		} {
			if !strings.Contains(report, want) {
				t.Fatalf("extend-check output missing %q:\n%s", want, report)
			}
		}
	})

	t.Run("IDL_Extend_Check_ReturnsExitCodes", func(t *testing.T) {
		binName := "microgen_extend_check_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		buildMicrogenCmd := exec.Command("go", "build", "-o", binName, "./cmd/microgen")
		buildMicrogenCmd.Dir = root
		if out, err := buildMicrogenCmd.CombinedOutput(); err != nil {
			t.Fatalf("build microgen binary for exit-code test failed: %v\n%s", err, out)
		}
		microgenBin := filepath.Join(root, binName)
		defer os.Remove(microgenBin)

		readyDir := filepath.Join(cwd, "testdata", "gen_idl_extend_check_ready")
		os.RemoveAll(readyDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		generateCmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", readyDir,
			"-import", "example.com/gen_idl_extend_check_ready",
			"-config=false",
			"-docs=false",
		)
		if out, err := generateCmd.CombinedOutput(); err != nil {
			t.Fatalf("initial microgen idl for extend-check exit-code fixture failed: %v\n%s", err, out)
		}

		readyCheckCmd := exec.Command(microgenBin, "extend",
			"-check",
			"-out", readyDir,
		)
		readyOut, err := readyCheckCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("microgen extend -check on ready project failed: %v\n%s", err, readyOut)
		}
		if !strings.Contains(string(readyOut), "- Overall status: ready") {
			t.Fatalf("ready extend-check output missing ready status:\n%s", readyOut)
		}

		legacyDir := filepath.Join(cwd, "testdata", "gen_idl_extend_check_missing")
		os.RemoveAll(legacyDir)
		if err := os.MkdirAll(legacyDir, 0o755); err != nil {
			t.Fatalf("MkdirAll legacyDir: %v", err)
		}
		generateLegacyCmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", legacyDir,
			"-import", "example.com/gen_idl_extend_check_missing",
			"-config=false",
			"-docs=false",
		)
		if out, err := generateLegacyCmd.CombinedOutput(); err != nil {
			t.Fatalf("initial microgen idl for missing-seam fixture failed: %v\n%s", err, out)
		}
		if err := os.Remove(filepath.Join(legacyDir, "cmd", "generated_routes.go")); err != nil {
			t.Fatalf("Remove generated_routes.go: %v", err)
		}

		missingCheckCmd := exec.Command(microgenBin, "extend",
			"-check",
			"-out", legacyDir,
		)
		missingOut, err := missingCheckCmd.CombinedOutput()
		if err == nil {
			t.Fatalf("microgen extend -check on missing-seam project succeeded unexpectedly:\n%s", missingOut)
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("extend-check missing-seam error = %T, want *exec.ExitError", err)
		}
		if exitErr.ExitCode() != 2 {
			t.Fatalf("extend-check missing-seam exit code = %d, want 2\n%s", exitErr.ExitCode(), missingOut)
		}
		if !strings.Contains(string(missingOut), "- Overall status: needs compatibility seams") {
			t.Fatalf("missing-seam extend-check output missing overall status:\n%s", missingOut)
		}
		if !strings.Contains(string(missingOut), "missing: cmd/generated_routes.go") {
			t.Fatalf("missing-seam extend-check output missing seam guidance:\n%s", missingOut)
		}
		if !strings.Contains(string(missingOut), "restore generated route and middleware seams before using `-append-middleware`") {
			t.Fatalf("missing-seam extend-check output missing remediation guidance:\n%s", missingOut)
		}
	})

}
