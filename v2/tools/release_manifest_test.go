package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit-tools/v2/internal/releaseconfig"
)

const releaseManifestName = "RELEASE_MANIFEST.json"

var releaseVersionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$`)

type moduleEdit struct {
	Module struct {
		Path string
	}
	Require []struct {
		Path    string
		Version string
	}
}

func TestReleaseManifestMatchesRepository(t *testing.T) {
	root := moduleRoot(t)
	manifest := readReleaseManifest(t, root)

	if manifest.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", manifest.SchemaVersion)
	}
	switch manifest.Phase {
	case "core-candidate", "nested-candidate", "released":
	default:
		t.Fatalf("unsupported release phase %q", manifest.Phase)
	}
	assertReleaseVersion(t, "previousCoreVersion", manifest.PreviousCoreVersion)
	assertReleaseVersion(t, "coreVersion", manifest.CoreVersion)
	if releaseMajor(manifest.CoreVersion) != "v2" {
		t.Fatalf("coreVersion = %q, want v2 release", manifest.CoreVersion)
	}
	if manifest.PreviousCoreVersion == manifest.CoreVersion {
		t.Fatal("previousCoreVersion and coreVersion must differ")
	}

	modulesByDir := make(map[string]releaseconfig.Module, len(manifest.Modules))
	modulesByPath := make(map[string]releaseconfig.Module, len(manifest.Modules))
	tags := make(map[string]struct{}, len(manifest.Modules))
	for _, module := range manifest.Modules {
		if module.Directory == "" || module.ModulePath == "" || module.Tag == "" {
			t.Fatalf("release module has empty identity field: %+v", module)
		}
		assertReleaseVersion(t, module.ModulePath, module.Version)
		if module.ReleaseOrder < 1 {
			t.Errorf("%s releaseOrder = %d, want positive value", module.ModulePath, module.ReleaseOrder)
		}
		if _, exists := modulesByDir[module.Directory]; exists {
			t.Errorf("duplicate release directory %q", module.Directory)
		}
		if _, exists := modulesByPath[module.ModulePath]; exists {
			t.Errorf("duplicate release module path %q", module.ModulePath)
		}
		if _, exists := tags[module.Tag]; exists {
			t.Errorf("duplicate release tag %q", module.Tag)
		}
		modulesByDir[module.Directory] = module
		modulesByPath[module.ModulePath] = module
		tags[module.Tag] = struct{}{}

		wantTag := module.Version
		if module.Directory != "." {
			wantTag = filepath.ToSlash(filepath.Join("v2", module.Directory, module.Version))
			if releaseMajor(module.Version) != "v0" {
				t.Errorf("nested module %s version = %q, want initial v0 release", module.ModulePath, module.Version)
			}
		}
		if module.Tag != wantTag {
			t.Errorf("%s tag = %q, want %q", module.ModulePath, module.Tag, wantTag)
		}

		moduleRoot := root
		if module.Directory != "." {
			moduleRoot = filepath.Join(root, filepath.FromSlash(module.Directory))
		}
		edit := readModuleEdit(t, moduleRoot)
		if edit.Module.Path != module.ModulePath {
			t.Errorf("%s go.mod module = %q, want %q", module.Directory, edit.Module.Path, module.ModulePath)
		}
		if module.Directory == "." {
			if module.Version != manifest.CoreVersion {
				t.Errorf("root module version = %q, want coreVersion %q", module.Version, manifest.CoreVersion)
			}
			if module.ReleaseOrder != 1 {
				t.Errorf("root releaseOrder = %d, want 1", module.ReleaseOrder)
			}
		}
		if module.DependsOnCore {
			wantCore := manifest.CoreVersion
			if manifest.Phase == "core-candidate" {
				wantCore = manifest.PreviousCoreVersion
			}
			if got := requiredVersion(edit, "github.com/dreamsxin/go-kit/v2"); got != wantCore {
				t.Errorf("%s requires core %q, want %q in phase %s", module.ModulePath, got, wantCore, manifest.Phase)
			}
			if module.ReleaseOrder <= 1 {
				t.Errorf("core-dependent module %s must release after core", module.ModulePath)
			}
		}
	}

	wantDirectories := discoverPublishableModuleDirectories(t, root)
	gotDirectories := make([]string, 0, len(modulesByDir))
	for directory := range modulesByDir {
		gotDirectories = append(gotDirectories, directory)
	}
	sort.Strings(gotDirectories)
	if strings.Join(gotDirectories, "\n") != strings.Join(wantDirectories, "\n") {
		t.Fatalf("release manifest module directories differ\n--- want\n%s\n--- got\n%s", strings.Join(wantDirectories, "\n"), strings.Join(gotDirectories, "\n"))
	}

	assertVersionText(t, filepath.Join(root, "Makefile"), "VERSION     ?= "+manifest.CoreVersion)
	assertVersionText(t, filepath.Join(root, "cmd", "microgen", "internal", "generator", "options.go"), `const defaultGoKitVersion = "`+manifest.CoreVersion+`"`)
	examples := readModuleEdit(t, filepath.Join(root, "examples"))
	if got := requiredVersion(examples, "github.com/dreamsxin/go-kit/v2"); got != manifest.CoreVersion {
		t.Errorf("examples require core %q, want %q", got, manifest.CoreVersion)
	}
	grpcModule, ok := modulesByPath["github.com/dreamsxin/go-kit/v2/integrations/grpc"]
	if !ok {
		t.Fatal("release manifest is missing integrations/grpc")
	}
	assertVersionText(t, filepath.Join(root, "cmd", "microgen", "templates", "go_mod.tmpl"), "github.com/dreamsxin/go-kit/v2/integrations/grpc "+grpcModule.Version)
	microgenModule, ok := modulesByPath["github.com/dreamsxin/go-kit/v2/cmd/microgen"]
	if !ok {
		t.Fatal("release manifest is missing cmd/microgen")
	}
	for _, readme := range []string{"README.md", "README_zh.md"} {
		path := filepath.Join(root, readme)
		assertVersionText(t, path, manifest.CoreVersion)
		assertVersionText(t, path, microgenModule.ModulePath+"@"+microgenModule.Version)
	}
	assertVersionText(t, filepath.Join(root, "CHANGELOG.md"), "## ["+strings.TrimPrefix(manifest.CoreVersion, "v")+"] - Release Candidate")

	fixtureOutput := commandOutput(t, root, "git", "ls-files", "--", "tools/testdata/*/go.mod")
	for _, relative := range strings.Fields(string(fixtureOutput)) {
		fixtureRoot := filepath.Dir(filepath.Join(root, filepath.FromSlash(relative)))
		if got := requiredVersion(readModuleEdit(t, fixtureRoot), "github.com/dreamsxin/go-kit/v2"); got != manifest.CoreVersion {
			t.Errorf("%s requires core %q, want %q", relative, got, manifest.CoreVersion)
		}
	}
}

func readReleaseManifest(t *testing.T, root string) releaseconfig.Manifest {
	t.Helper()
	manifest, err := releaseconfig.LoadManifest(filepath.Join(root, releaseManifestName))
	if err != nil {
		t.Fatalf("load %s: %v", releaseManifestName, err)
	}
	return manifest
}

func readModuleEdit(t *testing.T, root string) moduleEdit {
	t.Helper()
	output := commandOutput(t, root, "go", "mod", "edit", "-json")
	var edit moduleEdit
	if err := json.Unmarshal(output, &edit); err != nil {
		t.Fatalf("decode %s/go.mod: %v", root, err)
	}
	return edit
}

func requiredVersion(edit moduleEdit, modulePath string) string {
	for _, requirement := range edit.Require {
		if requirement.Path == modulePath {
			return requirement.Version
		}
	}
	return ""
}

func assertReleaseVersion(t *testing.T, name, version string) {
	t.Helper()
	if !releaseVersionPattern.MatchString(version) {
		t.Errorf("%s has invalid release version %q", name, version)
	}
}

func releaseMajor(version string) string {
	if index := strings.IndexByte(version, '.'); index >= 0 {
		return version[:index]
	}
	return version
}

func assertVersionText(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Errorf("%s does not contain release value %q", path, want)
	}
}

func discoverPublishableModuleDirectories(t *testing.T, root string) []string {
	t.Helper()
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "examples" || relative == "tools" {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
			directory := filepath.ToSlash(relative)
			if directory == "" {
				directory = "."
			}
			directories = append(directories, directory)
		}
		if relative == ".git" {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatalf("discover publishable modules: %v", err)
	}
	if len(directories) == 0 {
		t.Fatal("no publishable modules discovered")
	}
	sort.Strings(directories)
	return directories
}
