// Package feedback contains process-local result accounting for service
// discovery balancers. It deliberately stays out of the discovery data path:
// outcomes are observed where calls execute, while registry snapshots remain
// static and cheap to distribute.
//
// Table is the single store of per-instance dynamic state. Least request reads
// its in-flight depth, Scored reads its latency and error rate, slow start reads
// when an instance was first seen, and Ejector reads all of them. Keeping one
// store means those policies cannot disagree about what is happening.
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

// WithClock replaces the time source, so tests can assert time-dependent
// behaviour without sleeping.
func WithClock(now func() time.Time) Option {
	return func(t *Table) {
		if now != nil {
			t.now = now
		}
	}
}

// Table stores local EWMA latency/error measurements, transferred bytes and
// in-flight calls by instance address. Addresses are the stable identity
// exposed by sd.Instance.
//
// Measurements are retained until they are dropped, and instance addresses
// churn: a rolling deployment replaces every one of them. A long-lived table
// should therefore follow the discovery snapshot — see Follow — or be told what
// to keep with Retain. Without either, the table holds one entry per address it
// has ever observed.
type Table struct {
	mu    sync.RWMutex
	alpha float64
	now   func() time.Time
	items map[string]*entry
}

type entry struct {
	// samples, latencyNS, errorRate, bytes, firstSeen, generation and retired
	// are guarded by Table.mu.
	samples   uint64
	latencyNS float64
	errorRate float64
	bytes     int64
	firstSeen time.Time
	// generation counts how many times Reset has cleared this entry. A call
	// tracked before a Reset carries the older generation and its result is
	// discarded, because Reset means "forget what happened before now".
	generation uint64
	// retired marks an address Retain wanted to drop but could not, because a
	// call was still in flight. The last completion deletes it.
	retired bool

	// inflight is incremented and decremented under Table.mu, so Retain cannot
	// observe zero for a call that is about to start. It stays atomic because
	// Stats reads it under the read lock.
	inflight atomic.Int64
}

// NewTable creates an empty feedback table.
func NewTable(options ...Option) *Table {
	table := &Table{alpha: 0.2, now: time.Now, items: make(map[string]*entry)}
	for _, option := range options {
		if option != nil {
			option(table)
		}
	}
	return table
}

func (t *Table) entryForLocked(address string) *entry {
	item := t.items[address]
	if item == nil {
		item = &entry{firstSeen: t.now()}
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
	t.recordLocked(t.entryForLocked(instance.Address), outcome)
}

// recordLocked must be called with Table.mu held.
func (t *Table) recordLocked(item *entry, outcome sd.Outcome) {
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
	item, generation := t.begin(instance.Address)
	var once sync.Once
	return func(outcome sd.Outcome) {
		once.Do(func() { t.complete(instance.Address, item, generation, outcome) })
	}
}

// begin claims the entry and the in-flight slot in one critical section. Doing
// the two separately let Retain see an in-flight count of zero for a call that
// had already picked its entry, and delete the entry underneath it.
func (t *Table) begin(address string) (*entry, uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	item := t.entryForLocked(address)
	item.inflight.Add(1)
	return item, item.generation
}

// complete records the result, releases the in-flight slot, and drops an
// address that Retain marked as gone. Without the last part, an instance that
// leaves discovery mid-call keeps its entry until the next snapshot arrives —
// and if the set never changes again, forever.
func (t *Table) complete(address string, item *entry, generation uint64, outcome sd.Outcome) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// A result from before a Reset is exactly what Reset discarded. Recording
	// it would reverse the recovery: Reset zeroes the sample count, so the
	// stale result seeds the average instead of decaying into it, and one
	// straggling failure would re-eject the instance at full weight.
	if item.generation == generation {
		t.recordLocked(item, outcome)
	}

	if item.inflight.Add(-1) > 0 || !item.retired {
		return
	}
	// Compare identities: a re-registered address may already own a new entry.
	if t.items[address] == item {
		delete(t.items, address)
	}
}

