package health_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/health"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
)

// probeTable answers probes from a per-address verdict the test controls.
type probeTable struct {
	mu       sync.Mutex
	failures map[string]error
	calls    map[string]int
}

func newProbeTable() *probeTable {
	return &probeTable{failures: map[string]error{}, calls: map[string]int{}}
}

func (p *probeTable) probe() health.Probe {
	return func(_ context.Context, target sd.Instance) error {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.calls[target.Address]++
		return p.failures[target.Address]
	}
}

func (p *probeTable) fail(address string, err error) {
	p.mu.Lock()
	p.failures[address] = err
	p.mu.Unlock()
}

func (p *probeTable) heal(address string) {
	p.mu.Lock()
	delete(p.failures, address)
	p.mu.Unlock()
}

func (p *probeTable) probed(address string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[address]
}

func source(t *testing.T, addresses ...string) *instance.Cache {
	t.Helper()
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses(addresses...)})
	t.Cleanup(func() { _ = cache.Close() })
	return cache
}

func addressesOf(event sd.Event) []string {
	addresses := make([]string, len(event.Instances))
	for i, target := range event.Instances {
		addresses[i] = target.Address
	}
	return addresses
}

func waitForAddresses(t *testing.T, checker *health.Checker, want ...string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last []string
	for time.Now().Before(deadline) {
		last = addressesOf(checker.Register(nil))
		if equal(last, want) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("published instances = %v, want %v", last, want)
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// recyclingInstancer is a provider that overwrites the snapshot slice it already
// handed out, which sd.Event forbids. The checker keeps the snapshot between
// probe rounds, so aliasing it would let a provider bug rewrite the set being
// probed and published.
type recyclingInstancer struct {
	mu        sync.Mutex
	instances []sd.Instance
}

func (r *recyclingInstancer) Register(chan sd.Event) sd.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return sd.Event{Instances: r.instances}
}

func (r *recyclingInstancer) Deregister(chan sd.Event) {}

func (r *recyclingInstancer) Close() error { return nil }

// recycle rewrites the addresses in place, reusing the backing array.
func (r *recyclingInstancer) recycle(addresses ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, address := range addresses {
		r.instances[i] = sd.Instance{Address: address}
	}
}

// rudeInstancer closes the channel it was handed, which sd.Instancer forbids.
type rudeInstancer struct {
	mu          sync.Mutex
	instances   []sd.Instance
	subscribers []chan sd.Event
}

func (r *rudeInstancer) Register(ch chan sd.Event) sd.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ch != nil {
		r.subscribers = append(r.subscribers, ch)
	}
	return sd.Event{Instances: append([]sd.Instance(nil), r.instances...)}
}

func (r *rudeInstancer) Deregister(chan sd.Event) {}

func (r *rudeInstancer) Close() error { return nil }

func (r *rudeInstancer) closeSubscribers() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, subscriber := range r.subscribers {
		close(subscriber)
	}
	r.subscribers = nil
}

func TestCheck_SurvivesAProviderThatClosesItsChannel(t *testing.T) {
	probes := newProbeTable()
	provider := &rudeInstancer{instances: sd.Addresses("a:80", "b:80")}

	checker := health.Check(provider, probes.probe(),
		health.WithInterval(2*time.Millisecond),
		health.WithUnhealthyThreshold(1))
	defer checker.Close() //nolint:errcheck

	waitForAddresses(t, checker, "a:80", "b:80")
	provider.closeSubscribers()

	// The set the checker already has stays published, and probing keeps
	// running instead of spinning on zero-value events.
	rounds := probes.probed("a:80")
	time.Sleep(20 * time.Millisecond)
	waitForAddresses(t, checker, "a:80", "b:80")
	if probes.probed("a:80") <= rounds {
		t.Fatal("probing stopped after the provider closed the channel")
	}

	probes.fail("a:80", errors.New("down"))
	waitForAddresses(t, checker, "b:80")
}

func TestCheck_OwnsTheSnapshotItWasGiven(t *testing.T) {
	probes := newProbeTable()
	provider := &recyclingInstancer{instances: sd.Addresses("a:80", "b:80")}

	checker := health.Check(provider, probes.probe(),
		health.WithInterval(2*time.Millisecond),
		health.WithUnhealthyThreshold(1))
	defer checker.Close() //nolint:errcheck

	waitForAddresses(t, checker, "a:80", "b:80")

	provider.recycle("rewritten-a:80", "rewritten-b:80")

	// Nothing new was published, so the checker still holds what it copied.
	waitForAddresses(t, checker, "a:80", "b:80")
	if probes.probed("rewritten-a:80") != 0 {
		t.Error("the checker probed an address the provider wrote after publishing")
	}
}

