package selector_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

func labelled(address string, labels map[string]any) sd.Instance {
	return sd.Instance{Address: address, Metadata: labels}
}

func pick(t *testing.T, strategy selector.Strategy, set []sd.Instance) sd.Instance {
	t.Helper()
	index, done, err := strategy.Pick(context.Background(), nil, set)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if done != nil {
		done(sd.Outcome{})
	}
	return set[index]
}

func TestStrategies_EmptySnapshotReportsNoEndpoints(t *testing.T) {
	key := func(context.Context, any) string { return "k" }
	strategies := map[string]selector.Strategy{
		"round robin":     selector.RoundRobin(),
		"random":          selector.Random(),
		"weighted":        selector.WeightedRandom(selector.MetadataWeight("", 1)),
		"scored":          selector.Scored(func(sd.Instance) (float64, bool) { return 1, true }),
		"consistent hash": selector.ConsistentHash(key),
	}

	for name, strategy := range strategies {
		t.Run(name, func(t *testing.T) {
			if _, _, err := strategy.Pick(context.Background(), nil, nil); !errors.Is(err, sd.ErrNoEndpoints) {
				t.Fatalf("Pick(nil) error = %v, want ErrNoEndpoints", err)
			}
		})
	}
}

func TestRoundRobin_WalksEverySlot(t *testing.T) {
	set := instances("a:80", "b:80", "c:80")
	strategy := selector.RoundRobin()

	seen := map[string]int{}
	for i := 0; i < 6; i++ {
		seen[pick(t, strategy, set).Address]++
	}
	for _, instance := range set {
		if seen[instance.Address] != 2 {
			t.Fatalf("distribution = %v, want two calls each", seen)
		}
	}
}

func TestRandom_ReachesEveryInstance(t *testing.T) {
	set := instances("a:80", "b:80", "c:80")
	strategy := selector.Random()

	seen := map[string]int{}
	for i := 0; i < 300; i++ {
		seen[pick(t, strategy, set).Address]++
	}
	if len(seen) != len(set) {
		t.Fatalf("distribution = %v, want every instance selected at least once", seen)
	}
}

func TestWeightedRandom_FollowsRegisteredWeights(t *testing.T) {
	set := []sd.Instance{
		labelled("heavy:80", map[string]any{"weight": 9}),
		labelled("light:80", map[string]any{"weight": 1}),
	}
	strategy := selector.WeightedRandom(selector.MetadataWeight(selector.DefaultWeightKey, 1))

	seen := map[string]int{}
	for i := 0; i < 1000; i++ {
		seen[pick(t, strategy, set).Address]++
	}
	if seen["heavy:80"] <= seen["light:80"]*3 {
		t.Fatalf("distribution = %v, want the 9:1 weight to dominate", seen)
	}
}

// Zero weight drains an instance, and every weight at zero is a
// selectable-instance shortage rather than an empty set.
func TestWeightedRandom_DrainsAndReportsShortage(t *testing.T) {
	set := []sd.Instance{
		labelled("draining:80", map[string]any{"weight": 0}),
		labelled("live:80", map[string]any{"weight": 5}),
	}
	strategy := selector.WeightedRandom(selector.MetadataWeight("", 1))

	for i := 0; i < 50; i++ {
		if got := pick(t, strategy, set).Address; got != "live:80" {
			t.Fatalf("selected %q, want the instance that is not draining", got)
		}
	}

	drained := []sd.Instance{labelled("a:80", map[string]any{"weight": 0})}
	if _, _, err := strategy.Pick(context.Background(), nil, drained); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Pick error = %v, want ErrNoEndpoints", err)
	}
}

// A registry reports labels as strings, and an absent label must fall back
// rather than silently weigh zero.
func TestMetadataWeight_CoercesAndFallsBack(t *testing.T) {
	weight := selector.MetadataWeight("", 7)

	if got := weight(labelled("a:80", map[string]any{"weight": "3"})); got != 3 {
		t.Errorf("registry string weight = %d, want 3", got)
	}
	if got := weight(labelled("b:80", nil)); got != 7 {
		t.Errorf("missing weight = %d, want the fallback 7", got)
	}
	if got := weight(labelled("c:80", map[string]any{"weight": "not a number"})); got != 7 {
		t.Errorf("unparsable weight = %d, want the fallback 7", got)
	}
}

