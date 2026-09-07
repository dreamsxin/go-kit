package kit_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// drainRegistrar records when it was published and withdrawn.
type drainRegistrar struct {
	mu           sync.Mutex
	registers    int
	deregisters  int
	deregisterAt time.Time
	deregisterEr error
}

func (r *drainRegistrar) Register() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registers++
	return nil
}

func (r *drainRegistrar) Deregister() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deregisters++
	r.deregisterAt = time.Now()
	return r.deregisterEr
}

func (r *drainRegistrar) counts() (registers, deregisters int, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registers, r.deregisters, r.deregisterAt
}

// closingComponent records when its Shutdown ran, standing in for the listener
// that must not close before discovery has been told.
type closingComponent struct {
	mu         sync.Mutex
	shutdownAt time.Time
}

func (c *closingComponent) Name() string         { return "api" }
func (c *closingComponent) Start() error         { return nil }
func (c *closingComponent) Errors() <-chan error { return nil }

func (c *closingComponent) Shutdown(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdownAt = time.Now()
	return nil
}

func (c *closingComponent) closedAt() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.shutdownAt
}

// TestRegistrarDeregistersOnDrain proves leaving discovery is part of the
// announcement rather than part of the teardown: by the time anything is being
// stopped, callers looking this service up no longer find it.
func TestRegistrarDeregistersOnDrain(t *testing.T) {
	registrar := &drainRegistrar{}
	host := kit.MustNewHost(kit.WithRegistrar(registrar))
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if registers, deregisters, _ := registrar.counts(); registers != 1 || deregisters != 0 {
		t.Fatalf("after Start: %d register(s), %d deregister(s), want 1 and 0", registers, deregisters)
	}

	if err := host.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if _, deregisters, _ := registrar.counts(); deregisters != 1 {
		t.Fatalf("after Drain: %d deregister(s), want 1", deregisters)
	}

	// Shutdown is the fallback for a caller that skipped the announcement, so it
	// must not withdraw an instance twice — a second Deregister against a
	// provider that treats it as an error would fail a clean shutdown.
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if _, deregisters, _ := registrar.counts(); deregisters != 1 {
		t.Fatalf("after Shutdown: %d deregister(s), want the one from the announcement", deregisters)
	}
}

// TestRegistrarStillDeregistersWithoutADrain proves the fallback works: a Host
// shut down directly still withdraws the instance.
func TestRegistrarStillDeregistersWithoutADrain(t *testing.T) {
	registrar := &drainRegistrar{}
	host := kit.MustNewHost(kit.WithRegistrar(registrar))
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if _, deregisters, _ := registrar.counts(); deregisters != 1 {
		t.Fatalf("%d deregister(s), want 1", deregisters)
	}
}

// TestDeregistrationPrecedesTheDrainDelay proves the wait is on the useful side
// of the withdrawal. Deregistering and then immediately closing the listener
// would leave every peer that had already cached the address talking to a closed
// port; the delay exists to cover exactly that window, so it has to come after.
func TestDeregistrationPrecedesTheDrainDelay(t *testing.T) {
	registrar := &drainRegistrar{}
	api := &closingComponent{}
	const delay = 150 * time.Millisecond
	host := kit.MustNewHost(
		kit.WithLifecycle(api),       // binds the listener
		kit.WithRegistrar(registrar), // then publishes the address
		kit.WithDrainDelay(delay),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := host.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	_, deregisters, deregisteredAt := registrar.counts()
	if deregisters != 1 {
		t.Fatalf("%d deregister(s), want 1", deregisters)
	}
	closedAt := api.closedAt()
	if closedAt.IsZero() {
		t.Fatal("the serving component was never shut down")
	}
	if !deregisteredAt.Before(closedAt) {
		t.Fatalf("deregistered at %s but closed the listener at %s, want discovery told first",
			deregisteredAt, closedAt)
	}
	if waited := closedAt.Sub(deregisteredAt); waited < delay {
		t.Fatalf("waited %s between deregistering and closing, want at least the %s drain delay", waited, delay)
	}
}

// TestDeregistrationFailureIsReportedNotSwallowed proves a registry that refuses
// the withdrawal is visible: an instance still published while the process exits
// is the failure mode that sends traffic to a dead address.
func TestDeregistrationFailureIsReportedNotSwallowed(t *testing.T) {
	registrar := &drainRegistrar{deregisterEr: errors.New("etcd lease revoke failed")}
	host := kit.MustNewHost(kit.WithRegistrar(registrar))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := host.Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "etcd lease revoke failed") {
		t.Fatalf("Run = %v, want the deregistration failure reported", err)
	}
}
