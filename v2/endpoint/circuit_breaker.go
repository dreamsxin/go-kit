package endpoint

import (
	"context"
	"errors"
	"sync"
	"time"
)

// BreakerState reports the circuit breaker state.
type BreakerState int

const (
	// BreakerClosed accepts requests normally.
	BreakerClosed BreakerState = iota
	// BreakerOpen rejects requests until the next probing interval.
	BreakerOpen
	// BreakerHalfOpen lets one probe request through to test recovery.
	BreakerHalfOpen
)

// Default circuit breaker settings. NewCircuitBreaker falls back to these for
// any non-positive setting.
const (
	DefaultBreakerFailureThreshold = 5
	DefaultBreakerSuccessThreshold = 1
	DefaultBreakerOpenTimeout      = time.Minute
	// DefaultBreakerWindowSize is how many recent outcomes MaxErrorRate
	// considers when no window size is configured.
	DefaultBreakerWindowSize = 100
	// DefaultBreakerMinSamples is how many outcomes the window must hold
	// before MaxErrorRate may trip the breaker.
	DefaultBreakerMinSamples = 20
)

// BreakerSettings configures the endpoint circuit breaker.
//
// Two trip conditions are available and both may be armed at once. Consecutive
// counting (FailureThreshold) reacts fastest to a dependency that is completely
// down. Rate counting (MaxErrorRate over a rolling window) catches the more
// common failure: a dependency that fails a third of the time never produces a
// long enough consecutive run to trip a counter, so a counter alone leaves it
// running forever.
type BreakerSettings struct {
	// FailureThreshold is the number of consecutive endpoint failures that
	// trips the breaker into the open state. Non-positive selects
	// DefaultBreakerFailureThreshold.
	FailureThreshold int
	// SuccessThreshold is the number of consecutive probing successes that
	// closes the breaker again. Non-positive selects
	// DefaultBreakerSuccessThreshold.
	SuccessThreshold int
	// OpenTimeout is the time the breaker stays open before a probe is
	// allowed through. Non-positive selects DefaultBreakerOpenTimeout.
	OpenTimeout time.Duration
	// FailurePredicate reports whether an endpoint error represents a
	// dependency failure. Caller cancellation is excluded by default because
	// it does not say anything about dependency health. Set this to customize
	// classification, for example to exclude transport-specific errors.
	//
	// An excluded error is not recorded at all — neither failure nor success —
	// so it cannot dilute MaxErrorRate the way counting it as a success would.
	FailurePredicate func(error) bool

	// MaxErrorRate trips the breaker when the share of failures in the rolling
	// window exceeds it, as a fraction: 0.5 means more than half. Zero, the
	// default, leaves only consecutive counting armed.
	MaxErrorRate float64
	// WindowSize is how many recent outcomes MaxErrorRate considers.
	// Non-positive selects DefaultBreakerWindowSize.
	WindowSize int
	// MinSamples is how many outcomes the window must hold before MaxErrorRate
	// may trip the breaker, so one unlucky call in a quiet minute cannot open
	// the circuit. Non-positive selects DefaultBreakerMinSamples; larger than
	// WindowSize is capped to it, because a window that small can never hold
	// more.
	MinSamples int
	// SlowCallThreshold makes a call taking at least this long count as a
	// failure even when it returned no error. A dependency answering in thirty
	// seconds is not healthier than one returning errors: it holds the caller's
	// goroutines and spends its budget. Zero, the default, disables slow-call
	// accounting.
	//
	// It is also what makes a timeout legible. A context deadline arriving at
	// the endpoint says nothing about who owned the budget — the caller's or the
	// dependency's — so DeadlineExceeded alone cannot be classified, and the
	// default predicate therefore counts it as a failure. Measured duration can
	// be classified, which is why slow-call accounting is the honest way to
	// judge a dependency that got slow rather than broken.
	SlowCallThreshold time.Duration
}

// BreakerOption mutates BreakerSettings. See NewCircuitBreaker.
type BreakerOption func(*BreakerSettings)

// WithBreakerFailureThreshold sets the consecutive failure count that trips
// the breaker.
func WithBreakerFailureThreshold(n int) BreakerOption {
	return func(s *BreakerSettings) { s.FailureThreshold = n }
}

// WithBreakerSuccessThreshold sets the consecutive probing success count that
// closes the breaker.
func WithBreakerSuccessThreshold(n int) BreakerOption {
	return func(s *BreakerSettings) { s.SuccessThreshold = n }
}

// WithBreakerOpenTimeout sets how long the breaker stays open before probing.
func WithBreakerOpenTimeout(d time.Duration) BreakerOption {
	return func(s *BreakerSettings) { s.OpenTimeout = d }
}

