package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	handybreaker "github.com/streadway/handy/breaker"
	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
)

func TestDivide_Success(t *testing.T) {
	resp := divide(context.Background(), 10, 2)
	if resp.Failed() != nil {
		t.Fatalf("unexpected error: %v", resp.Failed())
	}
	if resp.Result != 5 {
		t.Errorf("got %f, want 5", resp.Result)
	}
}

func TestDivide_ByZero(t *testing.T) {
	resp := divide(context.Background(), 10, 0)
	if resp.Failed() == nil {
		t.Fatal("expected division by zero error")
	}
}

func TestCalcResponse_Failer(t *testing.T) {
	r := calcResponse{Err: errors.New("oops")}
	if r.Failed() == nil {
		t.Error("Failed() should return non-nil")
	}
	r2 := calcResponse{Result: 42}
	if r2.Failed() != nil {
		t.Error("Failed() should return nil for success")
	}
}

func TestDivideEndpoint_Success(t *testing.T) {
	resp, err := divideEndpoint(context.Background(), divReq{A: 9, B: 3})
	if err != nil {
		t.Fatalf("endpoint error: %v", err)
	}
	cr := resp.(calcResponse)
	if cr.Result != 3 {
		t.Errorf("got %f, want 3", cr.Result)
	}
}

func TestDivideTyped_Success(t *testing.T) {
	resp, err := divideTyped(context.Background(), divReq{A: 8, B: 4})
	if err != nil {
		t.Fatalf("typed endpoint error: %v", err)
	}
	if resp.Result != 2 {
		t.Errorf("got %f, want 2", resp.Result)
	}
}

func TestChain_ExecutionOrder(t *testing.T) {
	var order []string
	tag := func(name string) endpoint.Middleware {
		return func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, req any) (any, error) {
				order = append(order, name+":pre")
				resp, err := next(ctx, req)
				order = append(order, name+":post")
				return resp, err
			}
		}
	}
	chained := endpoint.Chain(tag("A"), tag("B"), tag("C"))(endpoint.Nop)
	chained(context.Background(), nil) //nolint:errcheck

	want := []string{"A:pre", "B:pre", "C:pre", "C:post", "B:post", "A:post"}
	for i, v := range want {
		if order[i] != v {
			t.Errorf("order[%d]: got %q, want %q", i, order[i], v)
		}
	}
}

func TestBuilder_WithMetrics(t *testing.T) {
	var m endpoint.Metrics
	ep := endpoint.NewBuilder(divideEndpoint).
		WithMetrics(&m).
		Build()

	ep(context.Background(), divReq{A: 6, B: 2}) //nolint:errcheck
	if m.RequestCount != 1 {
		t.Errorf("RequestCount: got %d, want 1", m.RequestCount)
	}
}

func TestErrorHandlingMiddleware(t *testing.T) {
	failEp := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("something went wrong")
	})
	wrapped := endpoint.ErrorHandlingMiddleware("myOp")(failEp)
	_, err := wrapped(context.Background(), nil)

	var ew *endpoint.ErrorWrapper
	if !errors.As(err, &ew) {
		t.Fatalf("expected *endpoint.ErrorWrapper, got %T", err)
	}
	if ew.Operation != "myOp" {
		t.Errorf("operation: got %q, want %q", ew.Operation, "myOp")
	}
}

func TestTimeoutMiddleware_Triggers(t *testing.T) {
	slowEp := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
		select {
		case <-time.After(5 * time.Second):
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	timedEp := endpoint.TimeoutMiddleware(20 * time.Millisecond)(slowEp)
	_, err := timedEp(context.Background(), nil)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestGobreaker_OpensAfterFailures(t *testing.T) {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "test",
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= 3 },
	})
	alwaysFail := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("backend down")
	})
	ep := circuitbreaker.Gobreaker(cb)(alwaysFail)

	for i := 0; i < 3; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Error("expected circuit breaker open error")
	}
}

func TestHandyBreaker(t *testing.T) {
	hb := circuitbreaker.HandyBreaker(handybreaker.NewBreaker(0.5))
	alwaysFail := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("fail")
	})
	ep := hb(alwaysFail)
	_, err := ep(context.Background(), nil)
	// first call may fail with backend error or breaker error — either is non-nil
	if err == nil {
		t.Error("expected error from always-failing endpoint")
	}
}

func TestErroringLimiter_AllowsThenRejects(t *testing.T) {
	// burst=2: first 2 succeed, then rejected
	lim := rate.NewLimiter(0, 2)
	ep := ratelimit.NewErroringLimiter(lim)(endpoint.Nop)

	for i := 0; i < 2; i++ {
		if _, err := ep(context.Background(), nil); err != nil {
			t.Errorf("call %d: unexpected error: %v", i+1, err)
		}
	}
	_, err := ep(context.Background(), nil)
	if !errors.Is(err, ratelimit.ErrLimited) {
		t.Errorf("expected ErrLimited, got %v", err)
	}
}

func TestDelayingLimiter_ContextDeadline(t *testing.T) {
	delayLim := rate.NewLimiter(rate.Every(time.Second), 1)
	ep := ratelimit.NewDelayingLimiter(delayLim)(endpoint.Nop)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ep(ctx, nil) //nolint:errcheck — consumes token
	_, err := ep(ctx, nil)
	if err == nil {
		t.Error("expected context deadline error on second call")
	}
}

func TestTypedEndpoint_Unwrap(t *testing.T) {
	typedEp := endpoint.TypedEndpoint[divReq, calcResponse](
		func(ctx context.Context, req divReq) (calcResponse, error) {
			return divide(ctx, req.A, req.B), nil
		},
	)
	unwrapped := endpoint.Unwrap[divReq, calcResponse](
		endpoint.NewTypedBuilder(typedEp).
			WithTimeout(2 * time.Second).
			Build(),
	)
	r, err := unwrapped(context.Background(), divReq{A: 9, B: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Result != 3 {
		t.Errorf("got %f, want 3", r.Result)
	}
}
