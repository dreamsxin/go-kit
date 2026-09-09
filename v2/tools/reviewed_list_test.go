package tools_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// reviewedList is a sorted set of names kept under review in testdata: the
// package paths the module publishes, the files a generator writes, anything
// where the question is which entries exist rather than what they contain.
//
// A digest would answer "something changed" and leave the reader to work out
// what. These lists are read by a person deciding whether a change may ship, so
// the stored form is the list itself and the failure names the entries that
// appeared and disappeared. The public API snapshot was moved to the same
// principle for the same reason; what remains behind a digest is the generated
// contract set, where the artefact is large enough that its own diff is the place
// to read it.
type reviewedList struct {
	// subject completes the sentence "the <subject> changed".
	subject string
	// file is the testdata file holding the reviewed copy, relative to testdata.
	file string
	// gained and lost label the two directions. A reader needs to know which one
	// is the dangerous one, and it is not the same for every list.
	gained string
	lost   string
	// consequence says what breaks when an entry disappears, so the failure
	// explains the stakes instead of only the diff.
	consequence string
	// updateFlag names the go test flag that refreshes the file, for the message
	// shown when it cannot be read.
	updateFlag string
}

// assert compares got with the reviewed copy, refreshing it first when update is
// set. got does not need to be sorted.
func (list reviewedList) assert(t *testing.T, got []string, update bool) {
	t.Helper()
	entries := append([]string(nil), got...)
	sort.Strings(entries)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	path := filepath.Join(cwd, "testdata", list.file)
	if update {
		if err := os.WriteFile(path, []byte(strings.Join(entries, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("update %s: %v", list.file, err)
		}
	}
	reviewed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (rerun with -args %s)", list.file, err, list.updateFlag)
	}

	added, removed := listDifference(reviewedEntries(reviewed), entries)
	if len(added) == 0 && len(removed) == 0 {
		return
	}
	var report strings.Builder
	for _, entry := range removed {
		report.WriteString("\n  " + list.lost + ": " + entry)
	}
	for _, entry := range added {
		report.WriteString("\n  " + list.gained + ": " + entry)
	}
	t.Fatalf("the %s changed:%s\n\n%s Refresh with: make update-snapshots",
		list.subject, report.String(), list.consequence)
}

// reviewedEntries reads a stored list, tolerating the line endings a Windows
// checkout would otherwise introduce.
func reviewedEntries(listing []byte) []string {
	var entries []string
	for _, line := range strings.Fields(strings.ReplaceAll(string(listing), "\r\n", "\n")) {
		if line != "" {
			entries = append(entries, line)
		}
	}
	sort.Strings(entries)
	return entries
}

// listDifference reports what current gained and lost relative to previous.
func listDifference(previous, current []string) (added, removed []string) {
	have := make(map[string]struct{}, len(current))
	for _, entry := range current {
		have[entry] = struct{}{}
	}
	had := make(map[string]struct{}, len(previous))
	for _, entry := range previous {
		had[entry] = struct{}{}
	}
	for _, entry := range previous {
		if _, ok := have[entry]; !ok {
			removed = append(removed, entry)
		}
	}
	for _, entry := range current {
		if _, ok := had[entry]; !ok {
			added = append(added, entry)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

// TestListDifferenceReportsBothDirections proves the comparison names what moved,
// on inputs written here: a real reviewed list matches itself, so it cannot show
// a failure.
func TestListDifferenceReportsBothDirections(t *testing.T) {
	t.Parallel()
	previous := []string{"example.com/a", "example.com/b"}
	current := []string{"example.com/a", "example.com/c"}

	added, removed := listDifference(previous, current)
	if len(removed) != 1 || removed[0] != "example.com/b" {
		t.Fatalf("removed = %v, want example.com/b", removed)
	}
	if len(added) != 1 || added[0] != "example.com/c" {
		t.Fatalf("added = %v, want example.com/c", added)
	}

	if added, removed := listDifference(previous, previous); len(added) != 0 || len(removed) != 0 {
		t.Fatalf("an unchanged list reported added=%v removed=%v", added, removed)
	}
}
