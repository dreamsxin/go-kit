package kit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/kit"
)

// doJSONPost serves one request through the component itself: kit.HTTP is an
// http.Handler, so nothing has to listen for these tests to be honest about routing.
func doJSONPost(t *testing.T, component *kit.HTTP, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, request)
	return recorder
}

type sizedRequest struct {
	Payload string `json:"payload"`
}

// TestPerRouteBodyLimitKeepsEndpointMiddleware is the trap this closes. Before, a
// route that needed a larger body had to be registered with Handle and a raw
// transport server, which skipped the component's endpoint middleware without saying
// so — observability lost as a side effect of a size.
func TestPerRouteBodyLimitKeepsEndpointMiddleware(t *testing.T) {
	var sawMiddleware bool
	component := kit.MustNewHTTP("127.0.0.1:0",
		kit.WithEndpointMiddleware(func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, request any) (any, error) {
				sawMiddleware = true
				return next(ctx, request)
			}
		}),
	)
	kit.HandleJSONTypedWithBodyLimit[sizedRequest, map[string]string](component, "POST /upload",
		func(_ context.Context, req sizedRequest) (map[string]string, error) {
			return map[string]string{"got": req.Payload}, nil
		}, 1<<20)

	recorder := doJSONPost(t, component, "/upload", `{"payload":"small"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if !sawMiddleware {
		t.Error("the component's endpoint middleware did not run; the per-route limit dropped the wiring")
	}
}

// TestPerRouteBodyLimitAppliesToThatRouteOnly: the component default still governs
// every other route, which is the point of saying it per route.
func TestPerRouteBodyLimitAppliesToThatRouteOnly(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithJSONMaxBodyBytes(64))

	kit.HandleJSONTyped[sizedRequest, map[string]string](component, "POST /small",
		func(_ context.Context, req sizedRequest) (map[string]string, error) {
			return map[string]string{"got": req.Payload}, nil
		})
	kit.HandleJSONTypedWithBodyLimit[sizedRequest, map[string]string](component, "POST /large",
		func(_ context.Context, req sizedRequest) (map[string]string, error) {
			return map[string]string{"got": req.Payload}, nil
		}, 1<<20)

	body := `{"payload":"` + strings.Repeat("x", 512) + `"}`

	tight := doJSONPost(t, component, "/small", body)
	if tight.Code == http.StatusOK {
		t.Errorf("the component limit did not apply to /small: status = %d", tight.Code)
	}
	roomy := doJSONPost(t, component, "/large", body)
	if roomy.Code != http.StatusOK {
		t.Errorf("the per-route limit did not apply to /large: status = %d, body = %s", roomy.Code, roomy.Body.String())
	}
}

func TestPerRouteBodyLimitRejectsANonPositiveLimit(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	defer func() {
		if recover() == nil {
			t.Error("a zero limit was accepted; an unbounded body should not be smuggled in per route")
		}
	}()
	kit.HandleJSONTypedWithBodyLimit[sizedRequest, map[string]string](component, "POST /zero",
		func(context.Context, sizedRequest) (map[string]string, error) { return nil, nil }, 0)
}
