package feedback

import (
	"context"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// Defaults for passive ejection. The ejection window mirrors Envoy's outlier
// detection: a first offence is short, and repeat offenders are held out for
// longer, up to a cap.
const (
	DefaultBaseEjectionDuration = 30 * time.Second
	DefaultMaxEjectionDuration  = 5 * time.Minute
	DefaultMaxEjectionPercent   = 50
)

// EjectionPolicy decides when an instance stops being a candidate.
//
// The thresholds are all opt-in: a zero MaxErrorRate, MaxLatency or MaxInFlight
// disables that check, so a policy states only what it actually cares about.
// MinSamples keeps one unlucky call from ejecting an instance nothing is known
// about yet.
type EjectionPolicy struct {
	MaxErrorRate float64
	MaxLatency   time.Duration
	MaxInFlight  int64
	MinSamples   uint64

	// BaseDuration is how long a first ejection lasts; each consecutive
	// ejection of the same instance doubles it, up to MaxDuration. Ejection has
	// to expire, because an instance receiving no traffic produces no new
	// measurements, so nothing would ever clear the ones that ejected it.
	BaseDuration time.Duration
	MaxDuration  time.Duration

	// MaxEjectionPercent caps how much of the candidate set may be removed,
	// defaulting to DefaultMaxEjectionPercent. A pool failing as a whole is more
	// likely to mean a shared dependency, or a threshold set too tight, than a
	// majority of bad instances. Envoy calls this panic mode.
	MaxEjectionPercent int
}

// EjectorOption configures an Ejector.
type EjectorOption func(*Ejector)

// WithEjectorClock replaces the time source, so tests can advance through an
// ejection window without sleeping.
func WithEjectorClock(now func() time.Time) EjectorOption {
	return func(e *Ejector) {
		if now != nil {
			e.now = now
		}
	}
}

// Ejector removes instances whose measured behaviour breaks a policy, and
// returns them to service when the ejection expires.
//
// It is separate from Table because ejection state belongs to a policy, not to
// an instance: two ejectors with different thresholds can read one table without
// overwriting each other's decisions. The table stays a scoreboard.
type Ejector struct {
	table  *Table
	policy EjectionPolicy
	now    func() time.Time

	mu    sync.Mutex
	state map[string]*ejection
}

type ejection struct {
	until time.Time
	// count is how many times this instance has been ejected, which drives the
	// backoff. It deliberately survives a clean pass: an instance that fails
	// again right after returning is flapping, and should be held out longer
	// than a first offender. Retain clears it when the instance leaves
	// discovery.
	count int
}

// NewEjector reads measurements from table and applies policy to them.
func NewEjector(table *Table, policy EjectionPolicy, options ...EjectorOption) *Ejector {
	if table == nil {
		panic("feedback: nil table")
	}
	if policy.BaseDuration <= 0 {
		policy.BaseDuration = DefaultBaseEjectionDuration
	}
	if policy.MaxDuration < policy.BaseDuration {
		policy.MaxDuration = max(DefaultMaxEjectionDuration, policy.BaseDuration)
	}
	if policy.MaxEjectionPercent <= 0 {
		policy.MaxEjectionPercent = DefaultMaxEjectionPercent
	}
	if policy.MaxEjectionPercent > 100 {
		policy.MaxEjectionPercent = 100
	}

	ejector := &Ejector{
		table:  table,
		policy: policy,
		now:    time.Now,
		state:  make(map[string]*ejection),
	}
	for _, option := range options {
		if option != nil {
			option(ejector)
		}
	}
	return ejector
}

// Filter narrows a candidate set to the instances that are currently in
// service. Pass it to selector.Filtered.
func (e *Ejector) Filter() sd.InstanceFilter {
	return func(_ context.Context, instances []sd.Instance) []sd.Instance {
		return e.apply(instances)
	}
}

// Ejected reports whether instance is currently held out of service, for
// logging and health endpoints.
func (e *Ejector) Ejected(instance sd.Instance) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	state := e.state[instance.Address]
	return state != nil && e.now().Before(state.until)
}

// Retain drops ejection state for addresses that have left discovery, so the
// ejector does not outgrow the service it guards. It implements Retainer, so one
// Follow call can drive both the table and the ejector.
func (e *Ejector) Retain(instances []sd.Instance) {
	keep := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		keep[instance.Address] = struct{}{}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for address := range e.state {
		if _, wanted := keep[address]; !wanted {
			delete(e.state, address)
		}
	}
}

func (e *Ejector) apply(instances []sd.Instance) []sd.Instance {
	now := e.now()

	e.mu.Lock()
	// Returning expired instances happens first and unconditionally: it must not
	// depend on how the rest of the pool looks, or a pool in panic mode would
	// never release anyone.
	returned := e.expireLocked(now, instances)
	serving := make([]sd.Instance, 0, len(instances))
	candidates := make([]sd.Instance, 0, len(instances))
	for _, instance := range instances {
		if state := e.state[instance.Address]; state != nil && now.Before(state.until) {
			continue
		}
		serving = append(serving, instance)
	}
	e.mu.Unlock()

	// Resetting outside the lock keeps two mutexes from being held at once.
	for _, instance := range returned {
		e.table.Reset(instance)
	}

	// Anything still serving is judged on its current measurements.
	healthy := make([]sd.Instance, 0, len(serving))
	for _, instance := range serving {
		if e.unhealthy(instance) {
			candidates = append(candidates, instance)
			continue
		}
		healthy = append(healthy, instance)
	}
	if len(candidates) == 0 {
		return serving
	}

	// The cap is measured against the candidates in hand, not against every
	// address ever seen: instances that already left discovery must not decide
	// whether a live one may be ejected.
	if len(candidates)*100 > len(instances)*e.policy.MaxEjectionPercent {
		return instances
	}

	e.mu.Lock()
	for _, instance := range candidates {
		e.ejectLocked(now, instance)
	}
	e.mu.Unlock()
	return healthy
}

// expireLocked clears ejections whose window has passed and reports the
// instances that just came back, so their stale measurements can be dropped.
func (e *Ejector) expireLocked(now time.Time, instances []sd.Instance) []sd.Instance {
	var returned []sd.Instance
	for _, instance := range instances {
		state := e.state[instance.Address]
		if state == nil || state.until.IsZero() || now.Before(state.until) {
			continue
		}
		state.until = time.Time{}
		returned = append(returned, instance)
	}
	return returned
}

func (e *Ejector) ejectLocked(now time.Time, instance sd.Instance) {
	state := e.state[instance.Address]
	if state == nil {
		state = &ejection{}
		e.state[instance.Address] = state
	}

	duration := e.policy.BaseDuration
	for i := 0; i < state.count && duration < e.policy.MaxDuration; i++ {
		duration *= 2
	}
	duration = min(duration, e.policy.MaxDuration)

	state.count++
	state.until = now.Add(duration)
}

func (e *Ejector) unhealthy(instance sd.Instance) bool {
	stats := e.table.Stats(instance)
	if stats.Samples < e.policy.MinSamples {
		return false
	}
	if e.policy.MaxErrorRate > 0 && stats.ErrorRate > e.policy.MaxErrorRate {
		return true
	}
	if e.policy.MaxLatency > 0 && stats.Latency > e.policy.MaxLatency {
		return true
	}
	return e.policy.MaxInFlight > 0 && stats.InFlight > e.policy.MaxInFlight
}
