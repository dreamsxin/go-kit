package main

import (
	"context"
	"strings"
	"testing"
)

// TestOneRequestIsOneTree pins the property the example demonstrates: a
// request through the full stack exports exactly one tree — a SERVER span
// named after the route, the endpoint span beneath it, and a CLIENT span on
// the outbound call, all sharing one trace.
func TestOneRequestIsOneTree(t *testing.T) {
	var out strings.Builder
	if err := run(context.Background(), &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	tree := out.String()
	for _, want := range []string{
		"trace ",
		"└─ server GET /aggregate",
		"└─ internal GET /aggregate",
		"└─ client GET",
	} {
		if !strings.Contains(tree, want) {
			t.Errorf("tree does not contain %q:\n%s", want, tree)
		}
	}

	// The three spans share one trace: the tree header carries it and no
	// second trace line appears.
	if got := strings.Count(tree, "trace "); got != 1 {
		t.Errorf("trace headers = %d, want exactly one:\n%s", got, tree)
	}

	// The response actually reached the end of the chain.
	if !strings.Contains(out.String(), "upstream says hello") {
		t.Errorf("response body missing from output:\n%s", out.String())
	}
}
