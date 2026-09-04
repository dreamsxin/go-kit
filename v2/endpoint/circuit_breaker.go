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
)

// BreakerSettings configures the endpoint circuit breaker.
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
	FailurePredicate func(error) bool
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

// CircuitBreaker is a dependency-free endpoint circuit breaker middleware.
// It rejects calls while open with ErrCircuitOpen; the half-open state lets a
// single probe through at a time until SuccessThreshold consecutive probes
// succeed. Timeouts and cancellations of the caller are unchanged; the breaker
// observes only endpoint errors.
//
// Example:
//
//	breaker := endpoint.NewCircuitBreaker(endpoint.WithBreakerFailureThreshold(3))
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
}

// NewCircuitBreaker constructs a circuit breaker with the default settings
// (5 consecutive failures, 1 minute open window, 1 probing success to close).
// Non-positive settings fall back to those defaults.
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
	return &CircuitBreaker{settings: settings, now: time.Now}
}

// Middleware returns the endpoint middleware that enforces the breaker.
func (cb *CircuitBreaker) Middleware() Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if err := cb.beforeRequest(); err != nil {
				return nil, err
			}
			resp, err := next(ctx, request)
			cb.afterRequest(err)
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

func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err != nil && !cb.settings.FailurePredicate(err) {
		if cb.state == BreakerHalfOpen {
			cb.probeInFlight = false
		}
		return
	}

	switch cb.state {
	case BreakerClosed:
		if err != nil {
			cb.failures++
			if cb.failures >= cb.settings.FailureThreshold {
				cb.trip()
			}
		} else {
			cb.failures = 0
		}
	case BreakerHalfOpen:
		cb.probeInFlight = false
		if err != nil {
			cb.trip()
			return
		}
		cb.successes++
		if cb.successes >= cb.settings.SuccessThreshold {
			cb.state = BreakerClosed
			cb.failures = 0
			cb.successes = 0
		}
	}
}

// trip moves the breaker to the open state and starts a new open window.
// The caller must hold cb.mu.
func (cb *CircuitBreaker) trip() {
	cb.state = BreakerOpen
	cb.openedAt = cb.now()
	cb.successes = 0
	cb.probeInFlight = false
}
