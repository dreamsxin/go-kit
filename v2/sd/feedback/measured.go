package feedback

import (
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// Measured is a feedback Table bound to a discovery subscription. Everything
// taken from it reads accounting that is aligned with the live snapshot, and
// records into the same table it reads.
//
// This is the assembly, and Measure is the only way to get one, which is what
// makes the invalid combinations unreachable rather than merely discouraged:
//
//   - A scored or least-request balancer cannot be built without the accounting
//     that feeds it, because the balancer comes from the table.
//   - A slow-start ramp cannot be built without the subscription that dates each
//     instance, because Measure opened it before returning.
//   - An Ejector cannot end up on a second subscription reporting a different
//     snapshot, because Eject joins this one.
//
// Close stops the subscription. It closes neither the Instancer nor any Balancer
// taken from here; a Balancer is closed by its own Close, and the Instancer by
// whoever created it.
type Measured struct {
	table    *Table
	follower *follower
}

// Measure subscribes to instancer and returns the accounting bound to it. The
// options configure the Table — see WithAlpha and WithClock.
//
// Pass the registration Instancer. A view derived from another one — active
// health checking, for one — is resolved to its source through
// sd.DerivedInstancer, because a withdrawal is indistinguishable from a
// deregistration to accounting: releasing an instance's measurements the moment a
// probe withdrew it would return it with a clean record, and active and passive
// health checking would cancel each other out.
//
// Measure panics on a nil Instancer, which is a programming error rather than a
// runtime condition.
func Measure(instancer sd.Instancer, options ...Option) *Measured {
	table := NewTable(options...)
	return &Measured{table: table, follower: follow(instancer, table)}
}

// Table reports the accounting itself, for a caller that reads Stats or records
// an outcome observed somewhere no balancer can see it.
func (m *Measured) Table() *Table { return m.table }

// Eject adds passive ejection driven by this table, on the subscription this
// Measured already holds, so the ejection state and the measurements behind it
// cannot disagree about which instances exist. An Ejector joining after the first
// snapshot is handed that snapshot rather than waiting for the next one.
//
// The returned Ejector's Filter belongs in the strategy; pass it through
// Balancer, or through Ranking for a shortlist.
func (m *Measured) Eject(policy EjectionPolicy, options ...EjectorOption) *Ejector {
	ejector := NewEjector(m.table, policy, options...)
	m.follower.add(ejector)
	return ejector
}

// LeastRequest balances source by in-flight depth — power of two choices, the
// value Envoy and gRPC use. It is the strategy to reach for when this process is
// on the data path: it measures what is happening rather than reading what was
// reported.
func (m *Measured) LeastRequest(source endpointer.InstanceEndpointer, options ...selector.LeastRequestOption) sd.Balancer {
	return m.Balancer(source, selector.LeastRequest(m.table.load(), options...))
}

// Scored balances source by what this table measured of each instance — error
// rate, latency, and local concurrency, combined by the formula documented on
// Stats — breaking ties at random.
//
// For a load signal this process did not measure itself, such as ORCA or LRS
// style out-of-band reporting, balancer.NewScored takes the caller's own
// selector.ScoreFunc and needs no table.
func (m *Measured) Scored(source endpointer.InstanceEndpointer) sd.Balancer {
	return m.Balancer(source, selector.Scored(m.table.score()))
}

// SlowStartWeighted balances source in proportion to weight, ramping each
// instance from one to its full weight over window, counted from the moment
// discovery first reported it.
//
// A freshly started instance has cold caches, an unwarmed JIT and no connection
// pool, so giving it a full share the moment it appears is how a deployment turns
// into a latency spike. The ramp is dated from the subscription rather than from
// the instance's first call, which is the whole reason this lives here: on a table
// that follows nothing, every instance looks brand new forever and the ramp never
// finishes.
//
// A window at or below zero means no ramp, leaving weight as it is. An instance
// weighted zero or below stays unselectable: zero means "never pick me", and
// ramping would contradict it.
func (m *Measured) SlowStartWeighted(source endpointer.InstanceEndpointer, weight selector.WeightFunc, window time.Duration) sd.Balancer {
	ramped := selector.SlowStart(weight, m.table.firstSeen(), window)
	return m.Balancer(source, selector.WeightedRandom(ramped))
}

// Balancer balances source with strategy, augmented with this table's
// accounting. It is the seam for a strategy the named constructors do not
// cover — one behind an ejection filter, or a consistent hash that should still
// record what it observes:
//
//	measured := feedback.Measure(instancer)
//	ejector := measured.Eject(feedback.EjectionPolicy{MaxErrorRate: 0.5, MinSamples: 20})
//	lb := measured.Balancer(set, selector.Filtered(selector.RoundRobin(), ejector.Filter()))
func (m *Measured) Balancer(source endpointer.InstanceEndpointer, strategy selector.Strategy) sd.Balancer {
	return balancer.New(source, m.table.Wrap(strategy))
}

// Ranking answers "where should I connect?" with an ordered shortlist scored by
// this table, for a routing service whose caller dials the instance itself.
//
// A Ranker owns no call, so it records nothing: there is no single outcome to
// attribute a shortlist to. The scores it reads are therefore only as good as
// what else feeds this table — a balancer taken from the same Measured, or
// Table.Observe called where the calls actually execute.
func (m *Measured) Ranking(source selector.Source, filters ...sd.InstanceFilter) selector.Ranker {
	return selector.NewRanker(source, m.table.score(), filters...)
}

// Close stops the discovery subscription. It is idempotent.
func (m *Measured) Close() error { return m.follower.Close() }
