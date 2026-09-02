package selector_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

func instances(addresses ...string) []sd.Instance { return sd.Addresses(addresses...) }

func TestNew_SelectsThroughStrategy(t *testing.T) {
	sel := selector.New(selector.Static(instances("b:80", "a:80")...), selector.RoundRobin())

	// Static sorts by address, so round robin walks a stable sequence.
	want := []string{"a:80", "b:80", "a:80"}
	for i, expected := range want {
		got, err := sel.Select(context.Background(), nil)
		if err != nil {
			t.Fatalf("Select %d: %v", i+1, err)
		}
		if got.Address != expected {
			t.Fatalf("select %d = %q, want %q", i+1, got.Address, expected)
		}
	}
}

func TestNew_PropagatesSourceError(t *testing.T) {
	failing := errors.New("registry down")
	source := selector.SourceFunc(func() ([]sd.Instance, error) { return nil, failing })

	if _, err := selector.New(source, selector.RoundRobin()).Select(context.Background(), nil); !errors.Is(err, failing) {
		t.Fatalf("Select error = %v, want %v", err, failing)
	}
}

func TestNew_EmptySnapshotReportsNoEndpoints(t *testing.T) {
	sel := selector.New(selector.Static(), selector.RoundRobin())

	if _, err := sel.Select(context.Background(), nil); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Select error = %v, want ErrNoEndpoints", err)
	}
}

func TestNew_NilArgumentsPanic(t *testing.T) {
	for name, build := range map[string]func(){
		"nil source":   func() { selector.New(nil, selector.RoundRobin()) },
		"nil strategy": func() { selector.New(selector.Static(), nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected a panic")
				}
			}()
			build()
		})
	}
}

// A request-aware strategy must survive binding, or a keyed strategy silently
// degrades to its random fallback.
func TestNewPassesRequestToStrategy(t *testing.T) {
	key := func(_ context.Context, request any) string { return request.(string) }
	sel := selector.New(selector.Static(instances("a:80", "b:80", "c:80")...), selector.ConsistentHash(key))

	first, err := sel.Select(context.Background(), "tenant-42")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	for i := 0; i < 20; i++ {
		again, err := sel.Select(context.Background(), "tenant-42")
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if again.Address != first.Address {
			t.Fatalf("same key resolved to %q then %q", first.Address, again.Address)
		}
	}
}

func TestSelectPassesRequestToStrategy(t *testing.T) {
	set := instances("a:80", "b:80", "c:80")
	key := func(_ context.Context, request any) string { return request.(string) }
	keyed := selector.New(selector.Static(set...), selector.ConsistentHash(key))

	pinned, err := selector.Select(context.Background(), keyed, "tenant-7")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	for i := 0; i < 20; i++ {
		got, err := selector.Select(context.Background(), keyed, "tenant-7")
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if got.Address != pinned.Address {
			t.Fatalf("Select did not use the keyed path: %q then %q", pinned.Address, got.Address)
		}
	}

	// A strategy without the request-aware path still works through Select.
	plain := selector.New(selector.Static(set...), selector.RoundRobin())
	if _, err := selector.Select(context.Background(), plain, "ignored"); err != nil {
		t.Fatalf("Select on a plain selector: %v", err)
	}
}