// WithBreakerFailurePredicate configures which endpoint errors count toward
// the failure threshold. A nil predicate restores the default policy.
func WithBreakerFailurePredicate(predicate func(error) bool) BreakerOption {
	return func(s *BreakerSettings) { s.FailurePredicate = predicate }
}

// WithBreakerMaxErrorRate arms rate-based tripping: the breaker opens when more
// than rate of the outcomes in the rolling window are failures. Values at or
// below zero disable it; values at or above 1 are meaningless, since a rate can
// never exceed 1, and are treated as 1 minus one sample.
func WithBreakerMaxErrorRate(rate float64) BreakerOption {
	return func(s *BreakerSettings) { s.MaxErrorRate = rate }
}

// WithBreakerWindowSize sets how many recent outcomes the rate check considers.
func WithBreakerWindowSize(n int) BreakerOption {
	return func(s *BreakerSettings) { s.WindowSize = n }
}

// WithBreakerMinSamples sets how many outcomes the window must hold before the
// rate check may trip the breaker.
func WithBreakerMinSamples(n int) BreakerOption {
	return func(s *BreakerSettings) { s.MinSamples = n }
}

// WithBreakerSlowCallThreshold counts a call taking at least d as a failure,
// even when it returned no error.
func WithBreakerSlowCallThreshold(d time.Duration) BreakerOption {
	return func(s *BreakerSettings) { s.SlowCallThreshold = d }
}

// CircuitBreaker is a dependency-free endpoint circuit breaker middleware.
// It rejects calls while open with ErrCircuitOpen; the half-open state lets a
// single probe through at a time until SuccessThreshold consecutive probes
// succeed. Timeouts and cancellations of the caller are unchanged; the breaker
// observes only endpoint errors and call durations.
//
// Example:
//
//	breaker := endpoint.NewCircuitBreaker(
//	    endpoint.WithBreakerMaxErrorRate(0.5),
//	    endpoint.WithBreakerSlowCallThreshold(2*time.Second),
//	)
//	ep := endpoint.NewBuilder(callDependency).Use(breaker.Middleware()).Build()
type CircuitBreaker struct {
	settings BreakerSettings

	mu            sync.Mutex
	state         BreakerState
	failures      int
	successes     int
	probeInFlight bool
	openedAt      time.Time
	now           func() time.Time

	// window is a ring of recent outcomes, true for a failure. It is reset on
	// every state transition: measurements taken before the breaker opened
	// describe a dependency the caller has since stopped talking to, so keeping
	// them would re-trip the circuit on the first call after recovery.
	window     []bool
	windowNext int
	windowLen  int
	windowBad  int
}

// NewCircuitBreaker constructs a circuit breaker with the default settings
// (5 consecutive failures, 1 minute open window, 1 probing success to close,
// and no rate or slow-call check). Non-positive settings fall back to those
// defaults.
func NewCircuitBreaker(options ...BreakerOption) *CircuitBreaker {
	var settings BreakerSettings
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	if settings.FailureThreshold < 1 {
		settings.FailureThreshold = DefaultBreakerFailureThreshold
	}
	if settings.SuccessThreshold < 1 {
		settings.SuccessThreshold = DefaultBreakerSuccessThreshold
	}
	if settings.OpenTimeout <= 0 {
		settings.OpenTimeout = DefaultBreakerOpenTimeout
	}
	if settings.FailurePredicate == nil {
		settings.FailurePredicate = func(err error) bool {
			return err != nil && !errors.Is(err, context.Canceled)
		}
	}

	breaker := &CircuitBreaker{settings: settings, now: time.Now}
	if !breaker.rateArmed() {
		return breaker
	}
	if breaker.settings.WindowSize < 1 {
		breaker.settings.WindowSize = DefaultBreakerWindowSize
	}
	if breaker.settings.MinSamples < 1 {
		breaker.settings.MinSamples = DefaultBreakerMinSamples
	}
	if breaker.settings.MinSamples > breaker.settings.WindowSize {
		breaker.settings.MinSamples = breaker.settings.WindowSize
	}
	// A rate can never exceed 1, so a threshold at or above 1 would arm a check
	// that never fires. Treat it as "every call in the window failed".
	if size := float64(breaker.settings.WindowSize); breaker.settings.MaxErrorRate >= 1 {
		breaker.settings.MaxErrorRate = (size - 1) / size
	}
	breaker.window = make([]bool, breaker.settings.WindowSize)
	return breaker
}

// rateArmed reports whether the rolling window is in use. Slow-call accounting
// needs it too: a slow call is a failure, and without a rate to compare against
// it would only ever feed the consecutive counter.
func (cb *CircuitBreaker) rateArmed() bool {
	return cb.settings.MaxErrorRate > 0
}

