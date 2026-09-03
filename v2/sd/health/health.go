// Package health adds active health checking to service discovery.
//
// Passive feedback — sd/feedback — can only judge instances that received
// traffic. That leaves two blind spots: an instance nothing has called yet, and
// an instance that is unreachable rather than slow. Active probing covers both,
// which is why every load balancer product ships it.
//
// Check is an sd.Instancer decorator rather than a filter, because probing is a
// background activity with a lifetime, and sd.Instancer is the discovery
// contract that has Close. Everything downstream — endpointer, selector,
// balancer — works unchanged.
package health

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
)

// Defaults chosen so that a failure is noticed in about half a minute without
// the probe traffic being noticeable.
const (
	DefaultInterval           = 10 * time.Second
	DefaultTimeout            = 2 * time.Second
	DefaultUnhealthyThreshold = 3
	DefaultHealthyThreshold   = 1
	DefaultConcurrency        = 8
)

// Probe reports whether one instance is serving. A nil error means healthy.
//
// A Probe must return when its context is cancelled. Close cancels the context
// and waits for the round in flight, so a Probe that ignores cancellation makes
// Close block for as long as that probe takes. TCPProbe and HTTPProbe honour it.
type Probe func(ctx context.Context, instance sd.Instance) error

// TCPProbe connects to the instance address and closes it again. It proves the
// port accepts connections, nothing more, which is what an L4 balancer checks.
func TCPProbe(timeout time.Duration) Probe {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	dialer := &net.Dialer{Timeout: timeout}
	return func(ctx context.Context, target sd.Instance) error {
		conn, err := dialer.DialContext(ctx, "tcp", target.Address)
		if err != nil {
			return err
		}
		return conn.Close()
	}
}

// HTTPProbe issues a GET against the instance and treats any status below 400 as
// healthy. An empty scheme means http.
func HTTPProbe(scheme, path string, timeout time.Duration) Probe {
	if scheme == "" {
		scheme = "http"
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	client := &http.Client{Timeout: timeout}
	return func(ctx context.Context, target sd.Instance) error {
		url := fmt.Sprintf("%s://%s%s", scheme, target.Address, path)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close() //nolint:errcheck
		if response.StatusCode >= 400 {
			return fmt.Errorf("health: %s returned %s", url, response.Status)
		}
		return nil
	}
}

// Option configures a Checker.
type Option func(*Checker)

// WithInterval sets how often every instance is probed.
func WithInterval(interval time.Duration) Option {
	return func(c *Checker) {
		if interval > 0 {
			c.interval = interval
		}
	}
}

// WithTimeout bounds one probe. It does not replace the timeout inside a Probe
// implementation; it cancels the context handed to it.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Checker) {
		if timeout > 0 {
			c.timeout = timeout
		}
	}
}

// WithUnhealthyThreshold sets how many consecutive failures remove an instance.
// Above one, a single blip cannot take an instance out of service.
func WithUnhealthyThreshold(failures int) Option {
	return func(c *Checker) {
		if failures > 0 {
			c.unhealthyThreshold = failures
		}
	}
}

// WithHealthyThreshold sets how many consecutive successes return an instance
// to service.
func WithHealthyThreshold(successes int) Option {
	return func(c *Checker) {
		if successes > 0 {
			c.healthyThreshold = successes
		}
	}
}

// WithInitiallyHealthy decides how an instance is treated before its first
// probe completes.
//
// The default is healthy. A client-side balancer that assumed the opposite
// would have nothing to call for one probe interval after startup, turning a
// restart into an outage. A gateway in front of slow-starting backends may
// prefer false.
//
// With false, each unprobed instance stays out of the published set until its
// first probe lands. Existing instances that have already passed continue to
// serve. The fail-open in publish does not override this, because it only
// applies once every instance has actually been probed.
func WithInitiallyHealthy(healthy bool) Option {
	return func(c *Checker) {
		c.initiallyHealthy = healthy
	}
}

// WithConcurrency bounds how many probes run at once.
func WithConcurrency(probes int) Option {
	return func(c *Checker) {
		if probes > 0 {
			c.concurrency = probes
		}
	}
}