// Reset discards the measurements for one instance while keeping the instance
// known: its first-seen time and in-flight count survive.
//
// This is what makes ejection reversible. A decayed average does not recover on
// its own once traffic stops, so returning an instance to service without
// clearing what got it ejected would eject it again on the next selection.
//
// Calls already in flight belong to the discarded period, so their results are
// dropped when they complete rather than recorded against the fresh state.
func (t *Table) Reset(instance sd.Instance) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	item := t.items[instance.Address]
	if item == nil {
		return
	}
	item.samples = 0
	item.latencyNS = 0
	item.errorRate = 0
	item.bytes = 0
	item.generation++
}

// Retain aligns the table with the discovery snapshot. Addresses in the
// snapshot that the table has not seen are registered with their arrival time,
// which is what makes slow start ramp from the moment an instance joined the
// service rather than from its first call: an instance nobody has called yet
// would otherwise be unknown, and slow start treats unknown as brand new
// forever. Addresses outside the snapshot are dropped, which keeps a
// long-running table the size of the service rather than the size of its
// deployment history. An address with calls still in flight is marked instead,
// and its last completion deletes it, so the lifecycle closes even if no
// further snapshot ever arrives.
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
	for _, instance := range instances {
		// Records the arrival time for a new address, keeps the existing one
		// for an address already known. A re-registered address is live again,
		// so an earlier retirement must not outlive it.
		t.entryForLocked(instance.Address).retired = false
	}
	for address, item := range t.items {
		if _, wanted := keep[address]; wanted {
			continue
		}
		if item.inflight.Load() > 0 {
			item.retired = true
			continue
		}
		delete(t.items, address)
	}
}

// Retainer is anything holding per-instance state that has to be released when
// an instance leaves discovery. Table and Ejector both qualify.
type Retainer interface {
	Retain(instances []sd.Instance)
}

// Follow keeps per-instance state aligned with an Instancer's snapshots. One
// subscription serves every retainer, so a table and the ejector reading it
// cannot drift apart. The returned Closer unsubscribes; it does not close the
// Instancer.
//
// Pass the raw Instancer, not a health.Check decorating it. A checker withdraws
// an instance it considers unhealthy, and to a retainer a withdrawal is
// indistinguishable from deregistration: Ejector.Retain drops the ejection
// state, so the instance returns with a clean record the moment probing
// recovers, and Table.Retain drops the measurements that ejected it. Active and
// passive health checking would cancel each other out. Following registration
// instead means a retainer only forgets an instance that has actually left the
// service.
func Follow(instancer sd.Instancer, retainers ...Retainer) io.Closer {
	if instancer == nil {
		panic("feedback: nil instancer")
	}
	kept := make([]Retainer, 0, len(retainers))
	for _, retainer := range retainers {
		if retainer != nil {
			kept = append(kept, retainer)
		}
	}

	events := make(chan sd.Event, 1)
	initial := instancer.Register(events)
	if initial.Err == nil {
		retain(kept, initial.Instances)
	}

	follower := &follower{instancer: instancer, events: events, done: make(chan struct{})}
	follower.wg.Add(1)
	go func() {
		defer follower.wg.Done()
		for {
			select {
			case <-follower.done:
				return
			case event, ok := <-events:
				// A provider must not close a channel it was handed (see
				// sd.Instancer), but this loop is not the place to spin if one
				// does.
				if !ok {
					return
				}
				// A failed snapshot says nothing about which instances exist,
				// so it must not be treated as "everything else is gone".
				if event.Err == nil {
					retain(kept, event.Instances)
				}
			}
		}
	}()
	return follower
}

func retain(retainers []Retainer, instances []sd.Instance) {
	for _, retainer := range retainers {
		retainer.Retain(instances)
	}
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
	// FirstSeen is when this instance entered the table, which is what slow
	// start ramps against. A table that follows discovery — see Follow — is
	// told about an instance when it registers, so this is its arrival time; a
	// table driven only by calls learns of it on its first call instead. It is
	// zero for an instance the table has never seen.
	FirstSeen time.Time
}

