// Package feedback contains process-local result accounting for service
// discovery balancers. It deliberately stays out of the discovery data path:
// outcomes are observed where calls execute, while registry snapshots remain
// static and cheap to distribute.
package feedback

import (
	"context"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// Option configures a Table.
type Option func(*Table)

// WithAlpha sets the EWMA smoothing factor. Values outside (0, 1] are
// ignored. The default 0.2 reacts quickly enough for endpoint failures while
// retaining a useful latency baseline.
func WithAlpha(alpha float64) Option {
	return func(t *Table) {
		if alpha > 0 && alpha <= 1 {
			t.alpha = alpha
		}
	}
}

// Table stores local EWMA latency/error measurements and in-flight calls by
// instance address. Addresses are the stable identity exposed by sd.Instance.
//
// Measurements are retained until they are dropped, and instance addresses
// churn: a rolling deployment replaces every one of them. A long-lived table
// should therefore follow the discovery snapshot — see Follow — or be told what
// to keep with Retain. Without either, the table holds one entry per address it
// has ever observed.
type Table struct {
	mu    sync.RWMutex
	alpha float64
	items map[string]*entry
}

type entry struct {
	// samples, latencyNS, errorRate and bytes are guarded by Table.mu.
	samples   uint64
	latencyNS float64
	errorRate float64
	bytes     int64

	// inflight is read and written outside the lock, including by callbacks
	// that outlive a Retain, so it carries its own synchronisation.
	inflight atomic.Int64
}

// NewTable creates an empty feedback table.
func NewTable(options ...Option) *Table {
	table := &Table{alpha: 0.2, items: make(map[string]*entry)}
	for _, option := range options {
		if option != nil {
			option(table)
		}
	}
	return table
}

func (t *Table) entryFor(address string) *entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.entryForLocked(address)
}

func (t *Table) entryForLocked(address string) *entry {
	item := t.items[address]
	if item == nil {
		item = &entry{}
		t.items[address] = item
	}
	return item
}

// Observe records one completed call for instance. It is safe to call
// directly, but callers that want in-flight accounting should use Track.
func (t *Table) Observe(instance sd.Instance, outcome sd.Outcome) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	item := t.entryForLocked(instance.Address)
	failed := 0.0
	if outcome.Err != nil {
		failed = 1
	}

	// The first sample seeds the EWMA; subsequent samples decay older data.
	if item.samples == 0 {
		item.latencyNS = float64(outcome.Latency)
		item.errorRate = failed
	} else {
		item.latencyNS = t.alpha*float64(outcome.Latency) + (1-t.alpha)*item.latencyNS
		item.errorRate = t.alpha*failed + (1-t.alpha)*item.errorRate
	}
	item.samples++
	item.bytes += outcome.Bytes
}

// Track marks a call in flight and returns the callback to pass as
// sd.Picked.Done. The callback is idempotent, so deferring it and explicit
// completion can safely coexist in adapters.
func (t *Table) Track(instance sd.Instance) sd.Done {
	if t == nil {
		return func(sd.Outcome) {}
	}
	item := t.entryFor(instance.Address)
	item.inflight.Add(1)
	var once sync.Once
	return func(outcome sd.Outcome) {
		once.Do(func() {
			t.Observe(instance, outcome)
			item.inflight.Add(-1)
		})
	}
}

// Retain drops measurements for addresses outside instances, which is what
// keeps a long-running table the size of the service rather than the size of
// its deployment history. Entries with calls still in flight are kept until
// those calls report.
//
// Pass the discovery snapshot, never a filtered candidate set: dropping an
// instance that a health filter just ejected would erase the very measurements
// that ejected it and admit it again.
//
// Retain walks the table, so call it when discovery changes — Follow does
// exactly that — rather than on the selection path. Comparing sizes first would
// be cheaper but wrong: a rolling deployment replaces addresses one for one, so
// a stale table can be exactly as large as the live snapshot.
func (t *Table) Retain(instances []sd.Instance) {
	if t == nil {
		return
	}
	keep := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		keep[instance.Address] = struct{}{}
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	for address, item := range t.items {
		if _, wanted := keep[address]; wanted {
			continue
		}
		if item.inflight.Load() > 0 {
			continue
		}
		delete(t.items, address)
	}
}

// Follow keeps the table aligned with an Instancer's snapshots. The returned
// Closer unsubscribes; it does not close the Instancer.
func (t *Table) Follow(instancer sd.Instancer) io.Closer {
	if t == nil {
		panic("feedback: nil table")
	}
	if instancer == nil {
		panic("feedback: nil instancer")
	}

	events := make(chan sd.Event, 1)
	initial := instancer.Register(events)
	if initial.Err == nil {
		t.Retain(initial.Instances)
	}

	follower := &follower{instancer: instancer, events: events, done: make(chan struct{})}
	follower.wg.Add(1)
	go func() {
		defer follower.wg.Done()
		for {
			select {
			case <-follower.done:
				return
			case event := <-events:
				// A failed snapshot says nothing about which instances exist,
				// so it must not be treated as "everything else is gone".
				if event.Err == nil {
					t.Retain(event.Instances)
				}
			}
		}
	}()
	return follower
}

type follower struct {
	instancer sd.Instancer
	events    chan sd.Event
	done      chan struct{}
	wg        sync.WaitGroup
	once      sync.Once
}

func (f *follower) Close() error {
	f.once.Do(func() {
		f.instancer.Deregister(f.events)
		close(f.done)
		f.wg.Wait()
	})
	return nil
}

