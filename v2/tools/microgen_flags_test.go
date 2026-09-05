package tools_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// generatorReferenceDocs are the documents that describe the microgen command
// line. Tutorials and the READMEs show a few invocations by way of example; these
// two claim to be the reference, so they are the ones held to the flag set.
var generatorReferenceDocs = []string{"MICROGEN.md", "MICROGEN_zh.md"}

// libraryProvidedFlags are supplied by the flag package rather than declared by
// microgen, so they appear in documentation without appearing in the flag set.
var libraryProvidedFlags = map[string]struct{}{"h": {}, "help": {}}

// TestMicrogenFlagsAreDocumented keeps the generator's command line and its
// reference documentation from drifting apart.
//
// The flag set is read from the program's own usage rather than from the source,
// so the test asks what a user would be told. Both directions matter and fail
// differently: a flag with no documentation is a feature nobody can find, and a
// documented flag that no longer exists is worse — a reader follows the
// documentation and the command rejects the argument.
func TestMicrogenFlagsAreDocumented(t *testing.T) {
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)

	usage, err := runQuiet(root, "go", "run", "./cmd/microgen", "--help")
	if err != nil {
		t.Fatalf("microgen --help: %v\n%s", err, usage)
	}
	defined := definedFlags(usage)
	// A usage format change would leave the parse empty and let this test pass
	// while checking nothing, which is the one way a gate fails silently.
	if len(defined) < 20 {
		t.Fatalf("parsed only %d flags from microgen usage, so the usage format changed:\n%s", len(defined), usage)
	}

	for _, name := range generatorReferenceDocs {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(data)

		undocumented, unknown := flagDrift(defined, text)
		if len(undocumented) > 0 {
			t.Errorf("%s does not document: %s\n\nA flag nobody documents is a flag nobody finds.",
				name, strings.Join(undocumented, " "))
		}
		if len(unknown) > 0 {
			t.Errorf("%s shows flags microgen does not accept: %s\n\nA reader following this would have the "+
				"command reject the argument.", name, strings.Join(unknown, " "))
		}
	}
}

// flagDrift compares a flag set with a document, reporting flags the document
// omits and flags it invokes that do not exist.
func flagDrift(defined []string, text string) (undocumented, unknown []string) {
	for _, flag := range defined {
		if !mentionsFlag(text, flag) {
			undocumented = append(undocumented, "-"+flag)
		}
	}
	for _, flag := range invokedFlags(text) {
		if _, ok := libraryProvidedFlags[flag]; ok {
			continue
		}
		if !containsFlag(defined, flag) {
			unknown = append(unknown, "-"+flag)
		}
	}
	sort.Strings(undocumented)
	sort.Strings(unknown)
	return undocumented, unknown
}

// TestMicrogenFlagDriftIsReported proves both directions fail, on inputs written
// here: the real documentation matches the real flag set, so it cannot show a
// failure.
func TestMicrogenFlagDriftIsReported(t *testing.T) {
	t.Parallel()
	defined := []string{"idl", "out"}

	if undocumented, unknown := flagDrift(defined, "run `microgen -idl api.go -out .`"); len(undocumented) != 0 || len(unknown) != 0 {
		t.Fatalf("a matching document reported undocumented=%v unknown=%v", undocumented, unknown)
	}

	undocumented, _ := flagDrift(defined, "run `microgen -idl api.go`")
	if len(undocumented) != 1 || undocumented[0] != "-out" {
		t.Fatalf("undocumented = %v, want -out", undocumented)
	}

	_, unknown := flagDrift(defined, "run `microgen -idl api.go -out . -gone`")
	if len(unknown) != 1 || unknown[0] != "-gone" {
		t.Fatalf("unknown = %v, want -gone", unknown)
	}

	// A library flag is documented without being declared, and must not be
	// reported as one the command rejects.
	if _, unknown := flagDrift(defined, "run `microgen -idl a -out . -h`"); len(unknown) != 0 {
		t.Fatalf("unknown = %v, want the library -h flag to be tolerated", unknown)
	}
}

func TestMicrogenFlagExtraction(t *testing.T) {
	t.Parallel()
	const usage = `Usage of microgen:
  -check
    	Scan an existing project
  -config-mode string
    	Generated config mode: file, hybrid, remote
  -out string
    	Output directory (default ".")
`
	got := definedFlags(usage)
	want := []string{"check", "config-mode", "out"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("definedFlags = %v, want %v", got, want)
	}

	const doc = "Run `microgen -idl api.go -out ./gen --openapi` to generate.\n" +
		"Unrelated: `go test -race ./...` on a line that also says microgen.\n"
	if flags := invokedFlags(doc); strings.Join(flags, ",") != "idl,openapi,out,race" {
		t.Fatalf("invokedFlags = %v, want idl, openapi, out and the race artifact", flags)
	}

	// A shorter flag must not be considered documented by a longer one that
	// happens to start with it.
	if mentionsFlag("only -config-mode here", "config") {
		t.Fatal("-config-mode was accepted as documentation for -config")
	}
	if mentionsFlag("only -dbname here", "db") {
		t.Fatal("-dbname was accepted as documentation for -db")
	}
	if !mentionsFlag("the -db flag, and more", "db") {
		t.Fatal("-db was not recognised where it is documented")
	}
}

var usageFlagPattern = regexp.MustCompile(`(?m)^\s{2}-([a-z][a-z0-9-]*)`)

// definedFlags reads the flag names out of a Go flag package usage listing.
func definedFlags(usage string) []string {
	var flags []string
	for _, match := range usageFlagPattern.FindAllStringSubmatch(usage, -1) {
		flags = append(flags, match[1])
	}
	sort.Strings(flags)
	return flags
}

var invocationFlagPattern = regexp.MustCompile(`[\s"'](--?)([a-z][a-z0-9-]*)`)

// invokedFlags reads flag names from every documentation line that mentions the
// generator. Command lines are where a stale flag survives: a prose mention is
// usually caught by a reader, an example is copied and pasted.
func invokedFlags(text string) []string {
	seen := map[string]struct{}{}
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "microgen") {
			continue
		}
		for _, match := range invocationFlagPattern.FindAllStringSubmatch(line, -1) {
			seen[match[2]] = struct{}{}
		}
	}
	flags := make([]string, 0, len(seen))
	for flag := range seen {
		flags = append(flags, flag)
	}
	sort.Strings(flags)
	return flags
}

// mentionsFlag reports whether text names exactly this flag. The following
// character has to end the name, or -config would be satisfied by -config-mode
// and -db by -dbname.
func mentionsFlag(text, flag string) bool {
	needle := "-" + flag
	for offset := 0; ; {
		index := strings.Index(text[offset:], needle)
		if index < 0 {
			return false
		}
		end := offset + index + len(needle)
		if end >= len(text) || !isFlagNameByte(text[end]) {
			return true
		}
		offset = end
	}
}

func isFlagNameByte(c byte) bool {
	return c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func containsFlag(flags []string, flag string) bool {
	index := sort.SearchStrings(flags, flag)
	return index < len(flags) && flags[index] == flag
}