// Middleware returns the endpoint middleware that enforces the breaker.
func (cb *CircuitBreaker) Middleware() Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if err := cb.beforeRequest(); err != nil {
				return nil, err
			}
			started := cb.now()
			resp, err := next(ctx, request)
			cb.afterRequest(err, cb.now().Sub(started))
			return resp, err
		}
	}
}

// State returns the current breaker state.
func (cb *CircuitBreaker) State() BreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// WithCircuitBreaker appends breaker.Middleware to the Builder. It panics when
// breaker is nil so misassembly fails at startup.
func (b *Builder) WithCircuitBreaker(breaker *CircuitBreaker) *Builder {
	if breaker == nil {
		panic("endpoint: circuit breaker cannot be nil")
	}
	return b.UseNamed("circuit_breaker", breaker.Middleware())
}

func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case BreakerClosed:
		return nil
	case BreakerOpen:
		if cb.now().Sub(cb.openedAt) < cb.settings.OpenTimeout {
			return cb.openRejection()
		}
		// Window elapsed: probing may begin.
		cb.state = BreakerHalfOpen
		cb.successes = 0
		cb.probeInFlight = true
		return nil
	case BreakerHalfOpen:
		if cb.probeInFlight {
			return ErrCircuitOpen
		}
		cb.probeInFlight = true
		return nil
	default:
		return ErrCircuitOpen
	}
}

// openRejection reports ErrCircuitOpen with the time left in the open window,
// so transports can emit a Retry-After hint. The caller must hold cb.mu.
func (cb *CircuitBreaker) openRejection() error {
	remaining := cb.settings.OpenTimeout - cb.now().Sub(cb.openedAt)
	return NewRetryAfterError(ErrCircuitOpen, remaining)
}

// afterRequest records one outcome. elapsed is measured around the wrapped
// endpoint, so it includes everything the breaker sits in front of.
func (cb *CircuitBreaker) afterRequest(err error, elapsed time.Duration) {
	slow := cb.settings.SlowCallThreshold > 0 && elapsed >= cb.settings.SlowCallThreshold

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil && !cb.settings.FailurePredicate(err) && !slow {
		// Nothing was observed about the dependency: not a failure, and not a
		// success either, so it must not enter the window — counting it as a
		// success would dilute the rate that the window exists to measure.
		if cb.state == BreakerHalfOpen {
			cb.probeInFlight = false
		}
		return
	}
	// A slow call counts even when the error was excluded: whoever owned the
	// cancelled budget, the dependency did not answer inside the threshold.
	failed := slow || err != nil

	switch cb.state {
	case BreakerClosed:
		cb.recordLocked(failed)
		if !failed {
			cb.failures = 0
			return
		}
		cb.failures++
		// A success can never raise the rate, so the check belongs here only.
		if cb.failures >= cb.settings.FailureThreshold || cb.rateExceededLocked() {
			cb.trip()
		}
	case BreakerHalfOpen:
		// Probes are deliberately kept out of the window: one probe cannot meet
		// MinSamples, and the window is reset on every transition anyway.
		cb.probeInFlight = false
		if failed {
			cb.trip()
			return
		}
		cb.successes++
		if cb.successes >= cb.settings.SuccessThreshold {
			cb.state = BreakerClosed
			cb.failures = 0
			cb.successes = 0
			cb.resetWindowLocked()
		}
	}
}

// recordLocked appends one outcome to the rolling window, evicting the oldest
// when it is full. It is a no-op when no rate check is armed.
// The caller must hold cb.mu.
func (cb *CircuitBreaker) recordLocked(failed bool) {
	if cb.window == nil {
		return
	}
	if cb.windowLen == len(cb.window) {
		if cb.window[cb.windowNext] {
			cb.windowBad--
		}
	} else {
		cb.windowLen++
	}
	cb.window[cb.windowNext] = failed
	if failed {
		cb.windowBad++
	}
	cb.windowNext = (cb.windowNext + 1) % len(cb.window)
}

// rateExceededLocked reports whether the window holds enough samples and too
// many failures. The caller must hold cb.mu.
func (cb *CircuitBreaker) rateExceededLocked() bool {
	if cb.window == nil || cb.windowLen < cb.settings.MinSamples {
		return false
	}
	return float64(cb.windowBad)/float64(cb.windowLen) > cb.settings.MaxErrorRate
}

// resetWindowLocked forgets every recorded outcome. windowLen gates all reads,
// so the stored values need not be cleared. The caller must hold cb.mu.
func (cb *CircuitBreaker) resetWindowLocked() {
	cb.windowNext, cb.windowLen, cb.windowBad = 0, 0, 0
}

// trip moves the breaker to the open state and starts a new open window.
// The caller must hold cb.mu.
func (cb *CircuitBreaker) trip() {
	cb.state = BreakerOpen
	cb.openedAt = cb.now()
	cb.successes = 0
	cb.probeInFlight = false
	cb.resetWindowLocked()
}
