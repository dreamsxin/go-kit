// Command releaseverify runs the cross-platform v2 release suites.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type module struct {
	name string
	dir  string
}

func main() {
	rootFlag := flag.String("root", "..", "v2 module root, resolved from the current directory")
	suitesFlag := flag.String("suites", "test,standalone,vet,tidy", "comma-separated suites: fmt,test,standalone,vet,tidy,race")
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
	for _, suite := range suites {
		fmt.Printf("\n>>> release suite: %s\n", suite)
		switch suite {
		case "fmt":
			// Formatting used to be checked only by `make verify`, which means
			// drift reached main whenever somebody did not run it. The check
			// itself lives in makeguard, so the local target and this suite
			// cannot disagree about what counts as formatted.
			run(filepath.Join(root, "tools"), nil, "go", "run", "./makeguard", "gofmt", root)
		case "test":
			runForModules(maintained, nil, "go", "test", "./...", "-count=1")
		case "standalone":
			// The published module is the root module, and it requires nothing
			// from this repository, so leaving the workspace needs no tag to
			// exist and no proxy round trip.
			runForModules(publishableModules(root), []string{"GOWORK=off"}, "go", "test", "./...", "-count=1")
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
		"fmt": {}, "test": {}, "standalone": {}, "vet": {}, "tidy": {}, "race": {},
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

// maintainedModules lists every module in the workspace. Only the first is
// published; the rest exist to exercise and generate it.
func maintainedModules(root string) []module {
	return []module{
		{name: "core", dir: root},
		{name: "examples", dir: filepath.Join(root, "examples")},
		{name: "tools", dir: filepath.Join(root, "tools")},
		{name: "contractcheck", dir: filepath.Join(root, "tools", "contractcheck")},
	}
}

// publishableModules lists the modules a consumer can require.
func publishableModules(root string) []module {
	return []module{{name: "core", dir: root}}
}

func runForModules(modules []module, extraEnv []string, name string, args ...string) {
	for _, module := range modules {
		fmt.Printf("\n--- %s: %s %s\n", module.name, name, strings.Join(args, " "))
		run(module.dir, extraEnv, name, args...)
	}
}

// runRace runs the core module's tests under the race detector.
//
// Every package, not a curated list: a list has to be edited whenever a package
// is added, and the ones that get forgotten are the ones nobody thought about
// concurrency for. The extra packages cost a few seconds against a suite that
// already takes tens.
func runRace(root string) {
	run(root, []string{"CGO_ENABLED=1"}, "go", "test", "-race", "./...", "-count=1")
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
