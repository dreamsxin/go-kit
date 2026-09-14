package tools_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

type releaseStatusDocument struct {
	path, heading, released, candidate string
}

// A release statement must name its version and phase together in the status
// section. Keywords in instructions or code examples do not prove that claim.
var releaseStatusDocuments = []releaseStatusDocument{
	{"../README.md", "Current Release", "`%s` is the current architecture release", "`%s` is the patch candidate on `main`"},
	{"../README_zh.md", "当前版本", "`%s` 是当前架构版本", "`%s` 是 `main` 上的补丁候选版本"},
	{"README.md", "Status", "`%s` is the current release.", "`%s` is the patch candidate on `main`"},
	{"README_zh.md", "当前状态", "`%s` 是当前发布版本。", "`%s` 是 `main` 上的补丁候选版本"},
	{"ARCHITECTURE.md", "Stability", "`%s` is the current released contract.", "`%s` is the patch candidate on `main`"},
	{"ARCHITECTURE_zh.md", "稳定性", "`%s` 是当前已发布契约。", "`%s` 是 `main` 上的补丁候选版本"},
	{"internal/docs/RELEASE.md", "Current Position", "%s is the current published release of the module:", "%s is the patch candidate being prepared from `main`"},
	{"internal/docs/RELEASE_zh.md", "当前状态", "%s 是当前已发布的模块版本：", "%s 是本次从 `main` 准备的补丁候选版本"},
}

func checkReleaseStatus(text string, doc releaseStatusDocument, phase, version string) error {
	template := doc.released
	switch phase {
	case "candidate":
		template = doc.candidate
	case "released":
	default:
		return fmt.Errorf("unknown release phase %q", phase)
	}
	want := fmt.Sprintf(template, version)
	pattern := regexp.QuoteMeta(want)
	if phase == "candidate" {
		pattern = strings.ReplaceAll(pattern, "patch ", "(?:patch )?")
		pattern = strings.ReplaceAll(pattern, "补丁", "(?:补丁)?")
	}
	var section []string
	inside, fenced, found := false, false, false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if line == "## "+doc.heading {
			if found {
				return fmt.Errorf("duplicate status heading %q", doc.heading)
			}
			inside, found = true, true
			continue
		}
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ") {
			inside = false
		}
		if inside {
			section = append(section, line)
		}
	}
	if !found {
		return fmt.Errorf("missing status heading %q", doc.heading)
	}
	actual := strings.Join(strings.Fields(strings.Join(section, "\n")), " ")
	if !regexp.MustCompile(pattern).MatchString(actual) {
		return fmt.Errorf("phase %s requires %q in %q; got %q", phase, want, doc.heading, actual)
	}
	return nil
}

func TestReleaseStatusRejectsUnrelatedPhaseText(t *testing.T) {
	const version = "v2.99.0"
	for _, doc := range releaseStatusDocuments {
		for _, phase := range []string{"candidate", "released"} {
			t.Run(doc.path+"/"+phase, func(t *testing.T) {
				correct, wrong := doc.released, doc.candidate
				if phase == "candidate" {
					correct, wrong = wrong, correct
				}
				statement := fmt.Sprintf(correct, version)
				heading := "## " + doc.heading + "\n\n"
				for _, tc := range []struct {
					name, text string
					valid      bool
				}{
					{"valid", heading + statement, true},
					{"minor candidate", heading + strings.ReplaceAll(strings.ReplaceAll(statement, "patch ", ""), "补丁", ""), true},
					{"wrapped", heading + strings.ReplaceAll(statement, " ", "\n"), true},
					{"wrong phase", heading + fmt.Sprintf(wrong, version) + "\n\n## Workflow\ncandidate 候选", false},
					{"wrong version", heading + fmt.Sprintf(correct, "v2.98.0") + "\n\n## History\n" + version, false},
					{"outside section", heading + "No release status.\n\n## Workflow\n" + statement, false},
					{"code example", heading + "```text\n" + statement + "\n```", false},
					{"missing section", "## Workflow\n" + statement, false},
				} {
					t.Run(tc.name, func(t *testing.T) {
						err := checkReleaseStatus(tc.text, doc, phase, version)
						if (err == nil) != tc.valid {
							t.Fatalf("valid=%v, error=%v", tc.valid, err)
						}
					})
				}
			})
		}
	}
}
