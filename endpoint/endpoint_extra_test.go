package endpoint_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
)

// ── Builder ───────────────────────────────────────────────────────────────────

func TestBuilder_Build_NoMiddleware(t *testing.T) {
	called := false
	base := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	})
	ep := endpoint.NewBuilder(base).Build()
	ep(context.Background(), nil) //nolint:errcheck
	if !called {
		t.Error("base endpoint was not called")
	}
}

func TestBuilder_Use_AppliesInOrder(t *testing.T) {
	var order []string
	tag := func(name string) endpoint.Middleware {
		return func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, req any) (any, error) {
				order = append(order, name)
				return next(ctx, req)
			}
		}
	}
	ep := endpoint.NewBuilder(endpoint.Nop).
		Use(tag("A")).
		Use(tag("B")).
		Use(tag("C")).
		Build()
	ep(context.Background(), nil) //nolint:errcheck

	want := []string{"A", "B", "C"}
	for i, v := range want {
		if order[i] != v {
			t.Errorf("order[%d]: got %q, want %q", i, order[i], v)
		}
	}
}

func TestBuilder_WithTimeout(t *testing.T) {
	slow := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
		select {
		case <-time.After(5 * time.Second):
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	ep := endpoint.NewBuilder(slow).WithTimeout(20 * time.Millisecond).Build()
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestBuilder_WithErrorHandling(t *testing.T) {
	base := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("boom")
	})
	ep := endpoint.NewBuilder(base).WithErrorHandling("myOp").Build()
	_, err := ep(context.Background(), nil)

	var ew *endpoint.ErrorWrapper
	if !errors.As(err, &ew) {
		t.Fatalf("expected *ErrorWrapper, got %T: %v", err, err)
	}
	if ew.Operation != "myOp" {
		t.Errorf("operation: got %q, want %q", ew.Operation, "myOp")
	}
}

func TestBuilder_WithMetrics_CountsSuccessAndError(t *testing.T) {
	var m endpoint.Metrics

	failOnce := true
	base := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		if failOnce {
			failOnce = false
			return nil, errors.New("fail")
		}
		return "ok", nil
	})
	ep := endpoint.NewBuilder(base).WithMetrics(&m).Build()

	ep(context.Background(), nil) //nolint:errcheck — error call
	ep(context.Background(), nil) //nolint:errcheck — success call

	if m.RequestCount != 2 {
		t.Errorf("RequestCount: got %d, want 2", m.RequestCount)
	}
	if m.ErrorCount != 1 {
		t.Errorf("ErrorCount: got %d, want 1", m.ErrorCount)
	}
	if m.SuccessCount != 1 {
		t.Errorf("SuccessCount: got %d, want 1", m.SuccessCount)
	}
}

// ── Chain ─────────────────────────────────────────────────────────────────────

func TestChain_SingleMiddleware(t *testing.T) {
	called := false
	mw := endpoint.Middleware(func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			called = true
			return next(ctx, req)
		}
	})
	ep := endpoint.Chain(mw)(endpoint.Nop)
	ep(context.Background(), nil) //nolint:errcheck
	if !called {
		t.Error("middleware was not called")
	}
}

func TestChain_MultipleMiddlewares_OuterFirst(t *testing.T) {
	var order []string
	mw := func(name string) endpoint.Middleware {
		return func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, req any) (any, error) {
				order = append(order, name)
				return next(ctx, req)
			}
		}
	}
	ep := endpoint.Chain(mw("outer"), mw("middle"), mw("inner"))(endpoint.Nop)
	ep(context.Background(), nil) //nolint:errcheck

	want := []string{"outer", "middle", "inner"}
	for i, v := range want {
		if order[i] != v {
			t.Errorf("order[%d]: got %q, want %q", i, order[i], v)
		}
	}
}

// ── MetricsMiddleware ─────────────────────────────────────────────────────────