// Stats returns the current measurements for instance. An unknown instance has
// zero values.
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
		FirstSeen: item.firstSeen,
	}
}

// Score returns a selector score function. Higher scores are better.
//
// The formula is (1-errorRate) / (1 + latencyMilliseconds + inFlight): errors
// scale the score down, while latency and local concurrency are summed, which
// treats one millisecond of latency as equivalent to one call in flight. That
// equivalence is a default, not a law. A caller who wants different weights
// reads Stats and writes its own selector.ScoreFunc.
//
// The score is only as good as what the table was told, so wrap the strategy
// that consumes it:
//
//	strategy := table.Wrap(selector.Scored(table.Score()))
//
// Without Wrap nothing records into the table, every instance scores the same,
// and Scored degrades to a random pick. An instance with no samples yet scores
// the maximum 1: no errors, no latency, nothing in flight. That is intentional —
// an unmeasured instance has to receive calls before it can be measured — and it
// is self-limiting under Wrap, because in-flight rises at Pick and drops the
// score before the first outcome arrives. To ramp a cold instance more gently
// than that, weight it with selector.SlowStart over Table.FirstSeen.
func (t *Table) Score() selector.ScoreFunc {
	return func(_ context.Context, _ any, instance sd.Instance) (float64, bool) {
		stats := t.Stats(instance)
		latencyMS := float64(stats.Latency) / float64(time.Millisecond)
		score := (1 - stats.ErrorRate) / (1 + latencyMS + float64(stats.InFlight))
		// The denominator is at least 1 and the numerator is bounded, so only a
		// corrupted stat could reach these branches. They are kept so a bad
		// score can never propagate into a comparison.
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

// FirstSeen returns when each instance entered the table, for
// selector.SlowStart. Follow the discovery snapshot so this is the instance's
// arrival time; without that, an instance is unknown until its first call and
// slow start treats unknown as brand new, so the ramp never starts. Instances
// the table has not seen report false.
func (t *Table) FirstSeen() selector.FirstSeenFunc {
	return func(instance sd.Instance) (time.Time, bool) {
		stats := t.Stats(instance)
		return stats.FirstSeen, !stats.FirstSeen.IsZero()
	}
}

// LeastRequest is the assembled least-request strategy: it reads this table's
// in-flight depth and records into the same table, so picking and accounting
// cannot drift apart.
func (t *Table) LeastRequest(options ...selector.LeastRequestOption) selector.Strategy {
	return t.Wrap(selector.LeastRequest(t.Load(), options...))
}

// Wrap augments a strategy with local accounting. The selected strategy's
// callback, when present, runs before the table records the same outcome.
//
// Close forwards to strategy, so a strategy that owns something still gets
// closed through this layer. The table itself holds no goroutine and needs no
// closing; a table that follows discovery is closed through the Follow closer.
func (t *Table) Wrap(strategy selector.Strategy) selector.Strategy {
	if strategy == nil {
		panic("feedback: nil strategy")
	}
	return &wrapped{table: t, strategy: strategy}
}

type wrapped struct {
	table     *Table
	strategy  selector.Strategy
	closeOnce sync.Once
	closeErr  error
}

// Close releases the wrapped strategy. It is idempotent.
func (w *wrapped) Close() error {
	w.closeOnce.Do(func() {
		w.closeErr = selector.CloseStrategy(w.strategy)
	})
	return w.closeErr
}

func (w *wrapped) Pick(ctx context.Context, request any, instances []sd.Instance) (int, sd.Done, error) {
	index, strategyDone, err := w.strategy.Pick(ctx, request, instances)
	if err != nil {
		return 0, nil, err
	}
	if index < 0 || index >= len(instances) {
		sd.Release(strategyDone, sd.ErrNoEndpoints)
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
