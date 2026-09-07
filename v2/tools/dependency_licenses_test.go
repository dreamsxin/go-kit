package tools_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// licenseDocs list the dependency licenses in each language.
var licenseDocs = []string{
	filepath.Join("docs", "licenses.md"),
	filepath.Join("docs", "licenses_zh.md"),
}

// documentedDependency reads one entry of those lists:
//
//   - `google.golang.org/grpc` — Apache-2.0 — `integrations/grpc`
//
// The dash is an em dash in English and a double em dash in Chinese, so the
// pattern accepts either.
var documentedDependency = regexp.MustCompile("(?m)^- `([^`]+)` +(?:—|——) +([A-Za-z0-9.+-]+) ")

// TestDependencyLicensesAreDocumented keeps the licensing document and the module
// graph in step.
//
// The published module is one module, so every direct requirement is recorded in
// a consumer's go.mod whether or not they import the package that needs it. Which
// licenses those are is a question a consumer's legal review asks before adopting
// anything, and the answer was previously nowhere: it had to be reconstructed from
// go.mod plus nineteen module caches. The document now answers it, and this gate
// is what stops the answer from going stale — a new dependency, a removed one, or
// one that relicenses fails here.
//
// The license is classified from each module's own license file rather than from a
// table someone typed, because the point is to catch the case where those two
// disagree.
func TestDependencyLicensesAreDocumented(t *testing.T) {
	t.Parallel()
	root := goKitRoot(t)

	actual := map[string]string{}
	for _, requirement := range readModuleEdit(t, root).Require {
		if requirement.Indirect {
			continue
		}
		actual[requirement.Path] = moduleLicense(t, root, requirement.Path)
	}
	if len(actual) < 10 {
		// The module requires far more than ten direct dependencies, so a short
		// read means go.mod was not parsed rather than that the list shrank.
		t.Fatalf("found only %d direct requirements, so go.mod was not read", len(actual))
	}

	for _, doc := range licenseDocs {
		data, err := os.ReadFile(filepath.Join(root, doc))
		if err != nil {
			t.Fatalf("read %s: %v", doc, err)
		}
		documented := map[string]string{}
		for _, match := range documentedDependency.FindAllStringSubmatch(string(data), -1) {
			documented[match[1]] = match[2]
		}

		for _, path := range sortedModulePaths(actual) {
			switch license, ok := documented[path]; {
			case !ok:
				t.Errorf("%s does not list %s (%s)\n\nA dependency a consumer inherits and cannot look "+
					"up is one their legal review has to reconstruct.", doc, path, actual[path])
			case license != actual[path]:
				t.Errorf("%s lists %s as %s, but its license file reads %s",
					doc, path, license, actual[path])
			}
		}
		for _, path := range sortedModulePaths(documented) {
			if _, ok := actual[path]; !ok {
				t.Errorf("%s lists %s, which is no longer a direct requirement", doc, path)
			}
		}
	}
}

// TestProjectLicenseIsOneText proves the two copies of the project license are the
// same text. The module lives in the v2 subdirectory and the repository root
// carries its own copy, so a consumer sees one or the other depending on how they
// vendored it; two copies that drift would be two different offers.
func TestProjectLicenseIsOneText(t *testing.T) {
	t.Parallel()
	root := goKitRoot(t)
	repoRoot := filepath.Dir(root)

	module := licenseText(t, filepath.Join(root, "LICENSE.txt"))
	repository := licenseText(t, filepath.Join(repoRoot, "LICENSE.txt"))
	if module != repository {
		t.Fatalf("v2/LICENSE.txt and the repository's LICENSE.txt differ")
	}
	if got := classifyLicense(module); got != "MIT" {
		t.Errorf("the project license reads as %s, and the READMEs and docs say MIT", got)
	}
	if !strings.Contains(module, "Copyright (c)") {
		t.Error("the license names no copyright holder, which MIT requires it to carry")
	}
}

// TestGeneratedOutputCarriesNoLicenseNotice pins the grant that docs/licenses.md
// makes: what the generator writes is the user's, with nothing flowing back.
//
// A grant like that is worth only as much as the output agrees with it. A license
// header added to a template, or a LICENSE file written into a generated project,
// would contradict the document in the one place a user actually looks — their own
// repository — and neither is the kind of change anyone would think to re-read a
// licensing document over.
func TestGeneratedOutputCarriesNoLicenseNotice(t *testing.T) {
	t.Parallel()
	root := goKitRoot(t)

	layout, err := os.ReadFile(filepath.Join(root, "tools", "testdata", "generated_layout.txt"))
	if err != nil {
		t.Fatalf("read the reviewed generated layout: %v", err)
	}
	for _, entry := range reviewedEntries(layout) {
		base := strings.ToUpper(filepath.Base(entry))
		if strings.HasPrefix(base, "LICEN") || strings.HasPrefix(base, "COPYING") {
			t.Errorf("a generated project contains %s, which decides a licensing question that is the "+
				"user's to decide", entry)
		}
	}

	templates := filepath.Join(root, "cmd", "microgen", "templates")
	entries, err := os.ReadDir(templates)
	if err != nil {
		t.Fatalf("read %s: %v", templates, err)
	}
	var readme string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(templates, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		text := string(data)
		if entry.Name() == "readme.tmpl" {
			readme = text
		}
		for _, notice := range []string{"SPDX-License-Identifier", "Copyright (c)"} {
			if strings.Contains(text, notice) {
				t.Errorf("%s emits %q into generated code, which asks something of the user's project",
					entry.Name(), notice)
			}
		}
	}

	// The grant is only useful if it reaches the person holding the output.
	if !strings.Contains(readme, "## License") ||
		!strings.Contains(readme, "github.com/dreamsxin/go-kit/v2") {
		t.Error("the generated README does not say who owns the generated code and what the framework's " +
			"license is")
	}
}

