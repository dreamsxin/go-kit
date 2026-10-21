package kit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/kit"
)

// TestHandleEndpointKeepsTheEndpointChain proves a route registered with its own
// decoder is wrapped exactly like a typed one: the component's middleware sees
// the call, and the endpoint receives what the decoder produced.
func TestHandleEndpointKeepsTheEndpointChain(t *testing.T) {
	calls := 0
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithEndpointMiddleware(
		func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, request any) (any, error) {
				calls++
				return next(ctx, request)
			}
		}))

	var received any
	ep := func(_ context.Context, request any) (any, error) {
		received = request
		return map[string]string{"ok": "yes"}, nil
	}
	decode := func(_ context.Context, r *http.Request) (any, error) {
		return r.Header.Get("X-Widget"), nil
	}
	kit.HandleEndpoint(component, "GET /widgets", ep, decode)

	request := httptest.NewRequest(http.MethodGet, "/widgets", nil)
	request.Header.Set("X-Widget", "w-1")
	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if calls != 1 {
		t.Fatalf("endpoint middleware ran %d times, want 1", calls)
	}
	if received != "w-1" {
		t.Fatalf("endpoint received %v, want the decoder's value", received)
	}
}

// TestHandleEndpointEncodesDecoderErrors proves a decoder's error reaches the
// client through the same encoder a typed route uses, rather than as a plain 500.
func TestHandleEndpointEncodesDecoderErrors(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	decode := func(context.Context, *http.Request) (any, error) {
		return nil, apperror.InvalidArgument("widget.header_missing", "X-Widget is required")
	}
	kit.HandleEndpoint(component, "GET /widgets", endpoint.Nop, decode)

	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/widgets", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %s is not the standard error envelope: %v", recorder.Body.String(), err)
	}
	if body.Code != "widget.header_missing" {
		t.Fatalf("code = %q, want widget.header_missing", body.Code)
	}
}

// TestHandleEndpointAppliesRouteMiddleware proves the generic registration path
// reads the same RouteOption vocabulary as the typed one, so a custom decoder
// does not quietly lose a per-route concern.
func TestHandleEndpointAppliesRouteMiddleware(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	middlewareRan := false
	decode := func(context.Context, *http.Request) (any, error) { return struct{}{}, nil }
	kit.HandleEndpoint(component, "GET /widgets", func(context.Context, any) (any, error) {
		return struct{}{}, nil
	}, decode, kit.RouteMiddleware(func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			middlewareRan = true
			return next(ctx, request)
		}
	}))

	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/widgets", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !middlewareRan {
		t.Fatal("route middleware did not run")
	}
}
