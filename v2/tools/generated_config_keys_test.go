package tools_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// configurationReferenceDocs describe the generated configuration loader. They
// are the documents a reader consults for the environment key names.
var configurationReferenceDocs = []string{
	filepath.Join("docs", "configuration.md"),
	filepath.Join("docs", "configuration_zh.md"),
}

// TestGeneratedConfigKeysAreDocumented keeps the documented environment keys and
// the keys the generated loader reads in step.
//
// The documentation used to list six keys under "environment variables use the
// APP_ prefix", reading as a complete set. The loader reads twenty-five: every
// server timeout, the whole APP_REMOTE_ family, the connection pool settings and
// the debug switches were absent, so an operator had no way to learn they exist
// except by reading generated code. Renaming one of them broke nothing either.
//
// Both directions fail: an undocumented key is a setting nobody can find, and a
// documented key the loader does not read is a setting that silently does
// nothing when an operator sets it in production.
func TestGeneratedConfigKeysAreDocumented(t *testing.T) {
	root := goKitRoot(t)
	outDir := generatedProjectDir(t, "gen_config_keys")

	// Every configuration surface at once: the DB, gRPC and remote sections each
	// contribute keys that a narrower project would not emit.
	idlFile := filepath.Join(root, "cmd", "microgen", "internal", "parser", "testdata", "basic.go")
	cmd := microgenCommand(t,
		"-idl", idlFile,
		"-out", outDir,
		"-import", "example.com/gen_config_keys",
		"-protocols", "http,grpc",
		"-config",
		"-config-mode", "hybrid",
		"-remote-provider", "consul",
		"-db",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("microgen failed: %v\n%s", err, out)
	}

	read := loaderEnvironmentKeys(t, filepath.Join(outDir, "config"))
	if len(read) < 20 {
		// A changed loader shape would leave the parse short and let this test
		// pass while comparing almost nothing.
		t.Fatalf("found only %d environment keys in the generated loader, so its shape changed: %v", len(read), read)
	}

	for _, name := range configurationReferenceDocs {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		documented := documentedEnvironmentKeys(string(data))

		missing, stale := pathSetDifference(read, documented)
		// pathSetDifference reports what the second gained and lost against the
		// first: gained means documented but not read, lost means read but not
		// documented.
		if len(stale) > 0 {
			t.Errorf("%s does not document: %s\n\nA setting nobody documents is a setting nobody can find.",
				name, strings.Join(stale, " "))
		}
		if len(missing) > 0 {
			t.Errorf("%s documents keys the loader does not read: %s\n\nAn operator setting one of these in "+
				"production would see nothing happen.", name, strings.Join(missing, " "))
		}
	}
}

var loaderKeyPattern = regexp.MustCompile(`read[A-Za-z]*\("([A-Z0-9_]+)"`)
var envPrefixPattern = regexp.MustCompile(`envPrefix\s*=\s*"([A-Z_]+)"`)

// loaderEnvironmentKeys reads the fully qualified keys the generated loader
// looks up, prefix included, so renaming the prefix fails too.
func loaderEnvironmentKeys(t *testing.T, configDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(configDir)
	if err != nil {
		t.Fatalf("read generated config directory: %v", err)
	}

	prefix := ""
	seen := map[string]struct{}{}
	var suffixes []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(configDir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		text := string(data)
		if match := envPrefixPattern.FindStringSubmatch(text); match != nil {
			prefix = match[1]
		}
		for _, match := range loaderKeyPattern.FindAllStringSubmatch(text, -1) {
			if _, ok := seen[match[1]]; ok {
				continue
			}
			seen[match[1]] = struct{}{}
			suffixes = append(suffixes, match[1])
		}
	}
	if prefix == "" {
		t.Fatal("the generated config package declares no envPrefix")
	}

	keys := make([]string, 0, len(suffixes))
	for _, suffix := range suffixes {
		keys = append(keys, prefix+suffix)
	}
	sort.Strings(keys)
	return keys
}

// documentedKeyPattern matches a row of the key table: the key, then the config
// field it sets. Requiring both columns keeps prose out — `APP_DB_*` written in a
// sentence, or an `APP_FEATURE_*` example belonging to user-owned custom config,
// is not a claim about the generated loader.
var documentedKeyPattern = regexp.MustCompile(`(?m)^(APP_[A-Z0-9_]+)\s+[A-Za-z]`)

// documentedEnvironmentKeys reads the keys a document tabulates.
func documentedEnvironmentKeys(text string) []string {
	seen := map[string]struct{}{}
	for _, match := range documentedKeyPattern.FindAllStringSubmatch(text, -1) {
		seen[match[1]] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
