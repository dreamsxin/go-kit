// Package feedback contains process-local result accounting for service
// discovery balancers. It deliberately stays out of the discovery data path:
// outcomes are observed where calls execute, while registry snapshots remain
// static and cheap to distribute.
//
// Table is the single store of per-instance dynamic state. Least request reads
// its in-flight depth, Scored reads its latency and error rate, slow start reads
// when an instance was first seen, and Ejector reads all of them. Keeping one
// store means those policies cannot disagree about what is happening.
//
// Measure is the entry point. It binds a Table to a discovery subscription and
// hands out the balancers that read it, so a measurement-driven assembly that
// compiles is one that works: the strategy comes with its accounting, the
// accounting comes with its subscription, and an Ejector joins the subscription
// that is already there. Table, Follow, and Wrap remain for a caller assembling
// those pieces itself — recording outcomes observed somewhere no balancer can
// see them, for one.
package feedback

import (
	"context"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/internal/subscription"
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
//
// Reading is lock-free and writing is serialized. A selection reads every
// candidate, so the read path is the one that has to scale: the entry table is
// published copy-on-write and each measurement is an atomic field. Recording,
// resetting, and retaining take the mutex, which keeps the in-flight accounting
// and the retirement lifecycle exactly as ordered as they were.
type Table struct {
	mu    sync.Mutex
	alpha float64
	now   func() time.Time
	items atomic.Pointer[map[string]*entry]
}

// entry holds one address's measurements.
//
// The fields are atomic so a selection can read them without taking the table's
// mutex. Writes still happen under it, so a recorded outcome is never lost to a
// concurrent one. A reader can see fields from either side of one recording —
// a sample count from before and a latency from after — which is what a load
// heuristic can afford: the next selection reads again a moment later.
type entry struct {
	samples   atomic.Uint64
	latencyNS atomic.Uint64 // float64 bits
	errorRate atomic.Uint64 // float64 bits
	bytes     atomic.Int64
	// firstSeen is Unix nanoseconds, or zero for an instance that has none.
	firstSeen atomic.Int64
	// generation counts how many times Reset has cleared this entry. A call
	// tracked before a Reset carries the older generation and its result is
	// discarded, because Reset means "forget what happened before now".
	generation atomic.Uint64
	// retired marks an address Retain wanted to drop but could not, because a
	// call was still in flight. The last completion deletes it. It is read and
	// written under Table.mu only.
	retired bool

	// inflight is incremented and decremented under Table.mu, so Retain cannot
	// observe zero for a call that is about to start.
	inflight atomic.Int64
}

// NewTable creates an empty feedback table.
func NewTable(options ...Option) *Table {
	table := &Table{alpha: 0.2, now: time.Now}
	items := map[string]*entry{}
	table.items.Store(&items)
	for _, option := range options {
		if option != nil {
			option(table)
		}
	}
	return table
}

// entries returns the published entry table. It is read-only: every mutation
// publishes a new map under Table.mu.
func (t *Table) entries() map[string]*entry {
	if items := t.items.Load(); items != nil {
		return *items
	}
	return nil
}

// entryForLocked must be called with Table.mu held. Admitting an address
// publishes a new map, so a concurrent reader sees either the old table or the
// new one and never a half-built map.
func (t *Table) entryForLocked(address string) *entry {
	current := t.entries()
	if item := current[address]; item != nil {
		return item
	}
	item := &entry{}
	item.firstSeen.Store(t.now().UnixNano())
	next := make(map[string]*entry, len(current)+1)
	for name, existing := range current {
		next[name] = existing
	}
	next[address] = item
	t.items.Store(&next)
	return item
}

// deleteLocked must be called with Table.mu held.
func (t *Table) deleteLocked(address string) {
	current := t.entries()
	if _, ok := current[address]; !ok {
		return
	}
	next := make(map[string]*entry, len(current))
	for name, existing := range current {
		if name != address {
			next[name] = existing
		}
	}
	t.items.Store(&next)
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
	if item.samples.Load() == 0 {
		storeFloat(&item.latencyNS, float64(outcome.Latency))
		storeFloat(&item.errorRate, failed)
	} else {
		storeFloat(&item.latencyNS, t.alpha*float64(outcome.Latency)+(1-t.alpha)*loadFloat(&item.latencyNS))
		storeFloat(&item.errorRate, t.alpha*failed+(1-t.alpha)*loadFloat(&item.errorRate))
	}
	item.samples.Add(1)
	item.bytes.Add(outcome.Bytes)
}

func storeFloat(target *atomic.Uint64, value float64) {
	target.Store(math.Float64bits(value))
}

func loadFloat(source *atomic.Uint64) float64 {
	return math.Float64frombits(source.Load())
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
	return item, item.generation.Load()
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
	if item.generation.Load() == generation {
		t.recordLocked(item, outcome)
	}

	if item.inflight.Add(-1) > 0 || !item.retired {
		return
	}
	// Compare identities: a re-registered address may already own a new entry.
	if t.entries()[address] == item {
		t.deleteLocked(address)
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
	item := t.entries()[instance.Address]
	if item == nil {
		return
	}
	item.samples.Store(0)
	item.latencyNS.Store(0)
	item.errorRate.Store(0)
	item.bytes.Store(0)
	item.generation.Add(1)
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
	for address, item := range t.entries() {
		if _, wanted := keep[address]; wanted {
			continue
		}
		if item.inflight.Load() > 0 {
			item.retired = true
			continue
		}
		t.deleteLocked(address)
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
// Follow is the primitive. Measure is the assembly: it builds the table, follows
// it, and hands out balancers that read it, which is the path that cannot be
// wired up wrong.
//
// A view derived from another Instancer — active health checking, for one — is
// resolved to the Instancer it derives from through sd.DerivedInstancer. A
// checker withdraws an instance it considers unhealthy, and to a retainer a
// withdrawal is indistinguishable from deregistration: Ejector.Retain drops the
// ejection state, so the instance returns with a clean record the moment probing
// recovers, and Table.Retain drops the measurements that ejected it. Active and
// passive health checking would cancel each other out. Following registration
// instead means a retainer only forgets an instance that has actually left the
// service.
func Follow(instancer sd.Instancer, retainers ...Retainer) io.Closer {
	return follow(instancer, retainers...)
}

// follower is the subscription behind Follow and Measure. A retainer can join
// after it starts, which is what lets an Ejector share the subscription the
// Table it reads already has instead of opening a second one that could report a
// different snapshot.
type follower struct {
	feed *subscription.Feed

	mtx       sync.Mutex
	retainers []Retainer
	snapshot  []sd.Instance
	known     bool
}

func follow(instancer sd.Instancer, retainers ...Retainer) *follower {
	if instancer == nil {
		panic("feedback: nil instancer")
	}
	f := &follower{retainers: make([]Retainer, 0, len(retainers))}
	for _, retainer := range retainers {
		if retainer != nil {
			f.retainers = append(f.retainers, retainer)
		}
	}
	f.feed = subscription.Start(registrations(instancer), f.update)
	return f
}

// registrations resolves a derived view to the Instancer it derives from, so
// what a retainer follows is registration rather than a filtered verdict.
func registrations(instancer sd.Instancer) sd.Instancer {
	for {
		derived, ok := instancer.(sd.DerivedInstancer)
		if !ok {
			return instancer
		}
		source := derived.Underlying()
		// A view that reports no source, or itself, is as far as this goes.
		if source == nil || source == instancer {
			return instancer
		}
		instancer = source
	}
}

func (f *follower) update(event sd.Event) {
	// A failed snapshot says nothing about which instances exist, so it must not
	// be treated as "everything else is gone".
	if event.Err != nil {
		return
	}
	f.mtx.Lock()
	f.snapshot = event.Instances
	f.known = true
	retainers := append([]Retainer(nil), f.retainers...)
	f.mtx.Unlock()
	// Outside the lock: a retainer walks its own state, and add must not wait on
	// it to publish the snapshot to a newcomer.
	for _, retainer := range retainers {
		retainer.Retain(event.Instances)
	}
}

// add registers a retainer and hands it the snapshot already seen, so one that
// joins after the first event is not left waiting for the next one — which, for
// a service whose instance set never changes again, is never.
func (f *follower) add(retainer Retainer) {
	if retainer == nil {
		return
	}
	f.mtx.Lock()
	f.retainers = append(f.retainers, retainer)
	snapshot, known := f.snapshot, f.known
	f.mtx.Unlock()
	if known {
		retainer.Retain(snapshot)
	}
}

func (f *follower) Close() error {
	f.feed.Stop()
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
//
// It takes no lock: a selection asks for every candidate, and making that wait
// on the recording path is what turns a load heuristic into a bottleneck.
func (t *Table) Stats(instance sd.Instance) Stats {
	if t == nil {
		return Stats{}
	}
	item := t.entries()[instance.Address]
	if item == nil {
		return Stats{}
	}
	return statsOf(item)
}

func statsOf(item *entry) Stats {
	stats := Stats{
		Samples:   item.samples.Load(),
		Latency:   time.Duration(loadFloat(&item.latencyNS)),
		ErrorRate: loadFloat(&item.errorRate),
		Bytes:     item.bytes.Load(),
		InFlight:  item.inflight.Load(),
	}
	if nanos := item.firstSeen.Load(); nanos != 0 {
		stats.FirstSeen = time.Unix(0, nanos)
	}
	return stats
}

// score rates an instance from what this table measured. Higher is better.
//
// The formula is (1-errorRate) / (1 + latencyMilliseconds + inFlight): errors
// scale the score down, while latency and local concurrency are summed, which
// treats one millisecond of latency as equivalent to one call in flight. That
// equivalence is a default, not a law. A caller who wants different weights reads
// Stats and writes its own selector.ScoreFunc, which balancer.NewScored takes.
//
// It is unexported because a bare score function is the one shape of this that
// does not work: handed to selector.Scored without Wrap, nothing records into the
// table, every instance scores the same, and selection degrades to random. Scored
// and Measured.Scored are the two ways to spend it, and both wrap.
//
// An instance with no samples yet scores the maximum 1: no errors, no latency,
// nothing in flight. That is intentional — an unmeasured instance has to receive
// calls before it can be measured — and it is self-limiting under Wrap, because
// in-flight rises at Pick and drops the score before the first outcome arrives.
// To ramp a cold instance more gently than that, see Measured.SlowStartWeighted.
func (t *Table) score() selector.ScoreFunc {
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

// load reports the in-flight depth of an instance, for selector.LeastRequest.
func (t *Table) load() selector.LoadFunc {
	return func(instance sd.Instance) int64 { return t.Stats(instance).InFlight }
}

// firstSeen reports when each instance entered the table, for selector.SlowStart.
//
// It is unexported because it is only meaningful on a table that follows
// discovery: without that, an instance is unknown until its first call, slow
// start treats unknown as brand new, and every weight collapses to 1 — uniform
// selection wearing a ramp's name. Measured.SlowStartWeighted is the assembly
// that has the subscription by construction.
func (t *Table) firstSeen() selector.FirstSeenFunc {
	return func(instance sd.Instance) (time.Time, bool) {
		stats := t.Stats(instance)
		return stats.FirstSeen, !stats.FirstSeen.IsZero()
	}
}

// LeastRequest is the assembled least-request strategy: it reads this table's
// in-flight depth and records into the same table, so picking and accounting
// cannot drift apart.
func (t *Table) LeastRequest(options ...selector.LeastRequestOption) selector.Strategy {
	return t.Wrap(selector.LeastRequest(t.load(), options...))
}

// Scored is the assembled scored strategy: it rates each instance by what this
// table measured — see the score formula on Stats — and records into the same
// table, so the scores it reads are the scores its own traffic produced.
//
// For a load signal this process did not measure itself — a report pushed by the
// instances, ORCA or LRS style out-of-band reporting — balancer.NewScored takes
// the caller's own selector.ScoreFunc and needs no table.
func (t *Table) Scored() selector.Strategy {
	return t.Wrap(selector.Scored(t.score()))
}

// Wrap augments a strategy with local accounting. The selected strategy's
// callback, when present, runs before the table records the same outcome.
//
// Wrapping a strategy this table already wraps returns it unchanged, because
// doubling the layer would count one call as two in flight — the misreading a
// measured strategy exists to prevent. `measured.Balancer(set, table.Scored())`
// is the reachable way to ask for that, and it is now harmless. The check is one
// level deep and by identity: a wrapper buried under another decorator is not
// found, and a different table's wrapper is left alone, since two tables
// deliberately recording the same traffic is a composition rather than a
// mistake.
//
// Close forwards to strategy, so a strategy that owns something still gets
// closed through this layer. The table itself holds no goroutine and needs no
// closing; a table that follows discovery is closed through the Follow closer,
// or through Measured.Close.
func (t *Table) Wrap(strategy selector.Strategy) selector.Strategy {
	if strategy == nil {
		panic("feedback: nil strategy")
	}
	if already, ok := strategy.(*wrapped); ok && already.table == t {
		return already
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
