package tools_test

import (
	"strings"
	"testing"
)

// The API snapshot exists to make an exported-surface change a signed-off event.
// It must not make a comment edit one — that was the old behaviour, and it put
// every documentation fix behind a snapshot refresh.
func TestDeclarationsOnlyIgnoresProse(t *testing.T) {
	t.Parallel()
	before := `package apperror // import "github.com/dreamsxin/go-kit/v2/apperror"

Package apperror defines transport-neutral application errors.

TYPES

type Error struct {
	// Has unexported fields.
}
    Error is a classified application error with a stable code.

func Internal(code, message string) *Error
    Internal creates a KindInternal error (HTTP 500 / gRPC Internal).
`
	// Same declarations, rewritten prose: a typo fix, a clarified sentence, and
	// a reworded package summary.
	after := `package apperror // import "github.com/dreamsxin/go-kit/v2/apperror"

Package apperror defines transport-neutral application errors. Transports map
these classes onto protocol statuses.

TYPES

type Error struct {
	// Has unexported fields.
}
    Error is a classified application error carrying a stable, machine-readable
    code and an optional client-safe message.

func Internal(code, message string) *Error
    Internal creates a KindInternal error. The message stays opaque to clients:
    500 is where unclassified failures land.
`
	if got, want := decl(t, after), decl(t, before); got != want {
		t.Errorf("prose changed the snapshot input\n--- before\n%s--- after\n%s", want, got)
	}
}

func TestDeclarationsOnlyKeepsTheSurface(t *testing.T) {
	t.Parallel()
	base := `package endpoint // import "github.com/dreamsxin/go-kit/v2/endpoint"

FUNCTIONS

func Chain(outer Middleware, others ...Middleware) Middleware
    Chain composes middleware.
`
	cases := map[string]string{
		"added symbol": base + `
func Nop(ctx context.Context, request any) (any, error)
    Nop is a no-op endpoint.
`,
		"changed signature": strings.Replace(base,
			"func Chain(outer Middleware, others ...Middleware) Middleware",
			"func Chain(others ...Middleware) Middleware", 1),
		"removed symbol": `package endpoint // import "github.com/dreamsxin/go-kit/v2/endpoint"

FUNCTIONS
`,
		"moved section": strings.Replace(base, "FUNCTIONS", "TYPES", 1),
	}
	want := decl(t, base)
	for name, changed := range cases {
		if decl(t, changed) == want {
			t.Errorf("%s did not change the snapshot input", name)
		}
	}
}

// A struct body and a grouped const block are part of the surface, so their
// lines must survive the filter while the prose around them does not.
func TestDeclarationsOnlyKeepsBlockBodies(t *testing.T) {
	t.Parallel()
	got := decl(t, `package sd // import "github.com/dreamsxin/go-kit/v2/sd"

CONSTANTS

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
)
    Outcomes a Done callback may report.

TYPES

type Instance struct {
	Address  string
	Metadata map[string]string
}
    Instance is a discovered address.
`)
	for _, want := range []string{
		"const (",
		"\tOutcomeSuccess Outcome = \"success\"",
		")",
		"type Instance struct {",
		"\tAddress  string",
		"}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("filtered output lost %q\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"Outcomes a Done callback",
		"Instance is a discovered address",
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("filtered output kept prose %q\n%s", unwanted, got)
		}
	}
}

func decl(t *testing.T, doc string) string {
	t.Helper()
	return string(declarationsOnly([]byte(doc)))
}
