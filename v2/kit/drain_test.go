package kit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// drainingComponent records what it was told and when, which is the whole point
// of the seam: a component decides for itself what draining means.
type drainingComponent struct {
	name      string
	events    *[]string
	mu        *sync.Mutex
	drainErr  error
	onDrain   func()
	onShutdow func()
}

func (c *drainingComponent) Name() string         { return c.name }
func (c *drainingComponent) Start() error         { return nil }
func (c *drainingComponent) Errors() <-chan error { return nil }
func (c *drainingComponent) record(event string) {
	c.mu.Lock()
	*c.events = append(*c.events, event)
	c.mu.Unlock()
}
func (c *drainingComponent) Drain(context.Context) error {
	c.record("drain:" + c.name)
	if c.onDrain != nil {
		c.onDrain()
	}
	return c.drainErr
}

func (c *drainingComponent) Shutdown(context.Context) error {
	c.record("shutdown:" + c.name)
	if c.onShutdow != nil {
		c.onShutdow()
	}
	return nil
}

func probeStatus(t *testing.T, component *kit.HTTP, path string) (int, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder.Code, recorder.Body.String()
}

// TestDrainFailsReadinessBeforeShutdown proves the announcement is the first
// thing that happens: a process that is going away stops claiming it is ready
// while it is still answering, which is the only signal a load balancer acts on.
// Liveness stays true, because finishing in-flight work is not a reason to be
// killed.
func TestDrainFailsReadinessBeforeShutdown(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	host := kit.MustNewHost(kit.WithLifecycle(component))

	if code, _ := probeStatus(t, component, "/readyz"); code != http.StatusOK {
		t.Fatalf("readyz before draining = %d, want 200", code)
	}
	if host.Draining() {
		t.Fatal("a host reports draining before anything asked it to stop")
	}

	if err := host.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !host.Draining() {
		t.Fatal("Draining() = false after Drain")
	}

	code, body := probeStatus(t, component, "/readyz")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("readyz while draining = %d, want 503", code)
	}
	if !strings.Contains(body, "draining") {
		t.Fatalf("readyz body = %s, want the draining check named", body)
	}
	if code, _ := probeStatus(t, component, "/livez"); code != http.StatusOK {
		t.Fatalf("livez while draining = %d, want 200: a draining process is not a dead one", code)
	}
}

// TestShutdownDrainsFirst proves the order holds even for a caller that skips
// Run: by the time a component is being torn down, readiness has already been
// failing. A component that observes readiness during its own Shutdown is the
// only witness that can tell those two orders apart.
func TestShutdownDrainsFirst(t *testing.T) {
	var mu sync.Mutex
	var events []string
	component := kit.MustNewHTTP("127.0.0.1:0")

	observedAtShutdown := 0
	observer := &drainingComponent{
		name: "observer", events: &events, mu: &mu,
		onShutdow: func() {
			recorder := httptest.NewRecorder()
			component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			observedAtShutdown = recorder.Code
		},
	}
	host := kit.MustNewHost(kit.WithLifecycle(component, observer))
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if observedAtShutdown != http.StatusServiceUnavailable {
		t.Fatalf("readyz observed during shutdown = %d, want 503", observedAtShutdown)
	}
	if got := strings.Join(events, ","); got != "drain:observer,shutdown:observer" {
		t.Fatalf("events = %s, want the drain before the shutdown", got)
	}
}

// TestDrainNotifiesComponentsInReverseOrder proves the announcement travels the
// same direction as teardown: the component nearest the caller stops accepting
// first, so nothing hands work to a component that has already stopped taking it.
func TestDrainNotifiesComponentsInReverseOrder(t *testing.T) {
	var mu sync.Mutex
	var events []string
	inner := &drainingComponent{name: "inner", events: &events, mu: &mu}
	outer := &drainingComponent{name: "outer", events: &events, mu: &mu}
	host := kit.MustNewHost(kit.WithLifecycle(inner, outer))

	if err := host.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if got := strings.Join(events, ","); got != "drain:outer,drain:inner" {
		t.Fatalf("events = %s, want the last attached component drained first", got)
	}

	// Drain is idempotent: a Run that drains and then calls Shutdown must not
	// announce twice, and neither must a caller that does both by hand.
	if err := host.Drain(context.Background()); err != nil {
		t.Fatalf("second Drain: %v", err)
	}
	if got := strings.Join(events, ","); got != "drain:outer,drain:inner" {
		t.Fatalf("events after a second Drain = %s, want no second announcement", got)
	}
}