func TestCheck_RemovesAnInstanceAfterTheUnhealthyThreshold(t *testing.T) {
	probes := newProbeTable()
	probes.fail("bad:80", errors.New("connection refused"))

	checker := health.Check(source(t, "bad:80", "good:80"), probes.probe(),
		health.WithInterval(2*time.Millisecond),
		health.WithUnhealthyThreshold(3))
	defer checker.Close() //nolint:errcheck

	waitForAddresses(t, checker, "good:80")

	// Three consecutive failures were required, so the instance survived the
	// first two rounds.
	if got := probes.probed("bad:80"); got < 3 {
		t.Fatalf("probes for the failing instance = %d, want at least 3", got)
	}
}

func TestCheck_KeepsAnInstanceBelowTheThreshold(t *testing.T) {
	probes := newProbeTable()
	probes.fail("flaky:80", errors.New("one blip"))

	checker := health.Check(source(t, "flaky:80", "good:80"), probes.probe(),
		health.WithInterval(time.Hour), // only the immediate first round runs
		health.WithUnhealthyThreshold(3))
	defer checker.Close() //nolint:errcheck

	// One failure out of three required must not take it out of service.
	waitForProbe(t, probes, "flaky:80")
	if got := addressesOf(checker.Register(nil)); len(got) != 2 {
		t.Fatalf("published instances = %v, want both", got)
	}
}

func TestCheck_ReturnsAnInstanceAfterTheHealthyThreshold(t *testing.T) {
	probes := newProbeTable()
	probes.fail("recovering:80", errors.New("down"))

	checker := health.Check(source(t, "recovering:80", "good:80"), probes.probe(),
		health.WithInterval(2*time.Millisecond),
		health.WithUnhealthyThreshold(1),
		health.WithHealthyThreshold(2))
	defer checker.Close() //nolint:errcheck

	waitForAddresses(t, checker, "good:80")
	probes.heal("recovering:80")
	waitForAddresses(t, checker, "good:80", "recovering:80")
}

// Nothing answering usually means the probe is wrong, not that every instance is
// down. Publishing an empty set would turn a monitoring fault into an outage.
func TestCheck_PublishesTheUncheckedSetWhenNothingPasses(t *testing.T) {
	probes := newProbeTable()
	failure := errors.New("probe misconfigured")
	probes.fail("a:80", failure)
	probes.fail("b:80", failure)

	checker := health.Check(source(t, "a:80", "b:80"), probes.probe(),
		health.WithInterval(2*time.Millisecond),
		health.WithUnhealthyThreshold(1))
	defer checker.Close() //nolint:errcheck

	waitForProbe(t, probes, "a:80")
	waitForProbe(t, probes, "b:80")
	waitForAddresses(t, checker, "a:80", "b:80")
}

// Fail-open is a default, not a law. A caller for whom reaching a dead instance
// is worse than reaching none must be able to turn it off, the same way
// sd/feedback lets MaxEjectionPercent be configured.
func TestCheck_FailClosedPublishesNothingWhenEveryProbeFails(t *testing.T) {
	probes := newProbeTable()
	failure := errors.New("probe misconfigured")
	probes.fail("a:80", failure)
	probes.fail("b:80", failure)

	checker := health.Check(source(t, "a:80", "b:80"), probes.probe(),
		health.WithInterval(2*time.Millisecond),
		health.WithUnhealthyThreshold(1),
		health.WithHealthyThreshold(1),
		health.WithFailOpen(false))
	defer checker.Close() //nolint:errcheck

	waitForProbe(t, probes, "a:80")
	waitForProbe(t, probes, "b:80")
	waitForAddresses(t, checker)

	// And it recovers: the option withholds the unchecked set, it does not
	// latch the checker off.
	probes.heal("a:80")
	waitForAddresses(t, checker, "a:80")
}