// WithFailOpen decides what to publish when every instance has been probed and
// none of them passed.
//
// The default is true: the unchecked set is published, on the reasoning that a
// probe failing for every instance is far more likely to be broken — a
// firewall, a path that moved, a threshold set too tight — than the whole
// service being down, and publishing an empty set turns a monitoring fault into
// an outage. Envoy calls the same idea panic mode, and sd/feedback spells it
// MaxEjectionPercent.
//
// Set false when calling a dead instance is worse than calling nothing: a
// writer that must not double-apply, a pool where a failed probe means the
// backend is mid-migration. The checker then publishes the empty set and
// callers see sd.ErrNoEndpoints, which is a decision about correctness, not
// availability, and only the caller can make it.
func WithFailOpen(failOpen bool) Option {
	return func(c *Checker) {
		c.failOpen = failOpen
	}
}

// WithLogger sets the logger used for state transitions.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Checker) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// Checker is an Instancer that republishes its source's snapshot with
// unhealthy instances removed.
type Checker struct {
	source sd.Instancer
	probe  Probe
	cache  *instance.Cache
	logger *slog.Logger

	interval           time.Duration
	timeout            time.Duration
	unhealthyThreshold int
	healthyThreshold   int
	initiallyHealthy   bool
	failOpen           bool
	concurrency        int

	events chan sd.Event
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once

	mu       sync.Mutex
	snapshot []sd.Instance
	states   map[string]*state
}

type state struct {
	healthy bool
	// probed records whether this instance has produced a probe result yet.
	// "Not measured" and "measured and failing" must not be confused: the
	// fail-open in publish is about a broken probe, and an instance nobody has
	// probed yet is no evidence of that.
	probed    bool
	successes int
	failures  int
}

var _ sd.Instancer = (*Checker)(nil)

// Check probes every instance the source reports and republishes the ones that
// answer.
func Check(source sd.Instancer, probe Probe, options ...Option) *Checker {
	if source == nil {
		panic("health: nil instancer")
	}
	if probe == nil {
		panic("health: nil probe")
	}

	checker := &Checker{
		source:             source,
		probe:              probe,
		cache:              instance.NewCache(),
		logger:             slog.New(slog.DiscardHandler),
		interval:           DefaultInterval,
		timeout:            DefaultTimeout,
		unhealthyThreshold: DefaultUnhealthyThreshold,
		healthyThreshold:   DefaultHealthyThreshold,
		initiallyHealthy:   true,
		failOpen:           true,
		concurrency:        DefaultConcurrency,
		events:             make(chan sd.Event, 1),
		states:             make(map[string]*state),
	}
	for _, option := range options {
		if option != nil {
			option(checker)
		}
	}
	checker.ctx, checker.cancel = context.WithCancel(context.Background())

	initial := source.Register(checker.events)
	checker.accept(initial)

	checker.wg.Add(1)
	go func() {
		defer checker.wg.Done()
		checker.loop()
	}()
	return checker
}

// Register subscribes to the checked view of the instance set.
func (c *Checker) Register(ch chan sd.Event) sd.Event { return c.cache.Register(ch) }

// Deregister removes a subscriber.
func (c *Checker) Deregister(ch chan sd.Event) { c.cache.Deregister(ch) }

// Close stops probing and releases subscribers. It unsubscribes from the source
// but does not close it: whoever built the source owns it.
func (c *Checker) Close() error {
	c.once.Do(func() {
		c.cancel()
		c.source.Deregister(c.events)
		c.wg.Wait()
		_ = c.cache.Close()
	})
	return nil
}

func (c *Checker) loop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	events := c.events

	// Probe immediately so the first verdict does not wait a full interval.
	c.round()
	for {
		select {
		case <-c.ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				// A provider must not close a channel it was handed (see
				// sd.Instancer). One that does costs us discovery updates, but
				// receiving from a nil channel blocks forever, so probing and
				// publishing carry on with the last snapshot instead of
				// spinning on zero-value events.
				events = nil
				continue
			}
			c.accept(event)
		case <-ticker.C:
			c.round()
		}
	}
}

