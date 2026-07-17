package tools_test

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMicrogenConfigIntegration(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	microgenPath := microgenMainPath(t)

	t.Run("IDL_Config_RemoteConsul_UsesRemoteAndFallsBackToLocal", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_remote_config")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_remote_config",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen remote-config fixture failed: %v\n%s", err, out)
		}

		probePkg := writeConfigRemoteProbe(t, outDir, "remoteconfigprobe", "example.com/gen_idl_remote_config")

		remotePayload := strings.Join([]string{
			"server:",
			"  http_addr: \":19090\"",
			"logging:",
			"  level: \"debug\"",
			"",
		}, "\n")
		encodedPayload := base64.StdEncoding.EncodeToString([]byte(remotePayload))
		remote := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/kv/microgen/config" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[{"Key":"microgen/config","Value":"`+encodedPayload+`"}]`)
		})
		testServer := httptest.NewServer(remote)
		defer testServer.Close()

		successConfig := fmt.Sprintf(strings.Join([]string{
			"server:",
			"  http_addr: \":8080\"",
			"logging:",
			"  level: \"info\"",
			"remote:",
			"  enabled: true",
			"  provider: \"consul\"",
			"  endpoint: %q",
			"  data_id: \"microgen/config\"",
			"  fallback_to_local: true",
			"",
		}, "\n"), testServer.URL)
		successConfigPath := filepath.Join(outDir, "config", "remote-success.yaml")
		if err := os.WriteFile(successConfigPath, []byte(successConfig), 0o644); err != nil {
			t.Fatalf("write success config: %v", err)
		}

		successProbe := exec.Command("go", "run", "-mod=mod", probePkg, "./config/remote-success.yaml")
		successProbe.Dir = outDir
		successProbe.Env = append(os.Environ(), "GOPROXY=https://proxy.golang.org,direct")
		successOut := runCommand(t, successProbe)
		if !strings.Contains(successOut, ":19090") {
			t.Fatalf("expected remote config to override http addr, got:\n%s", successOut)
		}
		if !strings.Contains(successOut, "debug") {
			t.Fatalf("expected remote config to override log level, got:\n%s", successOut)
		}

		envProbe := exec.Command("go", "run", "-mod=mod", probePkg, "./config/remote-success.yaml")
		envProbe.Dir = outDir
		envProbe.Env = append(os.Environ(),
			"GOPROXY=https://proxy.golang.org,direct",
			"APP_HTTP_ADDR=:29090",
			"APP_LOG_LEVEL=error",
		)
		envOut := runCommand(t, envProbe)
		if !strings.Contains(envOut, ":29090") || !strings.Contains(envOut, "error") {
			t.Fatalf("expected env to override remote config, got:\n%s", envOut)
		}

		fallbackAddr := freeTCPAddr(t)
		fallbackConfig := fmt.Sprintf(strings.Join([]string{
			"server:",
			"  http_addr: \":28080\"",
			"logging:",
			"  level: \"warn\"",
			"remote:",
			"  enabled: true",
			"  provider: \"consul\"",
			"  endpoint: \"http://%s\"",
			"  data_id: \"microgen/config\"",
			"  fallback_to_local: true",
			"",
		}, "\n"), fallbackAddr)
		fallbackConfigPath := filepath.Join(outDir, "config", "remote-fallback.yaml")
		if err := os.WriteFile(fallbackConfigPath, []byte(fallbackConfig), 0o644); err != nil {
			t.Fatalf("write fallback config: %v", err)
		}

		fallbackProbe := exec.Command("go", "run", "-mod=mod", probePkg, "./config/remote-fallback.yaml")
		fallbackProbe.Dir = outDir
		fallbackProbe.Env = append(os.Environ(), "GOPROXY=https://proxy.golang.org,direct")
		fallbackOut := runCommand(t, fallbackProbe)
		if !strings.Contains(fallbackOut, ":28080") {
			t.Fatalf("expected fallback config to keep local http addr, got:\n%s", fallbackOut)
		}
		if !strings.Contains(fallbackOut, "warn") {
			t.Fatalf("expected fallback config to keep local log level, got:\n%s", fallbackOut)
		}
	})

	t.Run("IDL_Config_RemoteConsul_StrictModeFailsWithoutFallback", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_idl_remote_config_strict")
		os.RemoveAll(outDir)

		idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_remote_config_strict",
			"-config-mode", "remote",
			"-remote-provider", "consul",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen strict remote-config fixture failed: %v\n%s", err, out)
		}

		configYAML := filepath.Join(outDir, "config", "config.yaml")
		mustContainFile(t, configYAML, "enabled: true")
		mustContainFile(t, configYAML, `provider: "consul"`)
		mustContainFile(t, configYAML, "fallback_to_local: false")

		probePkg := writeConfigRemoteProbe(t, outDir, "remoteconfigstrictprobe", "example.com/gen_idl_remote_config_strict")
		probe := exec.Command("go", "run", "-mod=mod", probePkg, "./config/config.yaml")
		probe.Dir = outDir
		probe.Env = append(os.Environ(), "GOPROXY=https://proxy.golang.org,direct")
		out, err := probe.CombinedOutput()
		if err == nil {
			t.Fatalf("expected strict remote config probe to fail, got success:\n%s", out)
		}
		if !strings.Contains(string(out), "remote consul endpoint is empty") {
			t.Fatalf("expected strict remote failure to mention empty endpoint, got:\n%s", out)
		}
	})

}