func TestMetricsMiddleware_TracksAll(t *testing.T) {
	var m endpoint.Metrics
	ep := endpoint.MetricsMiddleware(&m)(endpoint.Nop)

	for i := 0; i < 5; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}
	if m.RequestCount != 5 {
		t.Errorf("RequestCount: got %d, want 5", m.RequestCount)
	}
	if m.SuccessCount != 5 {
		t.Errorf("SuccessCount: got %d, want 5", m.SuccessCount)
	}
	if m.ErrorCount != 0 {
		t.Errorf("ErrorCount: got %d, want 0", m.ErrorCount)
	}
	// TotalDuration may be 0 for very fast endpoints; just check it's non-negative
	if m.TotalDuration < 0 {
		t.Error("TotalDuration should be >= 0")
	}
	if m.LastRequestTime.IsZero() {
		t.Error("LastRequestTime should be set")
	}
}

func TestMetricsMiddleware_CountsErrors(t *testing.T) {
	var m endpoint.Metrics
	failEp := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("fail")
	})
	ep := endpoint.MetricsMiddleware(&m)(failEp)

	ep(context.Background(), nil) //nolint:errcheck
	if m.ErrorCount != 1 {
		t.Errorf("ErrorCount: got %d, want 1", m.ErrorCount)
	}
	if m.SuccessCount != 0 {
		t.Errorf("SuccessCount: got %d, want 0", m.SuccessCount)
	}
}

// ── ErrorHandlingMiddleware ───────────────────────────────────────────────────

func TestErrorHandlingMiddleware_WrapsError(t *testing.T) {
	cause := errors.New("original")
	ep := endpoint.ErrorHandlingMiddleware("op")(
		endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			return nil, cause
		}),
	)
	_, err := ep(context.Background(), nil)

	var ew *endpoint.ErrorWrapper
	if !errors.As(err, &ew) {
		t.Fatalf("expected *ErrorWrapper, got %T", err)
	}
	if ew.Operation != "op" {
		t.Errorf("operation: got %q, want %q", ew.Operation, "op")
	}
	if !errors.Is(err, cause) {
		t.Error("Unwrap should expose original cause")
	}
}

func TestErrorHandlingMiddleware_PassthroughOnSuccess(t *testing.T) {
	ep := endpoint.ErrorHandlingMiddleware("op")(endpoint.Nop)
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestErrorWrapper_ErrorString(t *testing.T) {
	ew := &endpoint.ErrorWrapper{Operation: "myOp", Err: errors.New("boom")}
	got := ew.Error()
	if got != "myOp: boom" {
		t.Errorf("Error(): got %q, want %q", got, "myOp: boom")
	}
}

// ── TimeoutMiddleware ─────────────────────────────────────────────────────────

func TestTimeoutMiddleware_PassesThrough(t *testing.T) {
	ep := endpoint.TimeoutMiddleware(time.Second)(endpoint.Nop)
	_, err := ep(context.Background(), nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTimeoutMiddleware_CancelsContext(t *testing.T) {
	var ctxErr error
	ep := endpoint.TimeoutMiddleware(20*time.Millisecond)(
		endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
			<-ctx.Done()
			ctxErr = ctx.Err()
			return nil, ctxErr
		}),
	)
	ep(context.Background(), nil) //nolint:errcheck
	if ctxErr == nil {
		t.Error("expected context to be cancelled")
	}
}

// ── LoggingMiddleware ─────────────────────────────────────────────────────────

func TestLoggingMiddleware_Success(t *testing.T) {
	logger := kitlog.NewNopLogger()
	ep := endpoint.LoggingMiddleware(logger, "testOp")(endpoint.Nop)
	_, err := ep(context.Background(), nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoggingMiddleware_Error(t *testing.T) {
	logger := kitlog.NewNopLogger()
	failEp := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("fail")
	})
	ep := endpoint.LoggingMiddleware(logger, "testOp")(failEp)
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Error("expected error to propagate")
	}
}

// ── Nop ──────────────────────────────────────────────────────────────────────

func TestNop_ReturnsEmptyStruct(t *testing.T) {
	resp, err := endpoint.Nop(context.Background(), nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}
