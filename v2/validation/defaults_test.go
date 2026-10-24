package validation_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/validation"
)

type defaultsRequest struct {
	Name     string  `json:"name" validate:"minLen=1,maxLen=64"`
	Status   string  `json:"status" validate:"enum=active|suspended,default=active"`
	Replicas int     `json:"replicas" validate:"min=1,max=9,default=3"`
	Shared   bool    `json:"shared" validate:"enum=true|false,default=false"`
	Ratio    float64 `json:"ratio" validate:"min=0,max=1,default=0.5"`
	Icon     *string `json:"icon" validate:"maxLen=32,default=icon-default"`
	NoRule   string  `json:"no_rule"`
}

func TestDefaultsFillsZeroValues(t *testing.T) {
	req := &defaultsRequest{Name: "app"}
	if err := validation.Defaults(req); err != nil {
		t.Fatalf("Defaults: %v", err)
	}
	if req.Status != "active" {
		t.Errorf("Status = %q, want the default", req.Status)
	}
	if req.Replicas != 3 {
		t.Errorf("Replicas = %d, want 3", req.Replicas)
	}
	if req.Shared {
		t.Errorf("Shared = true, want false — the default, not a fill of an explicit false")
	}
	if req.Ratio != 0.5 {
		t.Errorf("Ratio = %v, want 0.5", req.Ratio)
	}
	if req.Icon == nil || *req.Icon != "icon-default" {
		t.Errorf("Icon = %v, want an allocated pointer to the default", req.Icon)
	}
}

func TestDefaultsLeavesSetValues(t *testing.T) {
	icon := "custom.png"
	req := &defaultsRequest{Name: "app", Status: "suspended", Replicas: 5, Shared: true, Ratio: 0.9, Icon: &icon}
	if err := validation.Defaults(req); err != nil {
		t.Fatalf("Defaults: %v", err)
	}
	if req.Status != "suspended" || req.Replicas != 5 || !req.Shared || req.Ratio != 0.9 || *req.Icon != "custom.png" {
		t.Fatalf("an explicitly set value was overwritten: %+v", req)
	}
	if req.NoRule != "" {
		t.Fatal("fields without default= must not be touched")
	}
}

// TestDefaultsThenValidateSatisfiesTheEnum pins the pipeline order the
// middleware documents: a filled default must pass the same field's enum.
func TestDefaultsThenValidateSatisfiesTheEnum(t *testing.T) {
	req := &defaultsRequest{Name: "app"}
	if err := validation.Defaults(req); err != nil {
		t.Fatalf("Defaults: %v", err)
	}
	if err := validation.Validate(req); err != nil {
		t.Fatalf("the filled default did not satisfy the field's own rules: %v", err)
	}
}

// TestDefaultsRejectsTypeMismatchedDefault makes a broken default fail the
// same way a malformed rule does: loudly at first use.
func TestDefaultsRejectsTypeMismatchedDefault(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("a numeric field with a non-numeric default must panic at first use")
		}
		if !strings.Contains(recovered.(string), "not an integer") {
			t.Fatalf("panic = %v, want a type message naming the default", recovered)
		}
	}()
	broken := struct {
		Count int `validate:"default=many"`
	}{}
	_ = validation.Defaults(&broken)
}

// TestDefaultsMiddlewareFillsTypedRequest pins validation.defaults-middleware:
// typed routes decode into struct values, which a middleware cannot mutate in
// place, so the filled copy is what the endpoint receives.
func TestDefaultsMiddlewareFillsTypedRequest(t *testing.T) {
	var seen []any
	ep := validation.DefaultsMiddleware()(
		validation.Middleware()(
			endpoint.Endpoint(func(_ context.Context, request any) (any, error) {
				seen = append(seen, request)
				return "ok", nil
			}),
		),
	)
	if _, err := ep(context.Background(), defaultsRequest{Name: "app"}); err != nil {
		t.Fatalf("call: %v", err)
	}
	first, ok := seen[0].(defaultsRequest)
	if !ok {
		t.Fatalf("endpoint received %T, want the filled value", seen[0])
	}
	if first.Status != "active" || first.Replicas != 3 {
		t.Fatalf("endpoint received %+v, want the filled request", first)
	}

	// A pointer request is filled in place.
	ptr := &defaultsRequest{Name: "app"}
	if _, err := ep(context.Background(), ptr); err != nil {
		t.Fatalf("pointer call: %v", err)
	}
	if ptr.Status != "active" {
		t.Fatalf("pointer request was not filled in place: %+v", ptr)
	}

	// A non-struct request passes untouched.
	if _, err := ep(context.Background(), "hello"); err != nil {
		t.Fatalf("string request: %v", err)
	}
}

// TestValidationIgnoresDefaultsStillHoldsForValidate keeps the boundary the
// two systems promise: Validate checks, it never fills.
func TestValidationIgnoresDefaultsStillHoldsForValidate(t *testing.T) {
	req := defaultsRequest{Name: "app"}
	err := validation.Validate(req)
	if err == nil {
		t.Fatal("an empty Status violates enum and must still fail Validate")
	}
	if !strings.Contains(err.Error(), "status must be one of") {
		t.Fatalf("error = %v, want it to name the field", err)
	}
	_ = errors.Is
}
