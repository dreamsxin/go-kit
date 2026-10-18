package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// moduleFiles are the files `go mod tidy` is allowed to rewrite. Nothing else
// in the tree is compared, which is what makes the suite usable before a commit:
// the question it answers is whether go.mod and go.sum match the source, not
// whether the working tree is clean.
var moduleFiles = []string{"go.mod", "go.sum"}

// moduleFilesState records the contents of the files that tidy may touch. A
// missing key means the file did not exist, so a tidy that creates it is a
// change and a restore that finds one removes it again.
type moduleFilesState map[string][]byte

func readModuleFiles(dir string) (moduleFilesState, error) {
	state := make(moduleFilesState, len(moduleFiles))
	for _, name := range moduleFiles {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		state[name] = data
	}
	return state, nil
}

func restoreModuleFiles(dir string, state moduleFilesState) error {
	for _, name := range moduleFiles {
		path := filepath.Join(dir, name)
		data, existed := state[name]
		if !existed {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// changedModuleFiles reports the files whose content differs after tidy. Line
// endings are normalized: on Windows the working tree may hold CRLF while the
// toolchain writes LF, and that is not a module change.
func changedModuleFiles(before, after moduleFilesState) []string {
	var changed []string
	for _, name := range moduleFiles {
		old, had := before[name]
		now, has := after[name]
		if had != has {
			changed = append(changed, name)
			continue
		}
		if had && !bytes.Equal(normalizeLineEndings(old), normalizeLineEndings(now)) {
			changed = append(changed, name)
		}
	}
	return changed
}

func normalizeLineEndings(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

// verifyTidy runs `go mod tidy` in every maintained module, compares go.mod and
// go.sum against their previous contents, and restores the tree. A module whose
// files tidy would rewrite fails the suite with a message naming the file; a
// module whose files are already tidy leaves the working tree byte-identical,
// so the check can run on an uncommitted tree.
func verifyTidy(modules []module) error {
	var problems []string
	for _, module := range modules {
		before, err := readModuleFiles(module.dir)
		if err != nil {
			return fmt.Errorf("%s: read module files: %w", module.name, err)
		}

		fmt.Printf("\n--- %s: go mod tidy\n", module.name)
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Dir = module.dir
		cmd.Env = commandEnv(nil)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		runErr := cmd.Run()

		after, readErr := readModuleFiles(module.dir)
		restoreErr := restoreModuleFiles(module.dir, before)
		if runErr != nil {
			return fmt.Errorf("%s: go mod tidy: %w", module.name, runErr)
		}
		if readErr != nil {
			return fmt.Errorf("%s: read module files after tidy: %w", module.name, readErr)
		}
		if restoreErr != nil {
			return fmt.Errorf("%s: restore module files: %w", module.name, restoreErr)
		}
		for _, name := range changedModuleFiles(before, after) {
			problems = append(problems, fmt.Sprintf("%s/%s", module.name, name))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("go mod tidy would rewrite %v; run `go mod tidy` in each module and commit the result",
			problems)
	}
	return nil
}