// moduleLicense classifies the license a module ships.
func moduleLicense(t *testing.T, root, modulePath string) string {
	t.Helper()
	dir := strings.TrimSpace(string(normalizeCommandOutput(
		commandOutput(t, root, "go", "list", "-m", "-f", "{{.Dir}}", modulePath))))
	if dir == "" {
		t.Fatalf("%s is not in the module cache; run go mod download", modulePath)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		upper := strings.ToUpper(entry.Name())
		if !strings.HasPrefix(upper, "LICEN") && !strings.HasPrefix(upper, "COPYING") {
			continue
		}
		if license := classifyLicense(licenseText(t, filepath.Join(dir, entry.Name()))); license != "" {
			return license
		}
	}
	t.Fatalf("no license this gate recognises in %s (%s); read it and extend classifyLicense",
		modulePath, dir)
	return ""
}

// classifyLicense names a license from its own text. It reports an empty string
// for anything it does not recognise, so an unknown license is read by a person
// rather than guessed at here.
func classifyLicense(text string) string {
	switch {
	case strings.Contains(text, "Mozilla Public License"):
		return "MPL-2.0"
	case strings.Contains(text, "Apache License") && strings.Contains(text, "Version 2.0"):
		return "Apache-2.0"
	case strings.Contains(text, "Permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(text, "Redistribution and use in source and binary forms"):
		if strings.Contains(text, "Neither the name") || strings.Contains(text, "nor the names") {
			return "BSD-3-Clause"
		}
		return "BSD-2-Clause"
	}
	return ""
}

// licenseText reads a license with its line endings and spacing normalised, so a
// checkout that rewrote CRLF does not read as a different text.
func licenseText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Join(strings.Fields(string(data)), " ")
}

func sortedModulePaths(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestLicenseClassifierNamesEachFamily proves the classifier tells the license
// families apart on text written here. Every dependency in the tree happens to be
// one of five, so the real graph cannot show it refusing an unknown one.
func TestLicenseClassifierNamesEachFamily(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, text, want string }{
		{"MIT", "MIT License Copyright (c) 2024 X Permission is hereby granted, free of charge, to any person", "MIT"},
		{"Apache", "Apache License Version 2.0, January 2004 TERMS AND CONDITIONS", "Apache-2.0"},
		{"MPL", "Mozilla Public License Version 2.0 1. Definitions", "MPL-2.0"},
		{
			"BSD-3-Clause",
			"Copyright (c) 2018 The Go Authors. Redistribution and use in source and binary forms, with or " +
				"without modification, are permitted. Neither the name of Google Inc. nor the names of its " +
				"contributors may be used to endorse",
			"BSD-3-Clause",
		},
		{
			"BSD-2-Clause",
			"Copyright (c) 2024 X Redistribution and use in source and binary forms, with or without " +
				"modification, are permitted provided that the following conditions are met",
			"BSD-2-Clause",
		},
		{"unknown", "This software is released under the Do What You Want license.", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyLicense(tc.text); got != tc.want {
				t.Fatalf("classifyLicense = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDocumentedDependencyPatternReadsBothLanguages proves the entry pattern reads
// the two list styles, since the Chinese document uses a double em dash.
func TestDocumentedDependencyPatternReadsBothLanguages(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		"- `google.golang.org/grpc` — Apache-2.0 — `integrations/grpc`",
		"- `go.uber.org/zap` —— MIT —— `integrations/zap`",
	} {
		match := documentedDependency.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("no entry read from %q", line)
		}
		if !strings.Contains(line, match[1]) || classifyLicenseName(match[2]) == "" {
			t.Fatalf("read %q as module %q license %q", line, match[1], match[2])
		}
	}
	if documentedDependency.MatchString("- `github.com/example/x` is worth a look") {
		t.Fatal("a prose bullet was read as a dependency entry")
	}
}

// classifyLicenseName reports whether a documented license name is one this gate
// can produce, so a typo in the document is a failure rather than a mismatch
// nobody can explain.
func classifyLicenseName(name string) string {
	switch name {
	case "MIT", "Apache-2.0", "MPL-2.0", "BSD-2-Clause", "BSD-3-Clause":
		return name
	}
	return ""
}
