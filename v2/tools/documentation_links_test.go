package tools_test

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)

func TestDocumentationLinksResolve(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	anchors := make(map[string]map[string]struct{})
	var markdownFiles []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			relative, _ := filepath.Rel(root, path)
			if relative == filepath.Join("tools", "testdata") || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			markdownFiles = append(markdownFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk documentation: %v", err)
	}

	for _, path := range markdownFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := stripFencedCode(string(data))
		anchors[path] = markdownAnchors(content)
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(content, -1) {
			target := strings.TrimSpace(strings.Trim(match[1], "<>"))
			if fields := strings.Fields(target); len(fields) > 0 {
				target = fields[0]
			}
			if err := validateDocumentationLink(path, target, anchors); err != nil {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s: %v", filepath.ToSlash(relative), err)
			}
		}
	}
}

func validateDocumentationLink(sourcePath, target string, anchors map[string]map[string]struct{}) error {
	parsed, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("invalid link %q: %w", target, err)
	}
	if parsed.IsAbs() || parsed.Scheme != "" || strings.HasPrefix(target, "//") {
		return nil
	}
	decodedPath, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return fmt.Errorf("invalid escaped link %q", target)
	}
	resolved := sourcePath
	if decodedPath != "" {
		resolved = filepath.Clean(filepath.Join(filepath.Dir(sourcePath), filepath.FromSlash(decodedPath)))
		info, err := os.Stat(resolved)
		if err != nil {
			return fmt.Errorf("link %q does not resolve", target)
		}
		if info.IsDir() {
			resolved = filepath.Join(resolved, "README.md")
			if _, err := os.Stat(resolved); err != nil {
				return fmt.Errorf("directory link %q has no README.md", target)
			}
		}
	}
	if parsed.Fragment == "" || !strings.EqualFold(filepath.Ext(resolved), ".md") {
		return nil
	}
	if _, ok := anchors[resolved]; !ok {
		data, err := os.ReadFile(resolved)
		if err != nil {
			return fmt.Errorf("read anchor target %q: %w", target, err)
		}
		anchors[resolved] = markdownAnchors(stripFencedCode(string(data)))
	}
	fragment, err := url.PathUnescape(parsed.Fragment)
	if err != nil {
		return fmt.Errorf("invalid anchor in %q", target)
	}
	if _, ok := anchors[resolved][strings.ToLower(fragment)]; !ok {
		return fmt.Errorf("anchor %q does not resolve", target)
	}
	return nil
}

func stripFencedCode(content string) string {
	var output strings.Builder
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence {
			output.WriteString(line)
			output.WriteByte('\n')
		}
	}
	return output.String()
}

func markdownAnchors(content string) map[string]struct{} {
	anchors := make(map[string]struct{})
	duplicates := make(map[string]int)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if heading == "" {
			continue
		}
		slug := markdownSlug(heading)
		if count := duplicates[slug]; count > 0 {
			anchors[fmt.Sprintf("%s-%d", slug, count)] = struct{}{}
		} else {
			anchors[slug] = struct{}{}
		}
		duplicates[slug]++
	}
	return anchors
}

func markdownSlug(heading string) string {
	heading = strings.ToLower(strings.ReplaceAll(heading, "`", ""))
	var slug strings.Builder
	lastHyphen := false
	for _, char := range heading {
		switch {
		case unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_':
			slug.WriteRune(char)
			lastHyphen = false
		case unicode.IsSpace(char) || char == '-':
			if !lastHyphen && slug.Len() > 0 {
				slug.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(slug.String(), "-")
}
