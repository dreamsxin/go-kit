package endpoint

import (
	"context"
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

// BreakerSettings configures the endpoint circuit breaker.
type BreakerSettings struct {
	// FailureThreshold is the number of consecutive endpoint failures that
	// trips the breaker into the open state. Zero selects 5.
	FailureThreshold int
	// SuccessThreshold is the number of consecutive probing successes that
	// closes the breaker again. Zero selects 1.
	SuccessThreshold int
	// OpenTimeout is the time the breaker stays open before a probe is
	// allowed through. Zero selects one minute.
	OpenTimeout time.Duration
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

// CircuitBreaker is a dependency-free endpoint circuit breaker middleware.
// It rejects calls while open with ErrCircuitOpen; the half-open state lets a
// single probe through to test recovery. Timeouts and cancellations of the
// caller are unchanged; the breaker observes only endpoint errors.
//
// Example:
//
//	breaker := endpoint.NewCircuitBreaker(endpoint.WithBreakerFailureThreshold(3))
//	ep = endpoint.NewBuilder(callDependency).Use(breaker).Build()
type CircuitBreaker struct {
	settings BreakerSettings

	mu            sync.Mutex
	state         BreakerState
	failures      int
	probeInFlight bool
	openedAt      time.Time
	now           func() time.Time
}

// NewCircuitBreaker constructs a circuit breaker with the default settings
// (5 consecutive failures, 1 minute open window, 1 probing success to close).
func NewCircuitBreaker(options ...BreakerOption) *CircuitBreaker {
	settings := BreakerSettings{
		FailureThreshold: 5,
		SuccessThreshold: 1,
		OpenTimeout:      time.Minute,
	}
	for _, option := range options {
		if option != nil {
			option(&settings)
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

func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case BreakerClosed:
		return nil
	case BreakerOpen:
		if cb.now().Sub(cb.openedAt) < cb.settings.OpenTimeout {
			return ErrCircuitOpen
		}
		// Window elapsed: one probe may pass.
		cb.state = BreakerHalfOpen
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

func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case BreakerClosed:
		if err != nil {
			cb.failures++
			if cb.failures >= cb.settings.FailureThreshold {
				cb.state = BreakerOpen
				cb.openedAt = cb.now()
			}
		} else {
			cb.failures = 0
		}
	case BreakerHalfOpen:
		cb.probeInFlight = false
		if err != nil {
			cb.state = BreakerOpen
			cb.openedAt = cb.now()
		} else {
			cb.state = BreakerClosed
			cb.failures = 0
		}
	}
}
