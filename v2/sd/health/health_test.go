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
