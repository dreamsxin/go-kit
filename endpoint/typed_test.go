package endpoint_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
)

type addReq  struct{ A, B int }
type addResp struct{ Sum int }

var addTyped endpoint.TypedEndpoint[addReq, addResp] = func(_ context.Context, req addReq) (addResp, error) {
	return addResp{Sum: req.A + req.B}, nil
}

// ── TypedEndpoint.Wrap / Unwrap round-trip ────────────────────────────────────

func TestTypedEndpoint_WrapUnwrap(t *testing.T) {
	plain := addTyped.Wrap()

	// plain Endpoint works
	resp, err := plain(context.Background(), addReq{3, 4})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.(addResp).Sum != 7 {
		t.Errorf("want 7, got %v", resp)
	}

	// Unwrap restores type safety
	typed := endpoint.Unwrap[addReq, addResp](plain)
	r, err := typed(context.Background(), addReq{10, 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Sum != 15 {
		t.Errorf("want 15, got %d", r.Sum)
	}
}

// ── NewTypedBuilder ───────────────────────────────────────────────────────────

func TestNewTypedBuilder(t *testing.T) {
	var called bool
	mw := endpoint.Middleware(func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			called = true
			return next(ctx, req)
		}
	})

	plain := endpoint.NewTypedBuilder(addTyped).Use(mw).Build()
	typed := endpoint.Unwrap[addReq, addResp](plain)

	r, err := typed(context.Background(), addReq{1, 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Sum != 3 {
		t.Errorf("want 3, got %d", r.Sum)
	}
	if !called {
		t.Error("middleware should have been called")
	}
}

// ── Unwrap propagates errors ──────────────────────────────────────────────────

func TestUnwrap_PropagatesError(t *testing.T) {
	sentinel := errors.New("fail")
	failEp := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, sentinel
	})
	typed := endpoint.Unwrap[addReq, addResp](failEp)
	_, err := typed(context.Background(), addReq{})
	if !errors.Is(err, sentinel) {
		t.Errorf("want sentinel error, got %v", err)
	}
}

// ── TypeAssertError ───────────────────────────────────────────────────────────

func TestUnwrap_TypeAssertError(t *testing.T) {
	// Endpoint returns wrong type
	wrongEp := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return "not an addResp", nil
	})
	typed := endpoint.Unwrap[addReq, addResp](wrongEp)
	_, err := typed(context.Background(), addReq{})
	if err == nil {
		t.Fatal("expected TypeAssertError, got nil")
	}
	var tae *endpoint.TypeAssertError
	if !errors.As(err, &tae) {
		t.Errorf("expected *TypeAssertError, got %T: %v", err, err)
	}
}

// ── TypedEndpoint + Builder.WithTimeout ──────────────────────────────────────

func TestTypedEndpoint_WithTimeout(t *testing.T) {
	slow := endpoint.TypedEndpoint[addReq, addResp](func(ctx context.Context, _ addReq) (addResp, error) {
		select {
		case <-time.After(5 * time.Second):
			return addResp{}, nil
		case <-ctx.Done():
			return addResp{}, ctx.Err()
		}
	})

	plain := endpoint.NewTypedBuilder(slow).
		WithTimeout(50 * time.Millisecond).
		Build()
	typed := endpoint.Unwrap[addReq, addResp](plain)

	_, err := typed(context.Background(), addReq{})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestTypedEndpoint_Wrap_ReturnsTypeAssertErrorOnWrongRequestType(t *testing.T) {
	plain := addTyped.Wrap()

	_, err := plain(context.Background(), "not an addReq")
	if err == nil {
		t.Fatal("expected TypeAssertError, got nil")
	}

	var tae *endpoint.TypeAssertError
	if !errors.As(err, &tae) {
		t.Fatalf("expected *TypeAssertError, got %T: %v", err, err)
	}
}
