package selector_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

func TestFiltered_MapsTheChoiceBackOntoTheCallerSnapshot(t *testing.T) {
	set := instances("a:80", "b:80", "c:80")
	// Round robin walks the narrowed slice, so without index remapping the
	// caller would be handed a:80 and b:80 instead of the surviving instance.
	strategy := selector.Filtered(selector.RoundRobin(), sd.Keep(func(instance sd.Instance) bool {
		return instance.Address == "c:80"
	}))

	for range 3 {
		index, _, err := strategy.Pick(context.Background(), nil, set)
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		if set[index].Address != "c:80" {
			t.Fatalf("picked %q at index %d, want c:80", set[index].Address, index)
		}
	}
}

func TestFiltered_AppliesFiltersInOrder(t *testing.T) {
	set := []sd.Instance{
		labelled("eu-a:80", map[string]any{"zone": "eu"}),
		labelled("eu-b:80", map[string]any{"zone": "eu"}),
		labelled("us-a:80", map[string]any{"zone": "us"}),
	}
	var sawAfterZone int
	count := sd.InstanceFilter(func(_ context.Context, candidates []sd.Instance) []sd.Instance {
		sawAfterZone = len(candidates)
		return candidates
	})

	strategy := selector.Filtered(selector.RoundRobin(), sd.Keep(sd.MetadataEquals("zone", "eu")), count)
	if _, _, err := strategy.Pick(context.Background(), nil, set); err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if sawAfterZone != 2 {
		t.Fatalf("the second filter saw %d candidates, want the 2 the first one kept", sawAfterZone)
	}
}

func TestFiltered_EmptyResultReportsNoEndpoints(t *testing.T) {
	strategy := selector.Filtered(selector.RoundRobin(), sd.Keep(func(sd.Instance) bool { return false }))
	if _, _, err := strategy.Pick(context.Background(), nil, instances("a:80")); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("error = %v, want ErrNoEndpoints", err)
	}
}

func TestFiltered_SkipsNilFilters(t *testing.T) {
	strategy := selector.Filtered(selector.RoundRobin(), nil, nil)
	if _, _, err := strategy.Pick(context.Background(), nil, instances("a:80")); err != nil {
		t.Fatalf("Pick: %v", err)
	}
}

func TestFiltered_RefusesInstancesTheCallerNeverDiscovered(t *testing.T) {
	// A filter that invents a candidate must not get it dialled.
	invent := sd.InstanceFilter(func(_ context.Context, _ []sd.Instance) []sd.Instance {
		return instances("elsewhere:80")
	})
	strategy := selector.Filtered(selector.RoundRobin(), invent)
	if _, _, err := strategy.Pick(context.Background(), nil, instances("a:80")); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("error = %v, want ErrNoEndpoints", err)
	}
}

func TestFiltered_PreservesTheStrategyCallback(t *testing.T) {
	reported := make(chan sd.Outcome, 1)
	inner := strategyFunc(func(_ context.Context, _ any, candidates []sd.Instance) (int, sd.Done, error) {
		return 0, func(outcome sd.Outcome) { reported <- outcome }, nil
	})

	strategy := selector.Filtered(inner, sd.Keep(func(sd.Instance) bool { return true }))
	_, done, err := strategy.Pick(context.Background(), nil, instances("a:80"))
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if done == nil {
		t.Fatal("the strategy callback was dropped")
	}
	done(sd.Outcome{Bytes: 7})
	if outcome := <-reported; outcome.Bytes != 7 {
		t.Fatalf("outcome = %+v, want the one the caller reported", outcome)
	}
}

func TestFiltered_NilStrategyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	selector.Filtered(nil)
}

func TestKeep_NilMatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	sd.Keep(nil)
}

type strategyFunc func(ctx context.Context, request any, instances []sd.Instance) (int, sd.Done, error)

func (f strategyFunc) Pick(ctx context.Context, request any, instances []sd.Instance) (int, sd.Done, error) {
	return f(ctx, request, instances)
}