// Stats is a point-in-time copy of one instance's feedback.
type Stats struct {
	Samples   uint64
	Latency   time.Duration
	ErrorRate float64
	Bytes     int64
	InFlight  int64
}

// Stats returns the current measurements for instance. An unknown instance
// has zero values and is considered healthy by Healthy.
func (t *Table) Stats(instance sd.Instance) Stats {
	if t == nil {
		return Stats{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	item := t.items[instance.Address]
	if item == nil {
		return Stats{}
	}
	return statsOf(item)
}

// statsOf must be called with Table.mu held.
func statsOf(item *entry) Stats {
	return Stats{
		Samples:   item.samples,
		Latency:   time.Duration(item.latencyNS),
		ErrorRate: item.errorRate,
		Bytes:     item.bytes,
		InFlight:  item.inflight.Load(),
	}
}

// Score returns a selector score function. Higher scores are better.
//
// The formula is (1-errorRate) / (1 + latencyMilliseconds + inFlight): errors
// scale the score down, while latency and local concurrency are summed, which
// treats one millisecond of latency as equivalent to one call in flight. That
// equivalence is a default, not a law. A caller who wants different weights
// reads Stats and writes its own selector.ScoreFunc.
func (t *Table) Score() selector.ScoreFunc {
	return func(instance sd.Instance) (float64, bool) {
		stats := t.Stats(instance)
		latencyMS := float64(stats.Latency) / float64(time.Millisecond)
		score := (1 - stats.ErrorRate) / (1 + latencyMS + float64(stats.InFlight))
		if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 {
			score = 0
		}
		return score, true
	}
}

// Load returns the in-flight depth of an instance, for
// selector.LeastRequest.
func (t *Table) Load() selector.LoadFunc {
	return func(instance sd.Instance) int64 { return t.Stats(instance).InFlight }
}

// LeastRequest is the assembled least-request strategy: it reads this table's
// in-flight depth and records into the same table, so picking and accounting
// cannot drift apart.
func (t *Table) LeastRequest(options ...selector.LeastRequestOption) selector.Strategy {
	return t.Wrap(selector.LeastRequest(t.Load(), options...))
}

// HealthPolicy controls passive endpoint exclusion. A zero threshold disables
// that check. MinSamples prevents one transient result from ejecting an
// otherwise unknown endpoint.
type HealthPolicy struct {
	MaxErrorRate float64
	MaxLatency   time.Duration
	MaxInFlight  int64
	MinSamples   uint64
	// MaxEjectionPercent caps passive ejection, defaulting to
	// DefaultMaxEjectionPercent. When more than this share of the candidates
	// looks unhealthy, no candidate is ejected: a pool that is failing as a
	// whole is more likely to mean a shared dependency, or a threshold set too
	// tight, than a majority of bad instances. Envoy calls this panic mode.
	MaxEjectionPercent int
}

// DefaultMaxEjectionPercent is the share of candidates passive health checking
// will remove before it gives up and admits all of them.
const DefaultMaxEjectionPercent = 50

// Healthy returns a filter that passively excludes candidates whose local
// measurements exceed policy.
//
// It is a set filter rather than a per-instance predicate because the ejection
// cap is a decision about the set: whether one instance may be removed depends
// on how many others are also failing. Feed it to selector.Filtered.
func (t *Table) Healthy(policy HealthPolicy) sd.InstanceFilter {
	maxEjection := policy.MaxEjectionPercent
	if maxEjection <= 0 {
		maxEjection = DefaultMaxEjectionPercent
	}
	if maxEjection > 100 {
		maxEjection = 100
	}

	return func(_ context.Context, instances []sd.Instance) []sd.Instance {
		healthy := make([]sd.Instance, 0, len(instances))
		for _, instance := range instances {
			if !t.unhealthy(instance, policy) {
				healthy = append(healthy, instance)
			}
		}
		if len(healthy) == len(instances) {
			return instances
		}
		// The cap is measured against the candidates in hand, not against every
		// address the table remembers: instances that have since been removed
		// from discovery must not decide whether a live one can be ejected.
		if (len(instances)-len(healthy))*100 > len(instances)*maxEjection {
			return instances
		}
		return healthy
	}
}

func (t *Table) unhealthy(instance sd.Instance, policy HealthPolicy) bool {
	stats := t.Stats(instance)
	if stats.Samples < policy.MinSamples {
		return false
	}
	if policy.MaxErrorRate > 0 && stats.ErrorRate > policy.MaxErrorRate {
		return true
	}
	if policy.MaxLatency > 0 && stats.Latency > policy.MaxLatency {
		return true
	}
	return policy.MaxInFlight > 0 && stats.InFlight > policy.MaxInFlight
}

// Wrap augments a strategy with local accounting. The selected strategy's
// callback, when present, runs before the table records the same outcome.
func (t *Table) Wrap(strategy selector.Strategy) selector.Strategy {
	if strategy == nil {
		panic("feedback: nil strategy")
	}
	return wrapped{table: t, strategy: strategy}
}

type wrapped struct {
	table    *Table
	strategy selector.Strategy
}

func (w wrapped) Pick(ctx context.Context, request any, instances []sd.Instance) (int, sd.Done, error) {
	index, strategyDone, err := w.strategy.Pick(ctx, request, instances)
	if err != nil {
		return 0, nil, err
	}
	if index < 0 || index >= len(instances) {
		return 0, nil, sd.ErrNoEndpoints
	}
	tableDone := w.table.Track(instances[index])
	return index, func(outcome sd.Outcome) {
		if strategyDone != nil {
			strategyDone(outcome)
		}
		tableDone(outcome)
	}, nil
}
