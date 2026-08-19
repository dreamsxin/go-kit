package endpoint_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

type createItemRequest struct {
	Name  string
	Count int
}

func (r createItemRequest) Validate() error {
	var verr *endpoint.ValidationError
	if r.Name == "" {
		verr = endpoint.NewValidationError("name", "is required")
	}
	if r.Count < 0 {
		if verr == nil {
			verr = endpoint.NewValidationError("count", "must not be negative")
		} else {
			verr = verr.Add("count", "must not be negative")
		}
	}
	if verr != nil {
		return verr
	}
	return nil
}

func TestValidationMiddleware_RejectsInvalidRequest(t *testing.T) {
	called := false
	ep := endpoint.ValidationMiddleware()(func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	})

	_, err := ep(context.Background(), createItemRequest{Name: "", Count: -1})

	if called {
		t.Fatal("endpoint should not run for an invalid request")
	}
	var verr *endpoint.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error: want *ValidationError, got %v", err)
	}
	if len(verr.Fields) != 2 {
		t.Fatalf("fields: got %+v, want 2 failures", verr.Fields)
	}
	if verr.Fields[0].Field != "name" || verr.Fields[1].Field != "count" {
		t.Errorf("fields: %+v", verr.Fields)
	}
}

func TestValidationMiddleware_PassesValidRequest(t *testing.T) {
	var gotName string
	ep := endpoint.ValidationMiddleware()(func(_ context.Context, req any) (any, error) {
		gotName = req.(createItemRequest).Name
		return "ok", nil
	})

	resp, err := ep(context.Background(), createItemRequest{Name: "widget", Count: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotName != "widget" || resp != "ok" {
		t.Errorf("request should reach the endpoint unchanged: %q %v", gotName, resp)
	}
}

func TestValidationMiddleware_PassesThroughNonValidatableRequests(t *testing.T) {
	called := false
	ep := endpoint.ValidationMiddleware()(func(_ context.Context, _ any) (any, error) {
		called = true
		return nil, nil
	})

	if _, err := ep(context.Background(), "plain string"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("endpoint should run for requests without Validate")
	}
}

func TestValidationMiddleware_WrapsPlainErrors(t *testing.T) {
	plain := errors.New("boom")
	ep := endpoint.ValidationMiddleware()(func(_ context.Context, _ any) (any, error) {
		return nil, plain
	})

	// Endpoint errors flow through unchanged.
	if _, err := ep(context.Background(), createItemRequest{Name: "ok"}); !errors.Is(err, plain) {
		t.Fatalf("endpoint error should pass through, got %v", err)
	}
}

type validatableFunc func() error

func (f validatableFunc) Validate() error { return f() }

func TestValidationMiddleware_WrapsPlainValidateError(t *testing.T) {
	ep := endpoint.ValidationMiddleware()(func(_ context.Context, _ any) (any, error) {
		t.Fatal("endpoint should not run")
		return nil, nil
	})

	req := validatableFunc(func() error { return errors.New("bad shape") })
	_, err := ep(context.Background(), req)

	var verr *endpoint.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("plain Validate error should wrap into ValidationError, got %v", err)
	}
	if len(verr.Fields) != 1 || verr.Fields[0].Reason != "bad shape" {
		t.Errorf("fields: %+v", verr.Fields)
	}
}

func TestBuilder_WithValidation(t *testing.T) {
	ep := endpoint.NewBuilder(func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}).WithValidation().Build()

	if _, err := ep(context.Background(), createItemRequest{Name: ""}); err == nil {
		t.Fatal("builder validation should reject invalid requests")
	}
	if _, err := ep(context.Background(), createItemRequest{Name: "ok"}); err != nil {
		t.Fatalf("valid request: %v", err)
	}
}