// accept records a new snapshot and republishes it. A discovery error is passed
// through untouched: it says nothing about instance health, and swallowing it
// would leave subscribers with a stale set and no explanation.
func (c *Checker) accept(event sd.Event) {
	if event.Err != nil {
		c.cache.Update(event)
		return
	}

	// The checker keeps this snapshot until the next event, so it has to own it.
	// sd.Instancer is a public extension point: a provider that reuses its
	// backing array between watches would otherwise rewrite the set the probe
	// rounds are walking. instance.Cache copies at the same boundary for the
	// same reason.
	instances := make([]sd.Instance, len(event.Instances))
	for i, item := range event.Instances {
		instances[i] = sd.Instance{Address: item.Address, Metadata: maps.Clone(item.Metadata)}
	}

	c.mu.Lock()
	c.snapshot = instances
	live := make(map[string]struct{}, len(instances))
	for _, target := range instances {
		live[target.Address] = struct{}{}
		if c.states[target.Address] == nil {
			c.states[target.Address] = &state{healthy: c.initiallyHealthy}
		}
	}
	for address := range c.states {
		if _, wanted := live[address]; !wanted {
			delete(c.states, address)
		}
	}
	c.mu.Unlock()

	c.publish()
}

func (c *Checker) round() {
	c.mu.Lock()
	targets := append([]sd.Instance(nil), c.snapshot...)
	c.mu.Unlock()
	if len(targets) == 0 {
		return
	}

	// A fixed pool rather than one goroutine per instance: with a large
	// service the difference is thousands of goroutines parked on a semaphore
	// every interval. Cancellation stops the feed, so Close does not wait for
	// probes that have not started.
	workers := c.concurrency
	if workers > len(targets) {
		workers = len(targets)
	}
	queue := make(chan sd.Instance)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range queue {
				ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
				c.record(target, c.probe(ctx, target))
				cancel()
			}
		}()
	}

feeding:
	for _, target := range targets {
		select {
		case queue <- target:
		case <-c.ctx.Done():
			break feeding
		}
	}
	close(queue)
	wg.Wait()

	if c.ctx.Err() != nil {
		return
	}
	c.publish()
}

func (c *Checker) record(target sd.Instance, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	current := c.states[target.Address]
	if current == nil {
		// The instance left discovery while its probe was in flight.
		return
	}
	current.probed = true

	if err != nil {
		current.successes = 0
		current.failures++
		if current.healthy && current.failures >= c.unhealthyThreshold {
			current.healthy = false
			c.logger.Warn("instance failed health checks",
				"address", target.Address, "failures", current.failures, "err", err)
		}
		return
	}

	current.failures = 0
	current.successes++
	if !current.healthy && current.successes >= c.healthyThreshold {
		current.healthy = true
		c.logger.Info("instance passed health checks",
			"address", target.Address, "successes", current.successes)
	}
}

func (c *Checker) publish() {
	c.mu.Lock()
	healthy := make([]sd.Instance, 0, len(c.snapshot))
	everyoneProbed := true
	for _, target := range c.snapshot {
		current := c.states[target.Address]
		if current == nil || !current.probed {
			everyoneProbed = false
		}
		if current != nil && current.healthy {
			healthy = append(healthy, target)
		}
	}
	total := len(c.snapshot)
	snapshot := append([]sd.Instance(nil), c.snapshot...)
	c.mu.Unlock()

	// Every probed instance failing usually means the probe itself is wrong — a
	// firewall, a path that moved — rather than every instance being down.
	// Publishing an empty set in that case turns a monitoring fault into an
	// outage, so the unchecked set is published instead. Same reasoning as the
	// ejection cap in sd/feedback. WithFailOpen(false) opts out, for callers
	// where reaching a dead instance is worse than reaching none.
	//
	// This requires every instance to have been probed. Otherwise
	// WithInitiallyHealthy(false) would defeat itself: at startup nothing is
	// healthy yet, and republishing the unchecked set is exactly the behaviour
	// that option exists to prevent.
	if total > 0 && len(healthy) == 0 && everyoneProbed && c.failOpen {
		c.logger.Warn("no instance passed health checks, publishing the unchecked set", "instances", total)
		c.cache.Update(sd.Event{Instances: snapshot})
		return
	}
	c.cache.Update(sd.Event{Instances: healthy})
}
