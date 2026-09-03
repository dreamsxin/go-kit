package balancer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

type closableStrategy struct{ closed bool }

func (s *closableStrategy) Pick(context.Context, any, []sd.Instance) (int, sd.Done, error) {
	return 0, nil, nil
}
func (s *closableStrategy) Close() error { s.closed = true; return nil }

type feedbackStrategy struct{ outcomes chan sd.Outcome }

func (s feedbackStrategy) Pick(context.Context, any, []sd.Instance) (int, sd.Done, error) {
	return 0, func(outcome sd.Outcome) { s.outcomes <- outcome }, nil
}

func labelledEndpointer(t *testing.T, instances ...sd.Instance) endpointer.InstanceEndpointer {
	t.Helper()
	cache := instance.NewCache()
	set := endpointer.NewEndpointer(cache, endpointer.Factory(echoFactory), nopLogger)
	t.Cleanup(func() { _ = set.Close() })
	cache.Update(sd.Event{Instances: instances})
	time.Sleep(20 * time.Millisecond)
	return set
}

// New turns any strategy into a Balancer, which is what keeps a custom
// strategy from having to reimplement endpoint lookup.
func TestNew_AppliesCustomStrategy(t *testing.T) {
	set := labelledEndpointer(t,
		sd.Instance{Address: "a:80", Metadata: map[string]any{"pick": false}},
		sd.Instance{Address: "b:80", Metadata: map[string]any{"pick": true}},
	)

	chooseLabelled := selector.Scored(func(_ context.Context, _ any, item sd.Instance) (float64, bool) {
		wanted, _ := sd.MetadataBool(item.Metadata, "pick")
		return 1, wanted
	})

	lb := balancer.New(set, chooseLabelled)
	for i := 0; i < 5; i++ {
		got := callPicked(t, pick(t, lb, nil), nil)
		if got != "b:80" {
			t.Fatalf("selected %v, want the labelled instance b:80", got)
		}
	}
}

// A request-aware strategy must keep its keyed path through New, or consistent
// hashing silently degrades to random selection.
func TestNewPassesRequestToStrategy(t *testing.T) {
	set := labelledEndpointer(t, sd.Addresses("a:80", "b:80", "c:80")...)

	lb := balancer.New(set, selector.ConsistentHash(func(_ context.Context, request any) string {
		return request.(string)
	}))
	first := pick(t, lb, "tenant-3")
	pinned := callPicked(t, first, nil)
	for i := 0; i < 10; i++ {
		got := callPicked(t, pick(t, lb, "tenant-3"), nil)
		if got != pinned {
			t.Fatalf("key moved from %v to %v", pinned, got)
		}
	}
}

func TestNew_NilArgumentsPanic(t *testing.T) {
	set := labelledEndpointer(t, sd.Addresses("a:80")...)

	for name, build := range map[string]func(){
		"nil source":   func() { balancer.New(nil, selector.RoundRobin()) },
		"nil strategy": func() { balancer.New(set, nil) },
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

func TestNew_CloseReleasesClosableStrategy(t *testing.T) {
	set := labelledEndpointer(t, sd.Addresses("a:80")...)
	strategy := &closableStrategy{}
	lb := balancer.New(set, strategy)
	if err := lb.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !strategy.closed {
		t.Fatal("balancer did not close its strategy")
	}
}

func TestNew_ForwardsPickedDone(t *testing.T) {
	set := labelledEndpointer(t, sd.Addresses("a:80")...)
	outcomes := make(chan sd.Outcome, 1)
	lb := balancer.New(set, feedbackStrategy{outcomes: outcomes})
	picked, err := lb.Pick(context.Background(), nil)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	want := sd.Outcome{Bytes: 9}
	picked.Done(want)
	select {
	case got := <-outcomes:
		if got.Bytes != want.Bytes {
			t.Fatalf("outcome = %+v, want %+v", got, want)
		}
	default:
		t.Fatal("strategy Done was not called")
	}
}

// NewScored is the seam for load signals measured elsewhere: the score table
// lives in the caller, and selection follows it without the registry being
// involved.
func TestNewScored_FollowsAnExternalScoreTable(t *testing.T) {
	set := labelledEndpointer(t, sd.Addresses("a:80", "b:80", "c:80")...)

	scores := map[string]float64{"a:80": 0.1, "b:80": 0.4, "c:80": 0.9}
	lb := balancer.NewScored(set, func(_ context.Context, _ any, item sd.Instance) (float64, bool) {
		score, known := scores[item.Address]
		return score, known
	})

	call := func() any {
		return callPicked(t, pick(t, lb, nil), nil)
	}

	if got := call(); got != "c:80" {
		t.Fatalf("selected %v, want the highest scoring c:80", got)
	}

	// The table is live: a new report changes the next selection without any
	// service-discovery event.
	scores["c:80"] = 0.0
	if got := call(); got != "b:80" {
		t.Fatalf("selected %v, want b:80 after c:80 was scored down", got)
	}
}

func TestNewScored_ExcludedInstancesReportNoEndpoints(t *testing.T) {
	set := labelledEndpointer(t, sd.Addresses("a:80")...)

	lb := balancer.NewScored(set, func(context.Context, any, sd.Instance) (float64, bool) { return 0, false })
	if _, err := lb.Pick(context.Background(), nil); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Endpoint error = %v, want ErrNoEndpoints", err)
	}
}