func TestScored_PicksHighestAndExcludesRejected(t *testing.T) {
	set := instances("a:80", "b:80", "c:80")
	scores := map[string]float64{"a:80": 0.2, "b:80": 0.9, "c:80": 0.5}
	strategy := selector.Scored(func(instance sd.Instance) (float64, bool) {
		score, ok := scores[instance.Address]
		return score, ok
	})

	if got := pick(t, strategy, set).Address; got != "b:80" {
		t.Fatalf("selected %q, want the highest score b:80", got)
	}

	// A hard filter is expressed by refusing to score.
	filtered := selector.Scored(func(instance sd.Instance) (float64, bool) {
		if instance.Address == "b:80" {
			return 0, false
		}
		return scores[instance.Address], true
	})
	if got := pick(t, filtered, set).Address; got != "c:80" {
		t.Fatalf("selected %q, want c:80 once b:80 is excluded", got)
	}

	none := selector.Scored(func(sd.Instance) (float64, bool) { return 0, false })
	if _, _, err := none.Pick(context.Background(), nil, set); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Pick error = %v, want ErrNoEndpoints when every instance is excluded", err)
	}
}

// Equal scores must not pin every caller onto the first match.
func TestScored_SpreadsTies(t *testing.T) {
	set := instances("a:80", "b:80", "c:80")
	strategy := selector.Scored(func(sd.Instance) (float64, bool) { return 1, true })

	seen := map[string]int{}
	for i := 0; i < 300; i++ {
		seen[pick(t, strategy, set).Address]++
	}
	if len(seen) != len(set) {
		t.Fatalf("distribution = %v, want ties spread over every instance", seen)
	}
}

func TestConsistentHash_KeepsKeysOnTheirOwner(t *testing.T) {
	set := instances("a:80", "b:80", "c:80")
	key := func(_ context.Context, request any) string { return request.(string) }
	strategy := selector.ConsistentHash(key, selector.WithReplicas(200))

	owners := map[string]string{}
	for _, name := range []string{"tenant-1", "tenant-2", "tenant-3", "tenant-4"} {
		index, _, err := strategy.Pick(context.Background(), name, set)
		if err != nil {
			t.Fatalf("Pick(%s): %v", name, err)
		}
		owners[name] = set[index].Address
	}

	// Removing one instance may only remap the keys it owned.
	reduced := instances("a:80", "b:80")
	for name, owner := range owners {
		index, _, err := strategy.Pick(context.Background(), name, reduced)
		if err != nil {
			t.Fatalf("Pick(%s) after shrink: %v", name, err)
		}
		got := reduced[index].Address
		if owner != "c:80" && got != owner {
			t.Errorf("key %s moved from %q to %q even though %q stayed", name, owner, got, owner)
		}
	}
}

func TestConsistentHash_SpreadsKeysOverTheRing(t *testing.T) {
	set := instances("a:80", "b:80", "c:80")
	key := func(_ context.Context, request any) string { return request.(string) }
	strategy := selector.ConsistentHash(key)

	const total = 3000
	seen := map[string]int{}
	for i := 0; i < total; i++ {
		index, _, err := strategy.Pick(context.Background(), "key-"+string(rune('a'+i%26))+string(rune('a'+i/26%26))+string(rune('a'+i/676%26)), set)
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		seen[set[index].Address]++
	}
	for _, instance := range set {
		if share := seen[instance.Address]; share < total/10 {
			t.Fatalf("distribution = %v, want each instance to own at least 10%% of keys", seen)
		}
	}
}

// Relabelling must not reshuffle the ring: keys whose owner is still healthy
// have no reason to move.
func TestConsistentHash_IgnoresLabelChanges(t *testing.T) {
	key := func(_ context.Context, request any) string { return request.(string) }
	strategy := selector.ConsistentHash(key)

	before := []sd.Instance{labelled("a:80", map[string]any{"zone": "x"}), labelled("b:80", nil)}
	after := []sd.Instance{labelled("a:80", map[string]any{"zone": "y"}), labelled("b:80", nil)}

	first, _, err := strategy.Pick(context.Background(), "tenant-9", before)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	second, _, err := strategy.Pick(context.Background(), "tenant-9", after)
	if err != nil {
		t.Fatalf("Pick after relabel: %v", err)
	}
	if before[first].Address != after[second].Address {
		t.Fatalf("relabel moved the key from %q to %q", before[first].Address, after[second].Address)
	}
}

// An unkeyed request has nothing to pin on, so it must spread instead of
// piling onto one instance.
func TestConsistentHash_UnkeyedSelectionIsRandom(t *testing.T) {
	set := instances("a:80", "b:80", "c:80")
	strategy := selector.ConsistentHash(func(context.Context, any) string { return "" })

	seen := map[string]int{}
	for i := 0; i < 300; i++ {
		index, _, err := strategy.Pick(context.Background(), nil, set)
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		seen[set[index].Address]++
		seen[pick(t, strategy, set).Address]++
	}
	if len(seen) != len(set) {
		t.Fatalf("distribution = %v, want unkeyed selection to spread", seen)
	}
}

func TestStrategies_NilFunctionsPanic(t *testing.T) {
	for name, build := range map[string]func(){
		"weighted": func() { selector.WeightedRandom(nil) },
		"scored":   func() { selector.Scored(nil) },
		"hash":     func() { selector.ConsistentHash(nil) },
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
