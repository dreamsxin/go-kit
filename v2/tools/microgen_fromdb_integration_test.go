package tools_test

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMicrogenFromDBIntegration(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	microgenPath := microgenMainPath(t)

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
			"-openapi",
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
		mustExistFile(t, filepath.Join(outDir, "model", "generated_user.go"))
		mustExistFile(t, filepath.Join(outDir, "docs", "openapi.json"))
		mustExistFile(t, filepath.Join(outDir, "docs", "schema.json"))
		mustExistFile(t, filepath.Join(outDir, "sdk", "typescript", "tsconfig.json"))
		mustContainFile(t, filepath.Join(outDir, "idl.go"), "type CreateUserRequest struct")
		mustContainFile(t, filepath.Join(outDir, "idl.go"), "type ListUsersRequest struct")
		mustContainFile(t, filepath.Join(outDir, "transport", "catalogservice", "transport_http.go"), "/user")
		mustContainFile(t, filepath.Join(outDir, "transport", "catalogservice", "transport_http.go"), "/users")
		validateGeneratedContracts(t, outDir)
		assertGeneratedContractSnapshot(t, "db", outDir)

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
		expectStatusContains(t, "GET", baseURL+"/debug/routes", "", http.StatusNotFound, "404 page not found")
		expectJSONStatusContains(t, "POST", baseURL+"/user", `{"username":"db-user","email":"db@example.com"}`, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
	})

}
