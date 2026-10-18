package resilience_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/endpoint/resilience"
)

var errBusiness = errors.New("business failure")

type carriedFailure struct{ err error }

func (c carriedFailure) Failed() error { return c.err }

// With the breaker outside it, the failure FailerMiddleware converts is what
// trips the breaker: the pairing the old Builder methods encoded, now spelled
// as two Use calls in order.
func TestFailerMiddlewareFeedsTheBreaker(t *testing.T) {
	base := endpoint.Endpoint(func(context.Context, any) (any, error) {
		return carriedFailure{err: errBusiness}, nil
	})
	breaker := resilience.NewCircuitBreaker(resilience.WithBreakerFailureThreshold(1))
	ep := endpoint.NewBuilder(base).
		Use(breaker.Middleware()).
		Use(endpoint.FailerMiddleware()).
		Build()

	if _, err := ep(context.Background(), nil); !errors.Is(err, errBusiness) {
		t.Fatalf("first call error = %v, want the carried failure", err)
	}
	if _, err := ep(context.Background(), nil); !errors.Is(err, endpoint.ErrCircuitOpen) {
		t.Fatalf("second call error = %v, want the breaker to be open", err)
	}
}

// An unset concurrency limit must not reject every request.
func TestBackpressureMiddlewareClampsNonPositiveLimit(t *testing.T) {
	ep := resilience.BackpressureMiddleware(0)(endpoint.Nop)
	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("err = %v, want the single permitted call to pass", err)
	}
}

func TestInFlightMiddlewareClampsNonPositiveLimit(t *testing.T) {
	var inflight int64
	ep := resilience.InFlightMiddleware(0, &inflight)(endpoint.Nop)
	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("err = %v, want the single permitted call to pass", err)
	}
}
