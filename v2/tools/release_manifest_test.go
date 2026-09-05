package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

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

	// Schema 2 describes one module. Schema 1 carried a module list, a release
	// order, and a per-module core requirement.
	if manifest.SchemaVersion != 2 {
		t.Fatalf("schemaVersion = %d, want 2", manifest.SchemaVersion)
	}
	switch manifest.Phase {
	case "candidate":
		if manifest.ReleaseDate != "" {
			t.Fatalf("releaseDate = %q in phase %q, want empty", manifest.ReleaseDate, manifest.Phase)
		}
	case "released":
		if _, err := time.Parse(time.DateOnly, manifest.ReleaseDate); err != nil {
			t.Fatalf("releaseDate = %q, want YYYY-MM-DD: %v", manifest.ReleaseDate, err)
		}
	default:
		t.Fatalf("unsupported release phase %q", manifest.Phase)
	}
	assertReleaseVersion(t, "coreVersion", manifest.CoreVersion)
	if releaseMajor(manifest.CoreVersion) != "v2" {
		t.Fatalf("coreVersion = %q, want v2 release", manifest.CoreVersion)
	}
	// The module lives in the v2 major-version subdirectory, so its tag is the
	// plain version rather than a directory-prefixed one.
	if manifest.Tag != manifest.CoreVersion {
		t.Fatalf("tag = %q, want %q", manifest.Tag, manifest.CoreVersion)
	}

	edit := readModuleEdit(t, root)
	if want := "github.com/dreamsxin/go-kit/v2"; edit.Module.Path != want {
		t.Fatalf("root go.mod module = %q, want %q", edit.Module.Path, want)
	}

	assertVersionText(t, filepath.Join(root, "Makefile"), "VERSION     ?= "+manifest.CoreVersion)
	assertVersionText(t, filepath.Join(root, "cmd", "microgen", "internal", "generator", "options.go"),
		`const defaultGoKitVersion = "`+manifest.CoreVersion+`"`)
	examples := readModuleEdit(t, filepath.Join(root, "examples"))
	if got := requiredVersion(examples, "github.com/dreamsxin/go-kit/v2"); got != manifest.CoreVersion {
		t.Errorf("examples require core %q, want %q", got, manifest.CoreVersion)
	}
	for _, readme := range []string{"README.md", "README_zh.md"} {
		path := filepath.Join(root, readme)
		assertVersionText(t, path, manifest.CoreVersion)
		// Documentation installs the generator with @latest so it never drifts
		// between releases.
		assertVersionText(t, path, "github.com/dreamsxin/go-kit/v2/cmd/microgen@latest")
	}

	changelogVersion := strings.TrimPrefix(manifest.CoreVersion, "v")
	changelogStatus := "Release Candidate"
	if manifest.Phase == "released" {
		changelogStatus = manifest.ReleaseDate
	}
	assertVersionText(t, filepath.Join(root, "CHANGELOG.md"), "## ["+changelogVersion+"] - "+changelogStatus)

}

// TestOnlyOneModuleIsPublishable keeps the single-module layout from eroding.
//
// A new go.mod anywhere under v2 that is not one of the repository-only modules
// would silently reintroduce multi-module releases: a second thing to version,
// tag, and keep in step with the core.
func TestOnlyOneModuleIsPublishable(t *testing.T) {
	root := moduleRoot(t)
	allowed := map[string]struct{}{
		"go.mod":                     {},
		"examples/go.mod":            {},
		"tools/go.mod":               {},
		"tools/contractcheck/go.mod": {},
	}

	var unexpected []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			// Generator fixtures are standalone projects, not part of the release.
			if relative == ".git" || relative == "tools/testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "go.mod" {
			return nil
		}
		if _, ok := allowed[relative]; !ok {
			unexpected = append(unexpected, relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk module definitions: %v", err)
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		t.Fatalf("module definitions outside the single published module:\n%s", strings.Join(unexpected, "\n"))
	}
}

// TestNestedModuleTagsAreGone keeps the single-module tag scheme intact.
//
// A release is one plain vX.Y.Z tag, so those accumulate and are not checked
// here. A directory-prefixed tag would mean a nested module is being versioned
// separately, which this layout does not do.
func TestNestedModuleTagsAreGone(t *testing.T) {
	root := moduleRoot(t)
	output := commandOutput(t, root, "git", "tag", "--list", "v2/*")
	if nested := strings.Fields(string(output)); len(nested) > 0 {
		t.Fatalf("nested module tags must be absent:\n%s", strings.Join(nested, "\n"))
	}
}

// TestManifestPhaseMatchesTagState pins the one direction that a local checkout
// can answer: a candidate must not already own its tag, or the next release would
// be cut against a version that is already published and immutable.
//
// The released phase is deliberately not checked here. Recording the release is a
// commit, that commit runs this suite, and requiring the tag would make the record
// commit and the tag push require each other — see tagRequirement in
// ./releasecheck. That a released manifest was actually published is checked by
// `releasecheck -check-published`, which resolves the version through the public
// proxy rather than trusting a local ref.
func TestManifestPhaseMatchesTagState(t *testing.T) {
	root := moduleRoot(t)
	manifest := readReleaseManifest(t, root)
	output := commandOutput(t, root, "git", "tag", "--list", manifest.Tag)
	tagged := len(strings.Fields(string(output))) > 0

	if manifest.Phase == "candidate" && tagged {
		t.Fatalf("tag %s already exists while the manifest is a candidate", manifest.Tag)
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