func TestCheck_PassesDiscoveryErrorsThrough(t *testing.T) {
	probes := newProbeTable()
	cache := instance.NewCache()
	t.Cleanup(func() { _ = cache.Close() })
	cache.Update(sd.Event{Instances: sd.Addresses("a:80")})

	checker := health.Check(cache, probes.probe(), health.WithInterval(time.Hour))
	defer checker.Close() //nolint:errcheck

	failure := errors.New("registry unreachable")
	cache.Update(sd.Event{Err: failure})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if event := checker.Register(nil); errors.Is(event.Err, failure) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the discovery error never reached subscribers")
}

func TestCheck_HidesNewInstancesUntilTheyPassWhenAskedTo(t *testing.T) {
	probes := newProbeTable()
	probes.fail("cold:80", errors.New("still starting"))

	cache := instance.NewCache()
	t.Cleanup(func() { _ = cache.Close() })
	cache.Update(sd.Event{Instances: sd.Addresses("warm:80")})

	checker := health.Check(cache, probes.probe(),
		health.WithInterval(2*time.Millisecond),
		health.WithInitiallyHealthy(false),
		health.WithHealthyThreshold(1))
	defer checker.Close() //nolint:errcheck

	waitForAddresses(t, checker, "warm:80")
	cache.Update(sd.Event{Instances: sd.Addresses("cold:80", "warm:80")})

	// cold:80 must not receive traffic before it answers once.
	waitForProbe(t, probes, "cold:80")
	if got := addressesOf(checker.Register(nil)); len(got) != 1 || got[0] != "warm:80" {
		t.Fatalf("published instances = %v, want only the instance that passed", got)
	}

	probes.heal("cold:80")
	waitForAddresses(t, checker, "cold:80", "warm:80")
}

// The fail-open for "everything failed" must not fire before anything has been
// probed, or WithInitiallyHealthy(false) defeats itself at startup: nothing is
// healthy yet, so republishing the unchecked set would publish exactly the
// instances the option asks to hide.
func TestCheck_DoesNotFailOpenBeforeTheFirstProbeCompletes(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }

	entered := make(chan string, 2)
	blocking := health.Probe(func(_ context.Context, target sd.Instance) error {
		entered <- target.Address
		<-release
		return errors.New("down")
	})

	checker := health.Check(source(t, "a:80", "b:80"), blocking,
		health.WithInterval(time.Hour),
		health.WithInitiallyHealthy(false),
		health.WithConcurrency(2))
	// Cleanups run last in first out, so the probes are released before Close
	// waits for them.
	t.Cleanup(func() { _ = checker.Close() })
	t.Cleanup(unblock)

	// Check publishes on the initial snapshot, before any probe has answered.
	if got := addressesOf(checker.Register(nil)); len(got) != 0 {
		t.Fatalf("published %v before the first probe, want nothing", got)
	}

	<-entered
	<-entered
	unblock()

	// Now every instance has been probed and every probe failed, which is the
	// case the fail-open exists for.
	waitForAddresses(t, checker, "a:80", "b:80")
}

func TestCheck_StopsProbingAndUnsubscribesOnClose(t *testing.T) {
	probes := newProbeTable()
	cache := source(t, "a:80")

	checker := health.Check(cache, probes.probe(), health.WithInterval(2*time.Millisecond))
	waitForProbe(t, probes, "a:80")

	if err := checker.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := checker.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	settled := probes.probed("a:80")
	time.Sleep(20 * time.Millisecond)
	if got := probes.probed("a:80"); got != settled {
		t.Fatalf("probes continued after Close: %d then %d", settled, got)
	}
}

func TestCheck_NilArgumentsPanic(t *testing.T) {
	probes := newProbeTable()
	for name, build := range map[string]func(){
		"nil instancer": func() { health.Check(nil, probes.probe()) },
		"nil probe":     func() { health.Check(instance.NewCache(), nil) },
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

func TestTCPProbe(t *testing.T) {
	listener, err := listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close() //nolint:errcheck

	probe := health.TCPProbe(time.Second)
	if err := probe(context.Background(), sd.Instance{Address: listener.Addr().String()}); err != nil {
		t.Fatalf("probe against a live listener: %v", err)
	}

	address := listener.Addr().String()
	_ = listener.Close()
	if err := probe(context.Background(), sd.Instance{Address: address}); err == nil {
		t.Fatal("probe against a closed port reported success")
	}
}

func listen() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}

func waitForProbe(t *testing.T, probes *probeTable, address string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if probes.probed(address) > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s was never probed", address)
}
