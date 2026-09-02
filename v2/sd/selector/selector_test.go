package selector_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

func instances(addresses ...string) []sd.Instance { return sd.Addresses(addresses...) }

// selectOne fails the test on error and reports the outcome, which is what
// every caller of Select must do with the callback it receives.
func selectOne(t *testing.T, sel selector.Selector, request any) sd.Instance {
	t.Helper()
	instance, done, err := sel.Select(context.Background(), request)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if done == nil {
		t.Fatal("Select returned a nil callback")
	}
	done(sd.Outcome{})
	return instance
}

func TestNew_SelectsThroughStrategy(t *testing.T) {
	sel := selector.New(selector.Static(instances("b:80", "a:80")...), selector.RoundRobin())

	// Static sorts by address, so round robin walks a stable sequence.
	want := []string{"a:80", "b:80", "a:80"}
	for i, expected := range want {
		if got := selectOne(t, sel, nil); got.Address != expected {
			t.Fatalf("select %d = %q, want %q", i+1, got.Address, expected)
		}
	}
}

func TestNew_PropagatesSourceError(t *testing.T) {
	failing := errors.New("registry down")
	source := selector.SourceFunc(func() ([]sd.Instance, error) { return nil, failing })

	_, _, err := selector.New(source, selector.RoundRobin()).Select(context.Background(), nil)
	if !errors.Is(err, failing) {
		t.Fatalf("Select error = %v, want %v", err, failing)
	}
}

func TestNew_EmptySnapshotReportsNoEndpoints(t *testing.T) {
	sel := selector.New(selector.Static(), selector.RoundRobin())

	_, _, err := sel.Select(context.Background(), nil)
	if !errors.Is(err, sd.ErrNoEndpoints) {
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

// The instance layer must forward the strategy's callback. Dropping it leaves a
// strategy that tracks calls believing every selection is still in flight.
func TestNew_ForwardsTheStrategyCallback(t *testing.T) {
	var reported []sd.Outcome
	tracking := strategyFunc(func(context.Context, any, []sd.Instance) (int, sd.Done, error) {
		return 0, func(outcome sd.Outcome) { reported = append(reported, outcome) }, nil
	})

	sel := selector.New(selector.Static(instances("a:80")...), tracking)
	_, done, err := sel.Select(context.Background(), nil)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	failure := errors.New("dial failed")
	done(sd.Outcome{Err: failure, Bytes: 7})
	// Callers that both defer the callback and report explicitly must not be
	// counted twice.
	done(sd.Outcome{})

	if len(reported) != 1 {
		t.Fatalf("strategy observed %d outcomes, want 1: %+v", len(reported), reported)
	}
	if !errors.Is(reported[0].Err, failure) || reported[0].Bytes != 7 {
		t.Fatalf("strategy observed %+v, want the caller's outcome", reported[0])
	}
}

// A strategy with no per-call state still has to yield a usable callback, or
// every caller needs to know which strategy it was handed.
func TestNew_ReturnsACallbackForStatelessStrategies(t *testing.T) {
	sel := selector.New(selector.Static(instances("a:80")...), selector.RoundRobin())

	_, done, err := sel.Select(context.Background(), nil)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if done == nil {
		t.Fatal("Select returned a nil callback for a stateless strategy")
	}
	done(sd.Outcome{})
}

// A request-aware strategy must survive binding, or a keyed strategy silently
// degrades to its random fallback.
func TestNewPassesRequestToStrategy(t *testing.T) {
	key := func(_ context.Context, request any) string { return request.(string) }
	sel := selector.New(selector.Static(instances("a:80", "b:80", "c:80")...), selector.ConsistentHash(key))

	first := selectOne(t, sel, "tenant-42")
	for i := 0; i < 20; i++ {
		if again := selectOne(t, sel, "tenant-42"); again.Address != first.Address {
			t.Fatalf("same key resolved to %q then %q", first.Address, again.Address)
		}
	}
}

func TestSelectPassesRequestToStrategy(t *testing.T) {
	set := instances("a:80", "b:80", "c:80")
	key := func(_ context.Context, request any) string { return request.(string) }
	keyed := selector.New(selector.Static(set...), selector.ConsistentHash(key))

	pinned, done, err := selector.Select(context.Background(), keyed, "tenant-7")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	done(sd.Outcome{})
	for i := 0; i < 20; i++ {
		got, done, err := selector.Select(context.Background(), keyed, "tenant-7")
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		done(sd.Outcome{})
		if got.Address != pinned.Address {
			t.Fatalf("Select did not use the keyed path: %q then %q", pinned.Address, got.Address)
		}
	}

	// A strategy without the request-aware path still works through Select.
	plain := selector.New(selector.Static(set...), selector.RoundRobin())
	if _, _, err := selector.Select(context.Background(), plain, "ignored"); err != nil {
		t.Fatalf("Select on a plain selector: %v", err)
	}
}

func TestSelectWithoutASelectorReportsNoEndpoints(t *testing.T) {
	_, _, err := selector.Select(context.Background(), nil, nil)
	if !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Select error = %v, want ErrNoEndpoints", err)
	}
}
