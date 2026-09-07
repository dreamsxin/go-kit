package kit

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Draining a process.
//
// Stopping is three steps, not one. A process that is going away first says so —
// readiness starts failing, and components that accept work of their own are told
// — then it waits long enough for whoever routes traffic to notice, and only then
// does it tear anything down. Skipping the announcement is what turns a rolling
// deploy into a handful of failed requests: a load balancer or a service registry
// keeps sending work for as long as it believes the instance is healthy, and its
// belief is refreshed on its own schedule, not ours.
//
// What draining *means* for a component is the component's decision. The Host
// announces, waits, and reports; a component that must stop accepting its own
// work implements Draining and does whatever that means for it.

// ErrDraining is the readiness failure a Host reports once the process has begun
// stopping. Liveness is unaffected: a process finishing in-flight work is working
// as intended, and restarting it would be the wrong response.
var ErrDraining = errors.New("kit: process is draining")

// drainingProbeName is the readiness check the Host owns. It is registered with
// the same registry a component serves probes from, so /readyz answers it.
const drainingProbeName = "draining"

// Draining is implemented by lifecycle components that need to know the process
// is stopping before their Shutdown runs.
//
// Drain announces; it does not finish work. Stop accepting new work, hand back
// leases, tell a queue consumer to stop pulling — then return. Whatever still has
// to be finished belongs in Shutdown, which is the call with the grace period.
//
// A Host drains components in reverse attachment order, the same order it shuts
// them down, so the component nearest the caller stops accepting first.
type Draining interface {
	Drain(ctx context.Context) error
}

// WithDrainDelay configures how long Run waits between announcing that the
// process is stopping and stopping it.
//
// The default is zero, which stops immediately after the announcement — the
// behaviour a Host had before this option existed. Set it to a little more than
// the interval at which whatever routes traffic to this instance re-reads
// readiness or discovery; anything shorter and the announcement has not been
// heard yet, anything much longer only delays the deploy.
func WithDrainDelay(delay time.Duration) HostOption {
	return func(h *Host) error {
		if delay < 0 {
			return fmt.Errorf("drain delay must be >= 0")
		}
		h.drainDelay = delay
		return nil
	}
}

// Draining reports whether the process has announced that it is stopping. A
// handler can use it to decline new work of its own, or to add a header a client
// can act on.
func (h *Host) Draining() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.draining
}

// Drain announces that the process is stopping: readiness begins failing, and
// every attached Draining component is told, in reverse attachment order.
//
// It is idempotent, and it does not wait — Run owns the drain delay, and a caller
// assembling a Host by hand chooses its own. A component's Drain error is
// reported but does not stop the sequence: the announcement has already been made
// to everything else, and a component that cannot stop accepting work still has
// to be shut down.
//
// Stable: kit.draining-readiness — once a process begins stopping, readiness fails with ErrDraining before any component is shut down, and liveness keeps passing.
// Covered by: TestDrainFailsReadinessBeforeShutdown, TestShutdownDrainsFirst
//
// Stable: kit.drain-notification — a Draining component is told the process is stopping, in reverse attachment order, before anything is shut down.
// Covered by: TestDrainNotifiesComponentsInReverseOrder
func (h *Host) Drain(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("kit: nil drain context")
	}
	h.mu.Lock()
	if h.draining {
		h.mu.Unlock()
		return nil
	}
	h.draining = true
	components := append([]Lifecycle(nil), h.components...)
	h.mu.Unlock()

	var result error
	for i := len(components) - 1; i >= 0; i-- {
		draining, ok := components[i].(Draining)
		if !ok {
			continue
		}
		if err := draining.Drain(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("drain lifecycle component %s: %w", lifecycleLabel(i, components[i]), err))
		}
	}
	return result
}

// waitDrainDelay holds the process open after the announcement, so the routing
// layer has time to act on it.
//
// The wait is not cancelled by the context that stopped the process: that context
// is already done by the time we get here, and cutting the wait short would defeat
// the announcement. Only a second, harder signal — which the OS delivers, not
// this package — ends it early.
//
// Stable: kit.drain-delay — Run waits the configured drain delay between failing readiness and stopping components, and waits not at all when the delay is zero.
// Covered by: TestRunWaitsTheDrainDelayBeforeShutdown, TestRunWithoutADrainDelayStopsImmediately
func (h *Host) waitDrainDelay() {
	h.mu.Lock()
	delay := h.drainDelay
	h.mu.Unlock()
	if delay <= 0 {
		return
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	<-timer.C
}

// registerDrainingProbe makes the drain state visible wherever this assembly
// serves readiness. A Host with no probe surface simply has nowhere to report it,
// which is not an error: a pure worker has no orchestrator asking.
func (h *Host) registerDrainingProbe() error {
	sink := h.readinessSink()
	if sink == nil {
		return nil
	}
	return sink.Probes().AddReadiness(drainingProbeName, func(context.Context) error {
		if h.Draining() {
			return ErrDraining
		}
		return nil
	})
}

func (h *Host) readinessSink() ReadinessSink {
	for _, component := range h.components {
		if candidate, ok := component.(ReadinessSink); ok && candidate.Probes() != nil {
			return candidate
		}
	}
	return nil
}
