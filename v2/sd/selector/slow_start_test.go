package selector_test

import (
	"context"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

func TestSlowStart_RampsWeightOverTheWindow(t *testing.T) {
	instance := sd.Instance{Address: "new:80"}
	full := selector.WeightFunc(func(sd.Instance) int { return 100 })
	window := time.Minute

	tests := map[string]struct {
		elapsed time.Duration
		want    int
	}{
		"just started":  {elapsed: 0, want: 1},
		"quarter way":   {elapsed: 15 * time.Second, want: 25},
		"half way":      {elapsed: 30 * time.Second, want: 50},
		"window passed": {elapsed: 2 * time.Minute, want: 100},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			firstSeen := time.Now().Add(-tt.elapsed)
			weight := selector.SlowStart(full, func(sd.Instance) (time.Time, bool) {
				return firstSeen, true
			}, window)

			got := weight(instance)
			// The ramp is time-based, so allow for the clock moving during the
			// test rather than asserting an exact share.
			if got < tt.want-2 || got > tt.want+2 {
				t.Fatalf("weight = %d, want about %d", got, tt.want)
			}
		})
	}
}

// A warming instance must still receive the traffic that warms it, so the ramp
// floors at one rather than zero.
func TestSlowStart_NeverStarvesAWarmingInstance(t *testing.T) {
	weight := selector.SlowStart(func(sd.Instance) int { return 3 },
		func(sd.Instance) (time.Time, bool) { return time.Now(), true }, time.Hour)

	if got := weight(sd.Instance{Address: "new:80"}); got != 1 {
		t.Fatalf("weight = %d, want 1", got)
	}
}

func TestSlowStart_TreatsUnknownInstancesAsBrandNew(t *testing.T) {
	weight := selector.SlowStart(func(sd.Instance) int { return 100 },
		func(sd.Instance) (time.Time, bool) { return time.Time{}, false }, time.Minute)

	if got := weight(sd.Instance{Address: "unknown:80"}); got != 1 {
		t.Fatalf("weight = %d, want 1", got)
	}
}

// Zero means "never pick me"; ramping would contradict that.
func TestSlowStart_LeavesZeroWeightAlone(t *testing.T) {
	weight := selector.SlowStart(func(sd.Instance) int { return 0 },
		func(sd.Instance) (time.Time, bool) { return time.Now(), true }, time.Minute)

	if got := weight(sd.Instance{Address: "drained:80"}); got != 0 {
		t.Fatalf("weight = %d, want 0", got)
	}
}

func TestSlowStart_WithoutAWindowIsANoOp(t *testing.T) {
	weight := selector.SlowStart(func(sd.Instance) int { return 42 },
		func(sd.Instance) (time.Time, bool) { return time.Now(), true }, 0)

	if got := weight(sd.Instance{Address: "any:80"}); got != 42 {
		t.Fatalf("weight = %d, want the original weight", got)
	}
}

func TestSlowStart_NilArgumentsPanic(t *testing.T) {
	for name, build := range map[string]func(){
		"nil weight": func() {
			selector.SlowStart(nil, func(sd.Instance) (time.Time, bool) { return time.Now(), true }, time.Minute)
		},
		"nil first seen": func() {
			selector.SlowStart(func(sd.Instance) int { return 1 }, nil, time.Minute)
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

// Slow start composes with weighted selection exactly as an operator-set weight
// does: a warming instance is picked, just rarely.
func TestSlowStart_ComposesWithWeightedRandom(t *testing.T) {
	warm := sd.Instance{Address: "warm:80"}
	cold := sd.Instance{Address: "cold:80"}
	firstSeen := map[string]time.Time{
		warm.Address: time.Now().Add(-time.Hour),
		cold.Address: time.Now(),
	}

	weight := selector.SlowStart(func(sd.Instance) int { return 100 },
		func(instance sd.Instance) (time.Time, bool) {
			seen, ok := firstSeen[instance.Address]
			return seen, ok
		}, time.Minute)

	strategy := selector.WeightedRandom(weight)
	set := []sd.Instance{warm, cold}
	picks := map[string]int{}
	for range 500 {
		index, _, err := strategy.Pick(context.Background(), nil, set)
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		picks[set[index].Address]++
	}

	if picks[warm.Address] < 400 {
		t.Fatalf("picks = %v, want the warm instance to take almost everything", picks)
	}
}
