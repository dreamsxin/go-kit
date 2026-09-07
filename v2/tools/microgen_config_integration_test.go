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
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestMicrogenConfigIntegration(t *testing.T) {
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)

	t.Run("IDL_Config_CustomHooksAreUserOwned", func(t *testing.T) {
		outDir := generatedProjectDir(t, "gen_idl_custom_config")
		idlFile := filepath.Join(root, "cmd", "microgen", "internal", "parser", "testdata", "basic.go")
		generateArgs := []string{
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_custom_config",
			"-config",
			"-docs=false",
		}
		if out, err := microgenCommand(t, generateArgs...).CombinedOutput(); err != nil {
			t.Fatalf("microgen custom-config fixture failed: %v\n%s", err, out)
		}

		customPath := filepath.Join(outDir, "config", "custom.go")
		customSource := `package config

import (
	"fmt"
	"os"
)

type CustomConfig struct {
	RedisAddr string ` + "`yaml:\"redis_addr\" mapstructure:\"redis_addr\"`" + `
}

func (cfg *CustomConfig) SetDefaults() { cfg.RedisAddr = "default:6379" }

func (cfg *CustomConfig) ApplyEnv() error {
	if value, ok := os.LookupEnv("APP_REDIS_ADDR"); ok {
		cfg.RedisAddr = value
	}
	return nil
}

func (cfg *CustomConfig) Validate() error {
	if cfg.RedisAddr == "" {
		return fmt.Errorf("redis_addr is required")
	}
	return nil
}
`
		if err := os.WriteFile(customPath, []byte(customSource), 0o644); err != nil {
			t.Fatal(err)
		}
		configPath := filepath.Join(outDir, "config", "custom.yaml")
		if err := os.WriteFile(configPath, []byte("custom:\n  redis_addr: yaml:6379\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		probeDir, err := os.MkdirTemp(outDir, ".customconfigprobe-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(probeDir)
		probeSource := `package main

import (
	"fmt"
	"os"

	"example.com/gen_idl_custom_config/config"
)

func main() {
	cfg, err := config.Load(os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Println(cfg.Custom.RedisAddr)
}
`
		if err := os.WriteFile(filepath.Join(probeDir, "main.go"), []byte(probeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		probeRel, err := filepath.Rel(outDir, probeDir)
		if err != nil {
			t.Fatal(err)
		}
		probePkg := "./" + filepath.ToSlash(probeRel)

		probe := exec.Command("go", "run", "-mod=mod", probePkg, "./config/custom.yaml")
		probe.Dir = outDir
		if out := runCommand(t, probe); !strings.Contains(out, "yaml:6379") {
			t.Fatalf("custom YAML hook output = %q", out)
		}
		probe = exec.Command("go", "run", "-mod=mod", probePkg, "./config/custom.yaml")
		probe.Dir = outDir
		probe.Env = append(os.Environ(), "APP_REDIS_ADDR=env:6379")
		if out := runCommand(t, probe); !strings.Contains(out, "env:6379") {
			t.Fatalf("custom env hook output = %q", out)
		}

		if out, err := microgenCommand(t, generateArgs...).CombinedOutput(); err != nil {
			t.Fatalf("microgen custom-config rerun failed: %v\n%s", err, out)
		}
		mustContainFile(t, customPath, "APP_REDIS_ADDR")
	})

	t.Run("IDL_Config_RemoteConsul_UsesRemoteAndFallsBackToLocal", func(t *testing.T) {
		outDir := generatedProjectDir(t, "gen_idl_remote_config")

		idlFile := filepath.Join(root, "cmd", "microgen", "internal", "parser", "testdata", "basic.go")
		cmd := microgenCommand(t,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_remote_config",
			"-config",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen remote-config fixture failed: %v\n%s", err, out)
		}
		compile := exec.Command("go", "test", "-mod=mod", "./...")
		compile.Dir = outDir
		if out, err := compile.CombinedOutput(); err != nil {
			t.Fatalf("generated config project does not compile: %v\n%s", err, out)
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
		outDir := generatedProjectDir(t, "gen_idl_remote_config_strict")

		idlFile := filepath.Join(root, "cmd", "microgen", "internal", "parser", "testdata", "basic.go")
		cmd := microgenCommand(t,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_remote_config_strict",
			"-config",
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

	// The last two stages of the precedence chain in docs/configuration.md —
	// command-line flags, then Config.Validate — live in the generated main, not
	// in the config package, so this case runs the built binary instead of a
	// probe calling config.Load.
	t.Run("IDL_Config_FlagsOverrideEnvAndAreValidatedAfterwards", func(t *testing.T) {
		outDir := generatedProjectDir(t, "gen_idl_flag_config")

		idlFile := filepath.Join(root, "cmd", "microgen", "internal", "parser", "testdata", "basic.go")
		if out, err := microgenCommand(t,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_flag_config",
			"-config",
			"-docs=false",
			"-model=false",
			"-db=false",
		).CombinedOutput(); err != nil {
			t.Fatalf("microgen flag-config fixture failed: %v\n%s", err, out)
		}

		binName := "microgen_flag_config_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		build := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		build.Dir = outDir
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("generated flag-config project build failed: %v\n%s", err, out)
		}
		defer os.Remove(filepath.Join(outDir, binName))

		envAddr := freeTCPAddr(t)
		flagAddr := freeTCPAddr(t)
		serve := exec.Command("./"+binName, "-http.addr="+flagAddr)
		serve.Dir = outDir
		serve.Env = append(os.Environ(), "APP_HTTP_ADDR="+envAddr)
		if err := serve.Start(); err != nil {
			t.Fatalf("start generated flag-config project: %v", err)
		}
		defer killCmd(t, serve)

		waitServer(t, "http://"+flagAddr+"/health")
		if resp, err := http.Get("http://" + envAddr + "/health"); err == nil {
			resp.Body.Close()
			t.Fatalf("APP_HTTP_ADDR=%s is serving, so the flag stage does not override the environment", envAddr)
		}

		// The file and the environment both hold a valid address here, so only a
		// validation pass that runs after the flag stage can reject this.
		reject := exec.Command("./"+binName, "-http.addr=")
		reject.Dir = outDir
		reject.Env = append(os.Environ(), "APP_HTTP_ADDR="+envAddr)
		out, err := reject.CombinedOutput()
		if err == nil {
			t.Fatalf("expected an empty -http.addr to fail validation, got success:\n%s", out)
		}
		if !strings.Contains(string(out), "invalid config") ||
			!strings.Contains(string(out), "server.http_addr is required") {
			t.Fatalf("expected validation after the flag stage to reject the flag value, got:\n%s", out)
		}
	})

	// TestGeneratedConfigKeysAreDocumented proves the documented keys are the keys
	// the loader mentions. This case proves each one reaches its field: a key read
	// into the wrong target, or read from a struct the loader no longer merges,
	// would satisfy the documentation gate and still do nothing in production.
	t.Run("IDL_Config_EveryDocumentedEnvironmentKeyIsApplied", func(t *testing.T) {
		outDir := generatedProjectDir(t, "gen_idl_env_config")

		idlFile := filepath.Join(root, "cmd", "microgen", "internal", "parser", "testdata", "basic.go")
		if out, err := microgenCommand(t,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_env_config",
			"-protocols", "http,grpc",
			"-config",
			"-config-mode", "hybrid",
			"-remote-provider", "consul",
			"-db",
			"-docs=false",
		).CombinedOutput(); err != nil {
			t.Fatalf("microgen env-config fixture failed: %v\n%s", err, out)
		}

		// Every value differs from the generated default, which the baseline run
		// below confirms rather than assumes, and is its own printed form so the
		// assertion reads as the documented key/value pair.
		overrides := [][2]string{
			{"APP_HTTP_ADDR", "127.0.0.1:18081"},
			{"APP_GRPC_ADDR", "127.0.0.1:18082"},
			{"APP_READ_TIMEOUT", "17s"},
			{"APP_READ_HEADER_TIMEOUT", "18s"},
			{"APP_WRITE_TIMEOUT", "19s"},
			{"APP_GRACEFUL_SHUTDOWN_TIMEOUT", "21s"},
			{"APP_DRAIN_DELAY", "3s"},
			{"APP_METRICS_PATH", "/internal/metrics"},
			{"APP_TLS_CERT_FILE", "/etc/tls/env.crt"},
			{"APP_TLS_KEY_FILE", "/etc/tls/env.key"},
			{"APP_LOG_LEVEL", "warn"},
			{"APP_LOG_FORMAT", "console"},
			{"APP_MIDDLEWARE_TIMEOUT", "22s"},
			{"APP_DB_DRIVER", "postgres"},
			{"APP_DB_DSN", "postgres://env/db"},
			{"APP_DB_AUTO_MIGRATE", "true"},
			{"APP_DB_MAX_OPEN_CONNS", "41"},
			{"APP_DB_MAX_IDLE_CONNS", "7"},
			{"APP_DB_CONN_MAX_LIFETIME", "23s"},
			{"APP_DEBUG_ROUTES_ENABLED", "true"},
			{"APP_DEBUG_PRINT_ROUTES", "true"},
			{"APP_REMOTE_ENABLED", "false"},
			{"APP_REMOTE_PROVIDER", "envconsul"},
			{"APP_REMOTE_ENDPOINT", "http://127.0.0.1:18500"},
			{"APP_REMOTE_NAMESPACE", "envns"},
			{"APP_REMOTE_GROUP", "envgroup"},
			{"APP_REMOTE_DATA_ID", "env/data"},
			{"APP_REMOTE_TIMEOUT", "24s"},
			{"APP_REMOTE_FALLBACK_TO_LOCAL", "false"},
		}

		fields := map[string]string{
			"APP_HTTP_ADDR":                 "Server.HTTPAddr",
			"APP_GRPC_ADDR":                 "Server.GRPCAddr",
			"APP_READ_TIMEOUT":              "Server.ReadTimeout",
			"APP_READ_HEADER_TIMEOUT":       "Server.ReadHeaderTimeout",
			"APP_WRITE_TIMEOUT":             "Server.WriteTimeout",
			"APP_GRACEFUL_SHUTDOWN_TIMEOUT": "Server.GracefulShutdownTimeout",
			"APP_DRAIN_DELAY":               "Server.DrainDelay",
			"APP_METRICS_PATH":              "Server.MetricsPath",
			"APP_TLS_CERT_FILE":             "Server.TLSCertFile",
			"APP_TLS_KEY_FILE":              "Server.TLSKeyFile",
			"APP_LOG_LEVEL":                 "Logging.Level",
			"APP_LOG_FORMAT":                "Logging.Format",
			"APP_MIDDLEWARE_TIMEOUT":        "Middleware.Timeout",
			"APP_DB_DRIVER":                 "Database.Driver",
			"APP_DB_DSN":                    "Database.DSN",
			"APP_DB_AUTO_MIGRATE":           "Database.AutoMigrate",
			"APP_DB_MAX_OPEN_CONNS":         "Database.MaxOpenConns",
			"APP_DB_MAX_IDLE_CONNS":         "Database.MaxIdleConns",
			"APP_DB_CONN_MAX_LIFETIME":      "Database.ConnMaxLifetime",
			"APP_DEBUG_ROUTES_ENABLED":      "Debug.RoutesEnabled",
			"APP_DEBUG_PRINT_ROUTES":        "Debug.PrintRoutes",
			"APP_REMOTE_ENABLED":            "Remote.Enabled",
			"APP_REMOTE_PROVIDER":           "Remote.Provider",
			"APP_REMOTE_ENDPOINT":           "Remote.Endpoint",
			"APP_REMOTE_NAMESPACE":          "Remote.Namespace",
			"APP_REMOTE_GROUP":              "Remote.Group",
			"APP_REMOTE_DATA_ID":            "Remote.DataID",
			"APP_REMOTE_TIMEOUT":            "Remote.Timeout",
			"APP_REMOTE_FALLBACK_TO_LOCAL":  "Remote.FallbackToLocal",
		}

		covered := make([]string, 0, len(overrides))
		for _, override := range overrides {
			if _, ok := fields[override[0]]; !ok {
				t.Fatalf("%s has no field to print, so this case cannot assert it", override[0])
			}
			covered = append(covered, override[0])
		}
		docs, err := os.ReadFile(filepath.Join(root, configurationReferenceDocs[0]))
		if err != nil {
			t.Fatalf("read %s: %v", configurationReferenceDocs[0], err)
		}
		if uncovered, unknown := listDifference(documentedEnvironmentKeys(string(docs)), covered); len(uncovered) > 0 || len(unknown) > 0 {
			if len(uncovered) > 0 {
				t.Errorf("this case does not set: %s\n\nA documented key nothing sets is a key that "+
					"may never have reached its field.", strings.Join(uncovered, " "))
			}
			if len(unknown) > 0 {
				t.Errorf("this case sets keys the documentation does not list: %s", strings.Join(unknown, " "))
			}
			t.FailNow()
		}

		probeDir, err := os.MkdirTemp(outDir, ".envconfigprobe-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(probeDir)
		var prints strings.Builder
		for _, key := range covered {
			fmt.Fprintf(&prints, "\tfmt.Printf(\"%s=%%v\\n\", cfg.%s)\n", key, fields[key])
		}
		probeSource := `package main

import (
	"fmt"

	"example.com/gen_idl_env_config/config"
)

func main() {
	cfg := config.Default()
	if err := config.ApplyEnv(cfg); err != nil {
		panic(err)
	}
` + prints.String() + `}
`
		if err := os.WriteFile(filepath.Join(probeDir, "main.go"), []byte(probeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		probeRel, err := filepath.Rel(outDir, probeDir)
		if err != nil {
			t.Fatal(err)
		}
		probePkg := "./" + filepath.ToSlash(probeRel)

		runProbe := func(env []string) map[string]string {
			probe := exec.Command("go", "run", "-mod=mod", probePkg)
			probe.Dir = outDir
			probe.Env = env
			values := map[string]string{}
			for _, line := range strings.Split(runCommand(t, probe), "\n") {
				if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
					values[key] = value
				}
			}
			return values
		}

		clean := make([]string, 0, len(os.Environ()))
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "APP_") {
				clean = append(clean, entry)
			}
		}
		defaults := runProbe(clean)
		applied := runProbe(append(append([]string{}, clean...), func() []string {
			set := make([]string, 0, len(overrides))
			for _, override := range overrides {
				set = append(set, override[0]+"="+override[1])
			}
			return set
		}()...))

		for _, override := range overrides {
			key, want := override[0], override[1]
			if defaults[key] == want {
				t.Errorf("%s (%s) defaults to %q, so setting it to that proves nothing", key, fields[key], want)
				continue
			}
			if got := applied[key]; got != want {
				t.Errorf("%s did not reach %s: got %q, want %q", key, fields[key], got, want)
			}
		}
	})

	// The subtest above runs one flag against validation. This one covers the rest
	// of them by shape, because only a project generated with every optional
	// surface declares them all: -auto-migrate used to be written back after
	// cfg.Validate(), so a value that flag introduced was never validated.
	t.Run("IDL_Config_GeneratedMainValidatesAfterEveryFlagWriteBack", func(t *testing.T) {
		outDir := generatedProjectDir(t, "gen_idl_flag_order")

		idlFile := filepath.Join(root, "cmd", "microgen", "internal", "parser", "testdata", "basic.go")
		if out, err := microgenCommand(t,
			"-idl", idlFile,
			"-out", outDir,
			"-import", "example.com/gen_idl_flag_order",
			"-protocols", "http,grpc",
			"-config",
			"-db",
			"-docs=false",
		).CombinedOutput(); err != nil {
			t.Fatalf("microgen flag-order fixture failed: %v\n%s", err, out)
		}

		main := readFile(t, filepath.Join(outDir, "cmd", "main.go"))
		validate := strings.Index(main, "cfg.Validate()")
		if validate < 0 {
			t.Fatal("the generated main does not call cfg.Validate(), so the documented final stage is missing")
		}
		writeBacks := flagWriteBackPattern.FindAllStringIndex(main, -1)
		if len(writeBacks) < 4 {
			// -http.addr, -grpc.addr, -db.dsn and -auto-migrate all write back in
			// this fixture; fewer means the parse no longer sees them.
			t.Fatalf("found only %d flag write-backs in the generated main, so its shape changed", len(writeBacks))
		}
		for _, at := range writeBacks {
			if at[0] > validate {
				t.Errorf("%q runs after cfg.Validate(), so the value that flag introduces is never validated",
					strings.TrimSpace(main[at[0]:at[1]]))
			}
		}
	})
}

// flagWriteBackPattern matches the generated main applying a parsed flag to the
// loaded config, which is the last stage of the documented precedence chain.
var flagWriteBackPattern = regexp.MustCompile(`(?m)^\s*cfg\.[A-Za-z.]+ = \*[A-Za-z]+$`)
