package selector_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

func scoreByAddress(scores map[string]float64) selector.ScoreFunc {
	return func(instance sd.Instance) (float64, bool) {
		score, ok := scores[instance.Address]
		return score, ok
	}
}

// tenantRanker is the reason Rank takes a request: a shortlist can depend on who
// is asking. NewRanker's scoring deliberately does not see the request — a score
// is a property of the instance — so a ranking that varies per request is a
// custom implementation, and this is what one looks like.
type tenantRanker struct {
	source selector.Source
	home   map[string]string // tenant -> zone
}

func (r tenantRanker) Rank(_ context.Context, request any, n int) ([]sd.Instance, error) {
	available, err := r.source.Instances()
	if err != nil {
		return nil, err
	}
	tenant, _ := request.(string)
	zone := r.home[tenant]

	ranked := make([]sd.Instance, 0, len(available))
	for _, candidate := range available {
		if candidate.Metadata["zone"] == zone {
			ranked = append(ranked, candidate)
		}
	}
	for _, candidate := range available {
		if candidate.Metadata["zone"] != zone {
			ranked = append(ranked, candidate)
		}
	}
	if n > 0 && n < len(ranked) {
		ranked = ranked[:n]
	}
	return ranked, nil
}

func TestRanker_ACustomImplementationCanRankByRequest(t *testing.T) {
	var _ selector.Ranker = tenantRanker{}

	pool := selector.Static(
		sd.Instance{Address: "east:80", Metadata: map[string]any{"zone": "east"}},
		sd.Instance{Address: "west:80", Metadata: map[string]any{"zone": "west"}},
	)
	ranker := tenantRanker{source: pool, home: map[string]string{
		"acme": "west", "globex": "east",
	}}

	for tenant, want := range map[string]string{"acme": "west:80", "globex": "east:80"} {
		shortlist, err := ranker.Rank(context.Background(), tenant, 1)
		if err != nil {
			t.Fatalf("Rank(%q): %v", tenant, err)
		}
		if got := addressesOfRanked(shortlist); len(got) != 1 || got[0] != want {
			t.Errorf("shortlist for %q = %v, want %v", tenant, got, want)
		}
	}

	// A caller with nothing to say passes nil, and still gets every candidate.
	shortlist, err := ranker.Rank(context.Background(), nil, 0)
	if err != nil {
		t.Fatalf("Rank(nil): %v", err)
	}
	if len(shortlist) != 2 {
		t.Errorf("shortlist for no request = %v, want both", addressesOfRanked(shortlist))
	}
}

func TestRanker_OrdersByScoreAndTruncates(t *testing.T) {
	source := selector.Static(instances("low:80", "high:80", "mid:80")...)
	ranker := selector.NewRanker(source, scoreByAddress(map[string]float64{
		"low:80": 0.1, "mid:80": 0.5, "high:80": 0.9,
	}))

	shortlist, err := ranker.Rank(context.Background(), nil, 2)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if got := addressesOfRanked(shortlist); len(got) != 2 || got[0] != "high:80" || got[1] != "mid:80" {
		t.Fatalf("shortlist = %v, want the two best in order", got)
	}
}

func TestRanker_ReturnsEverythingWhenNIsNotPositive(t *testing.T) {
	source := selector.Static(instances("a:80", "b:80", "c:80")...)
	ranker := selector.NewRanker(source, scoreByAddress(map[string]float64{
		"a:80": 1, "b:80": 2, "c:80": 3,
	}))

	shortlist, err := ranker.Rank(context.Background(), nil, 0)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if len(shortlist) != 3 {
		t.Fatalf("shortlist = %v, want every candidate", addressesOfRanked(shortlist))
	}
}

// A caller comparing two consecutive responses should see churn only when
// something actually changed, so equal scores must order deterministically.
func TestRanker_BreaksTiesByAddress(t *testing.T) {
	source := selector.Static(instances("b:80", "a:80", "c:80")...)
	ranker := selector.NewRanker(source, func(sd.Instance) (float64, bool) { return 1, true })

	first, err := ranker.Rank(context.Background(), nil, 3)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if got := addressesOfRanked(first); got[0] != "a:80" || got[1] != "b:80" || got[2] != "c:80" {
		t.Fatalf("shortlist = %v, want address order", got)
	}
}

func TestRanker_ExcludesInstancesWithoutAScore(t *testing.T) {
	source := selector.Static(instances("known:80", "unknown:80")...)
	ranker := selector.NewRanker(source, scoreByAddress(map[string]float64{"known:80": 1}))

	shortlist, err := ranker.Rank(context.Background(), nil, 0)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if got := addressesOfRanked(shortlist); len(got) != 1 || got[0] != "known:80" {
		t.Fatalf("shortlist = %v, want only the scored instance", got)
	}
}

func TestRanker_NothingScorableReportsNoEndpoints(t *testing.T) {
	source := selector.Static(instances("a:80")...)
	ranker := selector.NewRanker(source, func(sd.Instance) (float64, bool) { return 0, false })

	if _, err := ranker.Rank(context.Background(), nil, 0); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("error = %v, want ErrNoEndpoints", err)
	}
}

func TestRanker_AppliesFilters(t *testing.T) {
	set := []sd.Instance{
		labelled("eu:80", map[string]any{"zone": "eu"}),
		labelled("us:80", map[string]any{"zone": "us"}),
	}
	ranker := selector.NewRanker(selector.Static(set...),
		func(sd.Instance) (float64, bool) { return 1, true },
		sd.Keep(sd.MetadataEquals("zone", "eu")))

	shortlist, err := ranker.Rank(context.Background(), nil, 0)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if got := addressesOfRanked(shortlist); len(got) != 1 || got[0] != "eu:80" {
		t.Fatalf("shortlist = %v, want only the eu instance", got)
	}
}

func TestRanker_PropagatesSourceErrors(t *testing.T) {
	failure := errors.New("registry down")
	source := selector.SourceFunc(func() ([]sd.Instance, error) { return nil, failure })
	ranker := selector.NewRanker(source, func(sd.Instance) (float64, bool) { return 1, true })

	if _, err := ranker.Rank(context.Background(), nil, 1); !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the source error", err)
	}
}

func TestNewRanker_NilArgumentsPanic(t *testing.T) {
	for name, build := range map[string]func(){
		"nil source": func() {
			selector.NewRanker(nil, func(sd.Instance) (float64, bool) { return 1, true })
		},
		"nil score": func() {
			selector.NewRanker(selector.Static(), nil)
		},
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

func addressesOfRanked(set []sd.Instance) []string {
	addresses := make([]string, len(set))
	for i, instance := range set {
		addresses[i] = instance.Address
	}
	return addresses
}
