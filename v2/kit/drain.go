package kit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

// ─── the stopping signal a handler can watch ─────────────────────────────────

// ErrShutdownIncomplete reports that the graceful attempt did not finish inside
// its budget, so the remaining connections were closed. Its message says how many
// requests that was: the number an operator needs to decide whether the grace
// period or the handler is the thing to fix.
var ErrShutdownIncomplete = errors.New("kit: graceful shutdown did not finish")

// hardCloseGrace is how long Shutdown waits after cancelling request contexts
// before closing connections. It is deliberately short and not configurable: the
// configurable budget has already expired, and this is only the moment a
// cancelled handler needs in order to return.
const hardCloseGrace = 250 * time.Millisecond

type stoppingKey struct{}

func withStopping(ctx context.Context, stopping <-chan struct{}) context.Context {
	return context.WithValue(ctx, stoppingKey{}, stopping)
}

// Stopping reports a channel that closes when the process announces it is going
// away, before the grace period starts. A long-lived handler — a stream, a long
// poll — selects on it to end its own response, which is the difference between a
// client seeing the end of a stream and a client seeing a broken connection.
//
// The channel is nil when the request was not served by a kit HTTP component.
// Receiving from a nil channel blocks forever, so a select that also watches
// ctx.Done() behaves correctly either way.
//
//	for {
//	    select {
//	    case <-kit.Stopping(ctx):
//	        return nil // the process is going away; end the stream
//	    case <-ctx.Done():
//	        return ctx.Err()
//	    case event := <-events:
//	        // ...
//	    }
//	}
//
// Stable: kit.stopping-signal — a handler learns the process is stopping through Stopping(ctx), which closes when draining begins and before any connection is closed.
// Covered by: TestStoppingClosesWhenTheComponentDrains, TestStoppingIsNilOutsideAKitServer
func Stopping(ctx context.Context) <-chan struct{} {
	stopping, _ := ctx.Value(stoppingKey{}).(<-chan struct{})
	return stopping
}

// Drain announces the stop to this component's handlers. The server keeps
// serving: draining tells the responses in flight that the process is going away,
// and the grace period they are given belongs to Shutdown.
//
// Stable: kit.http-drains — the HTTP component closes its stopping signal when the process drains, while it is still serving.
// Covered by: TestStoppingClosesWhenTheComponentDrains
func (h *HTTP) Drain(context.Context) error {
	h.announceStopping()
	return nil
}

func (h *HTTP) announceStopping() {
	h.stopOnce.Do(func() { close(h.stopping) })
}

// closeWhatIsLeft ends a shutdown the grace period could not: it cancels the
// request contexts, gives handlers a moment to return, and closes whatever is
// still open.
func (h *HTTP) closeWhatIsLeft(srv *http.Server, cancelServe context.CancelFunc, gracefulErr error) error {
	if cancelServe != nil {
		cancelServe()
	}
	deadline := time.Now().Add(hardCloseGrace)
	for h.inFlight.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	interrupted := h.inFlight.Load()
	closeErr := srv.Close()
	return errors.Join(
		fmt.Errorf("%w: closed the listener and %d request(s) still in flight: %w",
			ErrShutdownIncomplete, interrupted, gracefulErr),
		closeErr,
	)
}

var _ Draining = (*HTTP)(nil)
