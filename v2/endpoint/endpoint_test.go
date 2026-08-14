package endpoint

import (
	"context"
	"errors"
	"testing"
)

// ─────────────────────────── Nop ───────────────────────────

func TestNop(t *testing.T) {
	resp, err := Nop(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Nop should not return error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("Nop should return non-nil response")
	}
}

// ─────────────────────────── Chain / Middleware ───────────────────────────

func TestEndpointBasic(t *testing.T) {
	ep := func(ctx context.Context, request interface{}) (interface{}, error) {
		return "response", nil
	}
	resp, err := ep(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "response" {
		t.Errorf("expected 'response', got %v", resp)
	}
}

func TestMiddlewareChain(t *testing.T) {
	var calls []string

	mw1 := func(next Endpoint) Endpoint {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			calls = append(calls, "mw1")
			return next(ctx, request)
		}
	}
	mw2 := func(next Endpoint) Endpoint {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			calls = append(calls, "mw2")
			return next(ctx, request)
		}
	}
	ep := func(ctx context.Context, request interface{}) (interface{}, error) {
		calls = append(calls, "endpoint")
		return "ok", nil
	}

	chained := Chain(mw1, mw2)(ep)
	resp, err := chained(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("expected 'ok', got %v", resp)
	}
	expected := []string{"mw1", "mw2", "endpoint"}
	if len(calls) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(calls), calls)
	}
	for i, c := range calls {
		if c != expected[i] {
			t.Errorf("call[%d]: want %q, got %q", i, expected[i], c)
		}
	}
}

// Chain 只包一层时应透传原 endpoint
func TestChainSingle(t *testing.T) {
	var called bool
	mw := func(next Endpoint) Endpoint {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			called = true
			return next(ctx, req)
		}
	}
	ep := func(ctx context.Context, request interface{}) (interface{}, error) {
		return "direct", nil
	}
	chained := Chain(mw)(ep)
	resp, err := chained(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "direct" {
		t.Errorf("expected 'direct', got %v", resp)
	}
	if !called {
		t.Error("middleware should have been called")
	}
}

// ─────────────────────────── Failer ───────────────────────────

type failResponse struct{ err error }

func (f failResponse) Failed() error { return f.err }

func TestFailer_WithError(t *testing.T) {
	sentinel := errors.New("business logic failed")
	var resp interface{} = failResponse{err: sentinel}
	if f, ok := resp.(Failer); ok {
		if f.Failed() != sentinel {
			t.Errorf("expected sentinel error, got %v", f.Failed())
		}
	} else {
		t.Fatal("response should implement Failer")
	}
}

func TestFailer_WithoutError(t *testing.T) {
	var resp interface{} = failResponse{err: nil}
	if f, ok := resp.(Failer); ok {
		if f.Failed() != nil {
			t.Errorf("expected nil, got %v", f.Failed())
		}
	} else {
		t.Fatal("response should implement Failer")
	}
}

// ─────────────────────────── MetricsMiddleware ───────────────────────────

func TestMetricsMiddleware_Success(t *testing.T) {
	m := &Metrics{}
	ep := MetricsMiddleware(m)(func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})

	for i := 0; i < 3; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}
	snapshot := m.Snapshot()
	if snapshot.RequestCount != 3 {
		t.Errorf("RequestCount: want 3, got %d", snapshot.RequestCount)
	}
	if snapshot.SuccessCount != 3 {
		t.Errorf("SuccessCount: want 3, got %d", snapshot.SuccessCount)
	}
	if snapshot.ErrorCount != 0 {
		t.Errorf("ErrorCount: want 0, got %d", snapshot.ErrorCount)
	}
	if snapshot.TotalDuration < 0 {
		t.Error("TotalDuration should be non-negative")
	}
	if snapshot.LastRequestTime.IsZero() {
		t.Error("LastRequestTime should not be zero")
	}
}

func TestMetricsMiddleware_Error(t *testing.T) {
	m := &Metrics{}
	sentinel := errors.New("oops")
	ep := MetricsMiddleware(m)(func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, sentinel
	})

	ep(context.Background(), nil) //nolint:errcheck
	ep(context.Background(), nil) //nolint:errcheck
	snapshot := m.Snapshot()
	if snapshot.RequestCount != 2 {
		t.Errorf("RequestCount: want 2, got %d", snapshot.RequestCount)
	}
	if snapshot.ErrorCount != 2 {
		t.Errorf("ErrorCount: want 2, got %d", snapshot.ErrorCount)
	}
	if snapshot.SuccessCount != 0 {
		t.Errorf("SuccessCount: want 0, got %d", snapshot.SuccessCount)
	}
}

func TestMetricsMiddleware_Mixed(t *testing.T) {
	m := &Metrics{}
	calls := 0
	ep := MetricsMiddleware(m)(func(ctx context.Context, req interface{}) (interface{}, error) {
		calls++
		if calls%2 == 0 {
			return nil, errors.New("even fail")
		}
		return "odd ok", nil
	})

	for i := 0; i < 4; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}
	snapshot := m.Snapshot()
	if snapshot.RequestCount != 4 {
		t.Errorf("RequestCount: want 4, got %d", snapshot.RequestCount)
	}
	if snapshot.SuccessCount != 2 {
		t.Errorf("SuccessCount: want 2, got %d", snapshot.SuccessCount)
	}
	if snapshot.ErrorCount != 2 {
		t.Errorf("ErrorCount: want 2, got %d", snapshot.ErrorCount)
	}
}

// ─────────────────────────── ErrorHandlingMiddleware ───────────────────────────

func TestErrorHandlingMiddleware_NoError(t *testing.T) {
	ep := ErrorHandlingMiddleware("op")(func(ctx context.Context, req interface{}) (interface{}, error) {
		return "val", nil
	})
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "val" {
		t.Errorf("expected 'val', got %v", resp)
	}
}

func TestErrorHandlingMiddleware_WrapsError(t *testing.T) {
	raw := errors.New("raw err")
	ep := ErrorHandlingMiddleware("myop")(func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, raw
	})
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ew *ErrorWrapper
	if !errors.As(err, &ew) {
		t.Fatalf("expected *ErrorWrapper, got %T: %v", err, err)
	}
	if ew.Operation != "myop" {
		t.Errorf("Operation: want %q, got %q", "myop", ew.Operation)
	}
	if !errors.Is(err, raw) {
		t.Errorf("Unwrap chain should reach raw error")
	}
}

func TestErrorWrapper_ErrorString(t *testing.T) {
	ew := &ErrorWrapper{Operation: "doThings", Err: errors.New("boom")}
	want := "doThings: boom"
	if ew.Error() != want {
		t.Errorf("want %q, got %q", want, ew.Error())
	}
}