// TestDrainReportsAComponentFailureWithoutStopping proves a component that cannot
// stop accepting work does not strand the rest: everything else was still told,
// and the teardown still runs.
func TestDrainReportsAComponentFailureWithoutStopping(t *testing.T) {
	var mu sync.Mutex
	var events []string
	failing := &drainingComponent{name: "failing", events: &events, mu: &mu, drainErr: errors.New("queue lease stuck")}
	quiet := &drainingComponent{name: "quiet", events: &events, mu: &mu}
	host := kit.MustNewHost(kit.WithLifecycle(quiet, failing))
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	err := host.Shutdown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "queue lease stuck") {
		t.Fatalf("Shutdown error = %v, want the drain failure reported", err)
	}
	if got := strings.Join(events, ","); got != "drain:failing,drain:quiet,shutdown:failing,shutdown:quiet" {
		t.Fatalf("events = %s, want every component drained and shut down", got)
	}
}

// TestRunWaitsTheDrainDelayBeforeShutdown proves the wait exists where it is
// useful: between the announcement nobody has read yet and the teardown that
// would fail the requests still being routed here.
func TestRunWaitsTheDrainDelayBeforeShutdown(t *testing.T) {
	var mu sync.Mutex
	var events []string
	var drainedAt, shutdownAt time.Time
	component := &drainingComponent{
		name: "worker", events: &events, mu: &mu,
		onDrain:   func() { drainedAt = time.Now() },
		onShutdow: func() { shutdownAt = time.Now() },
	}
	const delay = 150 * time.Millisecond
	host := kit.MustNewHost(kit.WithLifecycle(component), kit.WithDrainDelay(delay))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := host.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if waited := shutdownAt.Sub(drainedAt); waited < delay {
		t.Fatalf("waited %s between drain and shutdown, want at least %s", waited, delay)
	}
}

// TestRunWithoutADrainDelayStopsImmediately proves the wait is opt-in: an
// assembly that did not ask for one stops as directly as it did before the option
// existed.
func TestRunWithoutADrainDelayStopsImmediately(t *testing.T) {
	var mu sync.Mutex
	var events []string
	var drainedAt, shutdownAt time.Time
	component := &drainingComponent{
		name: "worker", events: &events, mu: &mu,
		onDrain:   func() { drainedAt = time.Now() },
		onShutdow: func() { shutdownAt = time.Now() },
	}
	host := kit.MustNewHost(kit.WithLifecycle(component))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := host.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if waited := shutdownAt.Sub(drainedAt); waited > 100*time.Millisecond {
		t.Fatalf("waited %s with no drain delay configured", waited)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Run took %s to stop with no drain delay", elapsed)
	}
}

// TestWithDrainDelayRejectsANegativeDelay proves the option validates where it is
// set rather than surprising someone during a deploy.
func TestWithDrainDelayRejectsANegativeDelay(t *testing.T) {
	if _, err := kit.NewHost(kit.WithDrainDelay(-time.Second)); err == nil {
		t.Fatal("NewHost accepted a negative drain delay")
	}
	if _, err := kit.NewHost(kit.WithDrainDelay(0)); err != nil {
		t.Fatalf("NewHost rejected a zero drain delay: %v", err)
	}
}

// TestDrainWithoutAProbeSurfaceIsNotAnError proves a pure worker still drains: it
// has nowhere to report readiness because nothing is asking, which is not a
// misconfiguration.
func TestDrainWithoutAProbeSurfaceIsNotAnError(t *testing.T) {
	var mu sync.Mutex
	var events []string
	worker := &drainingComponent{name: "worker", events: &events, mu: &mu}
	host, err := kit.NewHost(kit.WithLifecycle(worker))
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	if err := host.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !host.Draining() {
		t.Fatal("a host with no probe surface did not record that it is draining")
	}
}
