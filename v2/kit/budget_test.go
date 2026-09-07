package kit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// budgetComponent records the budget its Shutdown was given, and optionally
// spends all of it — which is what a component blocked on a slow dependency looks
// like.
type budgetComponent struct {
	name  string
	spend bool

	mu       sync.Mutex
	budget   time.Duration
	hadOne   bool
	expired  bool
	duration time.Duration
}

func (c *budgetComponent) Name() string         { return c.name }
func (c *budgetComponent) Start() error         { return nil }
func (c *budgetComponent) Errors() <-chan error { return nil }

func (c *budgetComponent) Shutdown(ctx context.Context) error {
	started := time.Now()
	c.mu.Lock()
	if deadline, ok := ctx.Deadline(); ok {
		c.hadOne = true
		c.budget = time.Until(deadline)
	}
	c.expired = ctx.Err() != nil
	c.mu.Unlock()

	if c.spend {
		<-ctx.Done()
	}
	c.mu.Lock()
	c.duration = time.Since(started)
	c.mu.Unlock()
	return nil
}

func (c *budgetComponent) observed() (budget, duration time.Duration, hadOne, expired bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.budget, c.duration, c.hadOne, c.expired
}

// TestShutdownGivesEachComponentAShareOfTheBudget proves a slow component cannot
// spend the whole grace period. With one shared deadline, the first component
// stopped could consume all of it and every component behind it would be handed a
// context that had already expired — a teardown that reads as graceful and behaves
// as a hard close.
func TestShutdownGivesEachComponentAShareOfTheBudget(t *testing.T) {
	first := &budgetComponent{name: "first"}
	second := &budgetComponent{name: "second"}
	slow := &budgetComponent{name: "slow", spend: true}

	// Attachment order is first, second, slow; teardown runs in reverse, so the
	// slow one goes first and is the one that could starve the others.
	host := kit.MustNewHost(kit.WithLifecycle(first, second, slow))
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	const budget = 300 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	if err := host.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	_, slowDuration, slowHadDeadline, _ := slow.observed()
	if !slowHadDeadline {
		t.Fatal("the slow component was given no deadline of its own")
	}
	if slowDuration >= budget {
		t.Errorf("the slow component spent %s of a %s budget, want a share of it", slowDuration, budget)
	}

	for _, component := range []*budgetComponent{second, first} {
		remaining, _, hadDeadline, expired := component.observed()
		if expired {
			t.Errorf("%s was shut down with an already expired context", component.name)
		}
		if !hadDeadline {
			t.Errorf("%s was given no deadline", component.name)
		}
		if remaining <= 0 {
			t.Errorf("%s had %s left, want time to stop in", component.name, remaining)
		}
	}
}

// TestShutdownWithoutADeadlineDividesNothing proves the share is derived rather
// than invented: a caller that passed no deadline gets none imposed on it.
func TestShutdownWithoutADeadlineDividesNothing(t *testing.T) {
	only := &budgetComponent{name: "only"}
	other := &budgetComponent{name: "other"}
	host := kit.MustNewHost(kit.WithLifecycle(only, other))
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	for _, component := range []*budgetComponent{only, other} {
		if _, _, hadDeadline, _ := component.observed(); hadDeadline {
			t.Errorf("%s was given a deadline the caller never set", component.name)
		}
	}
}
