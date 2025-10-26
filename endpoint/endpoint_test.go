package endpoint

import (
	"context"
	"testing"
)

func TestEndpointBasic(t *testing.T) {
	ep := func(ctx context.Context, request interface{}) (interface{}, error) {
		return "response", nil
	}

	ctx := context.Background()
	resp, err := ep(ctx, "test")
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

	ctx := context.Background()
	resp, err := chained(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp != "ok" {
		t.Errorf("expected 'ok', got %v", resp)
	}

	expectedCalls := []string{"mw1", "mw2", "endpoint"}
	if len(calls) != len(expectedCalls) {
		t.Errorf("expected %d calls, got %d", len(expectedCalls), len(calls))
	}

	for i, call := range calls {
		if call != expectedCalls[i] {
			t.Errorf("call %d: expected %s, got %s", i, expectedCalls[i], call)
		}
	}
}
