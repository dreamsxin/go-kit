package selector

import (
	"context"
	"math/rand/v2"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// ScoreFunc rates an instance; the highest score wins. Returning false
// excludes the instance entirely, which is how a hard filter — draining,
// unhealthy, saturated — is expressed without a second predicate.
//
// Scores come from wherever the caller measures them: a load report pushed by
// the instances, ORCA or LRS style out-of-band reporting, or a table the
// process maintains itself. Whatever the source, a score that arrives out of
// band is stale by at least one reporting interval; that is inherent to the
// channel. Use LeastRequest, or feedback.Table.LeastRequest for a version that
// also records what it measured, when the caller is on the data path and can
// measure the truth.
type ScoreFunc func(instance sd.Instance) (score float64, ok bool)

// Scored selects the highest-scoring instance, breaking ties at random so that
// equal scores do not pin every caller onto the first match.
//
// Scored panics on a nil score function, which is a programming error rather
// than a runtime condition.
func Scored(score ScoreFunc) Strategy {
	if score == nil {
		panic("selector: nil score function")
	}
	return scored{score: score}
}

type scored struct {
	score ScoreFunc
}

func (s scored) Pick(_ context.Context, _ any, instances []sd.Instance) (int, sd.Done, error) {
	best, bestScore, ties := -1, 0.0, 0
	for i, instance := range instances {
		score, ok := s.score(instance)
		if !ok {
			continue
		}
		switch {
		case best < 0 || score > bestScore:
			best, bestScore, ties = i, score, 1
		case score == bestScore:
			// Reservoir sampling over the tied instances: one pass, no slice.
			ties++
			if rand.N(ties) == 0 {
				best = i
			}
		}
	}
	if best < 0 {
		return 0, nil, sd.ErrNoEndpoints
	}
	return best, nil, nil
}
