package selector_test

import (
	"context"
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// countingStrategy reserves an in-flight slot on every Pick and releases it in
// Done, the way least-request and slow-start strategies do.
type countingStrategy struct {
	inflight int
	index    int
}

func (s *countingStrategy) Pick(context.Context, any, []sd.Instance) (int, sd.Done, error) {
	s.inflight++
	return s.index, func(sd.Outcome) { s.inflight-- }, nil
}

// A wrapper that discards a successful inner Pick must release its Done, or the
// reservation leaks for the life of the process.
func TestFilteredReleasesTheInnerDoneWhenItRefusesThePick(t *testing.T) {
	instances := []sd.Instance{{Address: "a:80"}}

	t.Run("index out of range", func(t *testing.T) {
		inner := &countingStrategy{index: 7}
		strategy := selector.Filtered(inner)

		if _, _, err := strategy.Pick(context.Background(), nil, instances); err == nil {
			t.Fatal("an out-of-range index must be refused")
		}
		if inner.inflight != 0 {
			t.Fatalf("in-flight = %d, want 0: the inner Done was dropped", inner.inflight)
		}
	})

	t.Run("filter invented an address", func(t *testing.T) {
		inner := &countingStrategy{}
		strategy := selector.Filtered(inner, func(_ context.Context, _ []sd.Instance) []sd.Instance {
			return []sd.Instance{{Address: "never-discovered:80"}}
		})

		if _, _, err := strategy.Pick(context.Background(), nil, instances); err == nil {
			t.Fatal("an address the caller never discovered must be refused")
		}
		if inner.inflight != 0 {
			t.Fatalf("in-flight = %d, want 0: the inner Done was dropped", inner.inflight)
		}
	})
}

func TestReleaseToleratesANilDone(t *testing.T) {
	sd.Release(nil, sd.ErrNoEndpoints) // must not panic
}
