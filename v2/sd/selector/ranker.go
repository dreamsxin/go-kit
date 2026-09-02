package selector

import (
	"context"
	"sort"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// Ranker answers "where should I connect?" with an ordered shortlist instead of
// a single instance.
//
// This is the shape a routing service needs: a caller that dials the instance
// itself wants candidates to fail over through, not one address it has to come
// back for. Rank is score-based by construction — round robin and consistent
// hashing define which instance is next, not a total order over all of them, so
// they cannot produce a ranking.
type Ranker interface {
	// Rank returns up to n instances, best first. A value of n at or below zero
	// returns every candidate.
	Rank(ctx context.Context, n int) ([]sd.Instance, error)
}

// NewRanker orders the source's instances by score, after applying filters.
// Instances that score returns false for are excluded, which is how a caller
// drops candidates it has no signal for.
func NewRanker(source Source, score ScoreFunc, filters ...sd.InstanceFilter) Ranker {
	if source == nil {
		panic("selector: nil source")
	}
	if score == nil {
		panic("selector: nil score function")
	}
	return &ranker{source: source, score: score, filters: filters}
}

type ranker struct {
	source  Source
	score   ScoreFunc
	filters []sd.InstanceFilter
}

type ranked struct {
	instance sd.Instance
	score    float64
}

func (r *ranker) Rank(ctx context.Context, n int) ([]sd.Instance, error) {
	instances, err := r.source.Instances()
	if err != nil {
		return nil, err
	}
	for _, filter := range r.filters {
		if filter == nil {
			continue
		}
		instances = filter(ctx, instances)
		if len(instances) == 0 {
			return nil, sd.ErrNoEndpoints
		}
	}

	scored := make([]ranked, 0, len(instances))
	for _, instance := range instances {
		if value, ok := r.score(instance); ok {
			scored = append(scored, ranked{instance: instance, score: value})
		}
	}
	if len(scored) == 0 {
		return nil, sd.ErrNoEndpoints
	}

	// Address breaks ties so that equal scores produce a stable order: a caller
	// comparing two consecutive responses should see churn only when something
	// actually changed.
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].instance.Address < scored[j].instance.Address
	})

	if n > 0 && n < len(scored) {
		scored = scored[:n]
	}
	shortlist := make([]sd.Instance, len(scored))
	for i, item := range scored {
		shortlist[i] = item.instance
	}
	return shortlist, nil
}
