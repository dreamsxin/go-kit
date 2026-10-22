// This file is the one place in this package that imports the assembly layer,
// and it does so from a test on purpose: the route table's whole claim is that
// it mounts on a kit component without becoming a second stack, and that claim
// is not testable from inside the package that makes it.
package rest_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/dreamsxin/go-kit/v2/resource"
	"github.com/dreamsxin/go-kit/v2/resource/rest"
)

// componentWithRoutes mounts the collection's routes on a kit component the way
// an application does, and returns the component plus the middleware counter.
func componentWithRoutes(t *testing.T, collection resource.Collection) (*kit.HTTP, func() int) {
	t.Helper()
	calls := 0
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithEndpointMiddleware(
		func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, request any) (any, error) {
				calls++
				return next(ctx, request)
			}
		}))
	routes, err := rest.Routes(collection, rest.WithOpenAccess())
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	for _, route := range routes {
		kit.HandleEndpoint(component, route.Pattern, route.Endpoint, route.Decode)
	}
	return component, func() int { return calls }
}

// TestResourceRoutesKeepTheEndpointChain proves the custom decoder does not
// buy the route a shortcut around the middleware a component installs. That
// shortcut is what a hand-assembled handler would take, and it is invisible
// until the day a rate limiter turns out not to cover the route.
func TestResourceRoutesKeepTheEndpointChain(t *testing.T) {
	component, calls := componentWithRoutes(t, newCollection(t))
	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/products", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if calls() != 1 {
		t.Fatalf("endpoint middleware ran %d times, want 1", calls())
	}
}

// TestResourceRouteAnswersTheSameOnAComponentAndAPlainMux is the parity
// judgement for this layer: how a route is mounted changes what surrounds it,
// not what it answers.
func TestResourceRouteAnswersTheSameOnAComponentAndAPlainMux(t *testing.T) {
	collection := newCollection(t)
	component, _ := componentWithRoutes(t, collection)
	plain := mount(t, collection)

	for _, target := range []string{"/products", "/products?size=2&sort=-price", "/products/2", "/products:aggregate?function=count"} {
		onComponent := httptest.NewRecorder()
		component.ServeHTTP(onComponent, httptest.NewRequest(http.MethodGet, target, nil))
		onMux := do(t, plain, http.MethodGet, target, "")
		if onComponent.Code != onMux.status {
			t.Fatalf("%s: status = %d on the component, %d on the mux", target, onComponent.Code, onMux.status)
		}
		if !bytes.Equal(onComponent.Body.Bytes(), onMux.body) {
			t.Fatalf("%s: body differs:\ncomponent: %s\nmux:       %s", target, onComponent.Body.String(), onMux.body)
		}
	}
}

// TestHandleEndpointRefusesToRegisterWithoutADecoder keeps a programming
// mistake at startup, where it can stop the process, rather than at the first
// request, where it is a 500 nobody can explain.
func TestHandleEndpointRefusesToRegisterWithoutADecoder(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	defer func() {
		if recover() == nil {
			t.Fatal("HandleEndpoint accepted a nil decoder")
		}
	}()
	kit.HandleEndpoint(component, "GET /nothing", endpoint.Nop, nil)
}
