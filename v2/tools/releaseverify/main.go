// Command releaseverify runs the cross-platform v2 release suites.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type module struct {
	name string
	dir  string
}

func main() {
	rootFlag := flag.String("root", "..", "v2 module root, resolved from the current directory")
	suitesFlag := flag.String("suites", "test,standalone,vet,tidy", "comma-separated suites: test,standalone,vet,tidy,race")
	flag.Parse()

	root, err := filepath.Abs(*rootFlag)
	if err != nil {
		fatalf("resolve v2 root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "RELEASE_MANIFEST.json")); err != nil {
		fatalf("%s is not a v2 release root: %v", root, err)
	}
	suites, err := parseSuites(*suitesFlag)
	if err != nil {
		fatalf("parse suites: %v", err)
	}

	maintained := maintainedModules(root)
	publishable := publishableModules(root)
	for _, suite := range suites {
		fmt.Printf("\n>>> release suite: %s\n", suite)
		switch suite {
		case "test":
			runForModules(maintained, nil, "go", "test", "./...", "-count=1")
		case "standalone":
			runForModules(publishable, []string{"GOWORK=off"}, "go", "test", "./...", "-count=1")
		case "vet":
			runForModules(maintained, nil, "go", "vet", "./...")
		case "tidy":
			runForModules(maintained, nil, "go", "mod", "tidy")
			repoRoot := strings.TrimSpace(commandOutput(root, nil, "git", "rev-parse", "--show-toplevel"))
			scope, err := filepath.Rel(repoRoot, root)
			if err != nil || scope == ".." || strings.HasPrefix(scope, ".."+string(filepath.Separator)) {
				fatalf("resolve v2 git scope from %s: %v", repoRoot, err)
			}
			run(repoRoot, nil, "git", "diff", "--exit-code", "--", filepath.ToSlash(scope))
		case "race":
			runRace(root)
		}
	}
}

func parseSuites(value string) ([]string, error) {
	allowed := map[string]struct{}{
		"test": {}, "standalone": {}, "vet": {}, "tidy": {}, "race": {},
	}
	seen := make(map[string]struct{})
	var suites []string
	for _, raw := range strings.Split(value, ",") {
		suite := strings.TrimSpace(raw)
		if suite == "" {
			continue
		}
		if _, ok := allowed[suite]; !ok {
			return nil, fmt.Errorf("unsupported suite %q", suite)
		}
		if _, ok := seen[suite]; ok {
			continue
		}
		seen[suite] = struct{}{}
		suites = append(suites, suite)
	}
	if len(suites) == 0 {
		return nil, fmt.Errorf("at least one suite is required")
	}
	return suites, nil
}

func maintainedModules(root string) []module {
	return []module{
		{name: "core", dir: root},
		{name: "microgen", dir: filepath.Join(root, "cmd", "microgen")},
		{name: "examples", dir: filepath.Join(root, "examples")},
		{name: "consul", dir: filepath.Join(root, "integrations", "consul")},
		{name: "etcd", dir: filepath.Join(root, "integrations", "etcd")},
		{name: "grpc", dir: filepath.Join(root, "integrations", "grpc")},
		{name: "zap", dir: filepath.Join(root, "integrations", "zap")},
		{name: "kit-grpc", dir: filepath.Join(root, "kit", "grpc")},
		{name: "otel", dir: filepath.Join(root, "observability", "otel")},
		{name: "tools", dir: filepath.Join(root, "tools")},
		{name: "contractcheck", dir: filepath.Join(root, "tools", "contractcheck")},
	}
}

func publishableModules(root string) []module {
	modules := maintainedModules(root)
	var publishable []module
	for _, module := range modules {
		switch module.name {
		case "examples", "tools", "contractcheck":
			continue
		default:
			publishable = append(publishable, module)
		}
	}
	return publishable
}

func runForModules(modules []module, extraEnv []string, name string, args ...string) {
	for _, module := range modules {
		fmt.Printf("\n--- %s: %s %s\n", module.name, name, strings.Join(args, " "))
		run(module.dir, extraEnv, name, args...)
	}
}

func runRace(root string) {
	checks := []struct {
		dir      string
		packages []string
	}{
		{
			dir: root,
			packages: []string{
				"./endpoint", "./kit", "./interaction/...", "./transport/...",
				"./sd/...", "./security/http", "./observability/slog",
			},
		},
		{dir: filepath.Join(root, "cmd", "microgen"), packages: []string{"./internal/generator"}},
		{dir: filepath.Join(root, "integrations", "consul"), packages: []string{"./..."}},
		{dir: filepath.Join(root, "integrations", "grpc"), packages: []string{"./..."}},
		{dir: filepath.Join(root, "integrations", "zap"), packages: []string{"./..."}},
		{dir: filepath.Join(root, "kit", "grpc"), packages: []string{"./..."}},
		{dir: filepath.Join(root, "observability", "otel"), packages: []string{"./..."}},
	}
	for _, check := range checks {
		args := append([]string{"test", "-race"}, check.packages...)
		run(check.dir, []string{"CGO_ENABLED=1"}, "go", args...)
	}
}

func run(dir string, extraEnv []string, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = commandEnv(extraEnv)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatalf("%s: %s %s: %v", filepath.ToSlash(dir), name, strings.Join(args, " "), err)
	}
}

func commandOutput(dir string, extraEnv []string, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = commandEnv(extraEnv)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fatalf("%s: %s %s: %v\n%s", filepath.ToSlash(dir), name, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func commandEnv(overrides []string) []string {
	if len(overrides) == 0 {
		return os.Environ()
	}
	keys := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		key, _, _ := strings.Cut(override, "=")
		keys[strings.ToUpper(key)] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, current := range os.Environ() {
		key, _, _ := strings.Cut(current, "=")
		if _, replaced := keys[strings.ToUpper(key)]; !replaced {
			environment = append(environment, current)
		}
	}
	return append(environment, overrides...)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// Keep command output stable when module lists become data-driven later.
func sortedModuleNames(modules []module) []string {
	names := make([]string, 0, len(modules))
	for _, module := range modules {
		names = append(names, module.name)
	}
	sort.Strings(names)
	return names
}
