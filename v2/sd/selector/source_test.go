package selector_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

func snapshot(t *testing.T, source selector.Source) []string {
	t.Helper()
	set, err := source.Instances()
	if err != nil {
		t.Fatalf("Instances: %v", err)
	}
	addresses := make([]string, len(set))
	for i, item := range set {
		addresses[i] = item.Address
	}
	return addresses
}

func TestStatic_SortsAndIsolatesTheSnapshot(t *testing.T) {
	source := selector.Static(instances("b:80", "a:80")...)

	if got := snapshot(t, source); !reflect.DeepEqual(got, []string{"a:80", "b:80"}) {
		t.Fatalf("snapshot = %v, want it sorted by address", got)
	}

	// A caller must not be able to rewrite the source through the slice it got.
	set, err := source.Instances()
	if err != nil {
		t.Fatalf("Instances: %v", err)
	}
	set[0] = sd.Instance{Address: "mutated"}
	if got := snapshot(t, source); got[0] != "a:80" {
		t.Fatalf("source was mutated through a returned snapshot: %v", got)
	}
}

func TestFilter_KeepsOnlyMatchesAndNeverFallsBack(t *testing.T) {
	source := selector.Static(
		labelled("north:80", map[string]any{"zone": "north"}),
		labelled("south:80", map[string]any{"zone": "south"}),
	)

	local := selector.Filter(source, sd.MetadataEquals("zone", "north"))
	if got := snapshot(t, local); !reflect.DeepEqual(got, []string{"north:80"}) {
		t.Fatalf("filtered = %v, want only the north instance", got)
	}

	empty := selector.Filter(source, sd.MetadataEquals("zone", "east"))
	if got := snapshot(t, empty); len(got) != 0 {
		t.Fatalf("filtered = %v, want empty rather than a fallback", got)
	}
	if _, _, err := selector.New(empty, selector.RoundRobin()).Select(context.Background(), nil); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Select error = %v, want ErrNoEndpoints", err)
	}
}

func TestPrefer_SpillsOverWhenNothingMatches(t *testing.T) {
	source := selector.Static(
		labelled("north:80", map[string]any{"zone": "north"}),
		labelled("south:80", map[string]any{"zone": "south"}),
	)

	preferred := selector.Prefer(source, sd.MetadataEquals("zone", "north"))
	if got := snapshot(t, preferred); !reflect.DeepEqual(got, []string{"north:80"}) {
		t.Fatalf("preferred = %v, want the local instance while it exists", got)
	}

	spilled := selector.Prefer(source, sd.MetadataEquals("zone", "east"))
	if got := snapshot(t, spilled); len(got) != 2 {
		t.Fatalf("preferred = %v, want the full set as fallback", got)
	}
}

func TestFilterAndPrefer_PropagateSourceErrors(t *testing.T) {
	failing := errors.New("registry down")
	source := selector.SourceFunc(func() ([]sd.Instance, error) { return nil, failing })

	for name, decorated := range map[string]selector.Source{
		"filter": selector.Filter(source, sd.HasMetadata("zone")),
		"prefer": selector.Prefer(source, sd.HasMetadata("zone")),
	} {
		if _, err := decorated.Instances(); !errors.Is(err, failing) {
			t.Errorf("%s error = %v, want %v", name, err, failing)
		}
	}
}

func TestFilterAndPrefer_NilArgumentsPanic(t *testing.T) {
	source := selector.Static(instances("a:80")...)

	for name, build := range map[string]func(){
		"filter nil match":  func() { selector.Filter(source, nil) },
		"prefer nil match":  func() { selector.Prefer(source, nil) },
		"filter nil source": func() { selector.Filter(nil, sd.HasMetadata("zone")) },
		"prefer nil source": func() { selector.Prefer(nil, sd.HasMetadata("zone")) },
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

func TestSubscribe_TracksInstancerSnapshots(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: instances("b:80", "a:80")})

	subscription := selector.Subscribe(cache)
	t.Cleanup(func() { _ = subscription.Close() })

	if got := snapshot(t, subscription); !reflect.DeepEqual(got, []string{"a:80", "b:80"}) {
		t.Fatalf("initial snapshot = %v, want the registered instances sorted", got)
	}

	cache.Update(sd.Event{Instances: instances("c:80")})
	waitFor(t, subscription, []string{"c:80"})
}

// Without InvalidateOnError a registry outage must not withdraw instances that
// are still up.
func TestSubscribe_KeepsSnapshotThroughDiscoveryErrors(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: instances("a:80")})

	subscription := selector.Subscribe(cache)
	t.Cleanup(func() { _ = subscription.Close() })

	cache.Update(sd.Event{Err: errors.New("registry down")})
	time.Sleep(30 * time.Millisecond)

	if got := snapshot(t, subscription); !reflect.DeepEqual(got, []string{"a:80"}) {
		t.Fatalf("snapshot = %v, want the last good snapshot", got)
	}
}

func TestSubscribe_InvalidateOnErrorDropsSnapshotAfterGracePeriod(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: instances("a:80")})

	failing := errors.New("registry down")
	subscription := selector.Subscribe(cache, selector.InvalidateOnError(40*time.Millisecond))
	t.Cleanup(func() { _ = subscription.Close() })

	cache.Update(sd.Event{Err: failing})
	time.Sleep(10 * time.Millisecond)
	if got := snapshot(t, subscription); len(got) != 1 {
		t.Fatalf("snapshot within the grace period = %v, want the cached instance", got)
	}

	time.Sleep(60 * time.Millisecond)
	if _, err := subscription.Instances(); !errors.Is(err, failing) {
		t.Fatalf("Instances error = %v, want the discovery error after the grace period", err)
	}
}

func TestSubscribe_CloseStopsServingInstances(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: instances("a:80")})

	subscription := selector.Subscribe(cache)
	if err := subscription.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := subscription.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if _, err := subscription.Instances(); !errors.Is(err, selector.ErrClosed) {
		t.Fatalf("Instances error = %v, want ErrClosed", err)
	}
}

func TestSubscribe_NilInstancerPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic on a nil instancer")
		}
	}()
	selector.Subscribe(nil)
}

func waitFor(t *testing.T, source selector.Source, want []string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var got []string
	for time.Now().Before(deadline) {
		got = snapshot(t, source)
		if reflect.DeepEqual(got, want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("snapshot = %v, want %v", got, want)
}
