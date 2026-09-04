package endpoint

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errBusiness = errors.New("business failure")

type carriedFailure struct{ err error }

func (c carriedFailure) Failed() error { return c.err }

// A Failer response reaches middleware with a nil error, so metrics and
// resilience count it as a success. FailerMiddleware converts it first.
func TestFailerMiddlewareSurfacesTheCarriedError(t *testing.T) {
	base := Endpoint(func(context.Context, any) (any, error) {
		return carriedFailure{err: errBusiness}, nil
	})

	if _, err := base(context.Background(), nil); err != nil {
		t.Fatalf("the endpoint itself must still return nil, got %v", err)
	}

	_, err := FailerMiddleware()(base)(context.Background(), nil)
	if !errors.Is(err, errBusiness) {
		t.Fatalf("error = %v, want the carried failure", err)
	}
}

func TestFailerMiddlewarePassesThroughSuccess(t *testing.T) {
	base := Endpoint(func(context.Context, any) (any, error) {
		return carriedFailure{}, nil
	})

	response, err := FailerMiddleware()(base)(context.Background(), nil)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if response == nil {
		t.Fatal("a response that did not fail must be preserved")
	}
}

func TestResponseErrorIgnoresNonFailers(t *testing.T) {
	if err := ResponseError("plain"); err != nil {
		t.Fatalf("ResponseError = %v, want nil", err)
	}
}

// With the breaker outside it, the converted failure is what trips the breaker.
func TestFailerMiddlewareFeedsTheBreaker(t *testing.T) {
	base := Endpoint(func(context.Context, any) (any, error) {
		return carriedFailure{err: errBusiness}, nil
	})
	ep := NewBuilder(base).
		WithCircuitBreaker(NewCircuitBreaker(WithBreakerFailureThreshold(1))).
		WithFailer().
		Build()

	if _, err := ep(context.Background(), nil); !errors.Is(err, errBusiness) {
		t.Fatalf("first call error = %v, want the carried failure", err)
	}
	if _, err := ep(context.Background(), nil); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("second call error = %v, want the breaker to be open", err)
	}
}

// context.WithTimeout(ctx, 0) yields an already expired context, so an unset
// timeout must mean no timeout instead of a guaranteed failure.
func TestTimeoutMiddlewareTreatsNonPositiveAsNoDeadline(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		var hadDeadline bool
		ep := TimeoutMiddleware(d)(Endpoint(func(ctx context.Context, _ any) (any, error) {
			_, hadDeadline = ctx.Deadline()
			return nil, ctx.Err()
		}))
		if _, err := ep(context.Background(), nil); err != nil {
			t.Fatalf("d = %v: err = %v, want nil", d, err)
		}
		if hadDeadline {
			t.Fatalf("d = %v: the endpoint should see no deadline", d)
		}
	}
}

// An unset concurrency limit must not reject every request.
func TestBackpressureMiddlewareClampsNonPositiveLimit(t *testing.T) {
	ep := BackpressureMiddleware(0)(Nop)
	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("err = %v, want the single permitted call to pass", err)
	}
}

func TestInFlightMiddlewareClampsNonPositiveLimit(t *testing.T) {
	var inflight int64
	ep := InFlightMiddleware(0, &inflight)(Nop)
	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("err = %v, want the single permitted call to pass", err)
	}
}
