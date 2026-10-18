package endpoint_test

import (
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// TestBuilder_Describe pins the label contract the startup log relies on: a
// labelled middleware reports its label in application order, an unlabelled one
// reports "?", and the shorthands report the name they were installed under.
func TestBuilder_Describe(t *testing.T) {
	b := endpoint.NewBuilder(endpoint.Nop).
		WithValidation().
		UseNamed("auth", func(next endpoint.Endpoint) endpoint.Endpoint { return next }).
		Use(func(next endpoint.Endpoint) endpoint.Endpoint { return next }).
		WithTimeout(5)

	got := b.Describe()
	want := []string{"validation", "auth", "?", "timeout"}
	if len(got) != len(want) {
		t.Fatalf("labels: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
