package tools_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"
)

// surfaceWidth caps how many `go doc` processes run at once.
//
// Nearly all of `go doc -all`'s cost is starting a go command and loading one
// package's export data, not rendering the docs: 39 packages take about 100
// seconds one at a time and about 12 at this width. The width is fixed rather
// than derived from GOMAXPROCS because the work is process-bound, not
// CPU-bound, and a fixed number keeps the suite's timing predictable on a CI
// runner that reports a large core count it cannot actually use.
const surfaceWidth = 8

// reviewedSurface caches the declarations of one checkout.
//
// The snapshot test asks "did any declaration change since someone reviewed
// it" and the compatibility test asks "did any of them change in a way that
// breaks a consumer", so both start from the same 39 `go doc` runs and both
// read the same tree without writing to it. Collecting twice was the single
// largest cost in the release suite — 117 seconds for the compatibility gate
// alone — and sharing the read does not weaken either question.
//
// The key is the checkout root, so the tag worktree the compatibility gate
// compares against is collected separately, as it must be.
var reviewedSurface struct {
	sync.Mutex
	byRoot map[string]map[string]string
}

// reviewedDeclarations returns the declaration text of every reviewed package
// under root, keyed by import path.
func reviewedDeclarations(t *testing.T, root string) map[string]string {
	t.Helper()
	if cached, ok := cachedSurface(root); ok {
		return cached
	}
	declarations, err := collectDeclarations(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return rememberSurface(root, declarations)
}

func cachedSurface(root string) (map[string]string, bool) {
	reviewedSurface.Lock()
	defer reviewedSurface.Unlock()
	declarations, ok := reviewedSurface.byRoot[root]
	return declarations, ok
}

// rememberSurface stores a collection unless another test stored one first, so
// two tests racing on a cold cache both finish and agree on a single result.
func rememberSurface(root string, declarations map[string]string) map[string]string {
	reviewedSurface.Lock()
	defer reviewedSurface.Unlock()
	if reviewedSurface.byRoot == nil {
		reviewedSurface.byRoot = map[string]map[string]string{}
	}
	if existing, ok := reviewedSurface.byRoot[root]; ok {
		return existing
	}
	reviewedSurface.byRoot[root] = declarations
	return declarations
}

// collectDeclarations reads every reviewed package's declarations under root.
func collectDeclarations(root string) (map[string]string, error) {
	importPaths, err := reviewedImportPaths(root)
	if err != nil {
		return nil, err
	}

	results := make([]string, len(importPaths))
	errs := make([]error, len(importPaths))
	slots := make(chan struct{}, surfaceWidth)
	var wait sync.WaitGroup
	for i, importPath := range importPaths {
		wait.Add(1)
		go func(i int, importPath string) {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			doc, err := commandIn(root, "go", "doc", "-all", importPath)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = string(normalizeCommandOutput(declarationsOnly(doc)))
		}(i, importPath)
	}
	wait.Wait()

	declarations := make(map[string]string, len(importPaths))
	var failures []string
	for i, importPath := range importPaths {
		if errs[i] != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", importPath, errs[i]))
			continue
		}
		declarations[importPath] = results[i]
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return nil, fmt.Errorf("read the exported surface:\n%s", strings.Join(failures, "\n"))
	}
	return declarations, nil
}

// exportedSurface reads the declarations of every reviewed package under root,
// keyed by import path and then by symbol.
func exportedSurface(t *testing.T, root string) map[string]map[string]string {
	t.Helper()
	declarations := reviewedDeclarations(t, root)
	surface := make(map[string]map[string]string, len(declarations))
	for importPath, text := range declarations {
		surface[importPath] = apiSymbols([]byte(text))
	}
	return surface
}

// reviewedImportPaths lists the packages whose exported surface is reviewed.
//
// Everything ships from one module now, so the list comes from one go list. The
// microgen tree is excluded: it is a command, and its behaviour is pinned by the
// generator contract tests rather than by a doc snapshot.
func reviewedImportPaths(root string) ([]string, error) {
	const microgenPrefix = "github.com/dreamsxin/go-kit/v2/cmd/microgen"

	output, err := commandIn(root, "go", "list", "./...")
	if err != nil {
		return nil, err
	}
	var importPaths []string
	for _, importPath := range strings.Fields(string(output)) {
		if importPath == microgenPrefix || strings.HasPrefix(importPath, microgenPrefix+"/") {
			continue
		}
		importPaths = append(importPaths, importPath)
	}
	sort.Strings(importPaths)
	return importPaths, nil
}

// commandIn runs a command in dir and returns its stdout.
func commandIn(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w\n%s%s", name, strings.Join(args, " "), err, stdout.Bytes(), stderr.Bytes())
	}
	return stdout.Bytes(), nil
}
