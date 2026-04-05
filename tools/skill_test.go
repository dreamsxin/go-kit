// Package tools_test verifies that every code snippet in SKILL.md compiles
// and behaves correctly.  If a snippet in SKILL.md changes, the corresponding
// test here must be updated to match.
package tools_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
	kitlog "github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/instance"
	httpserver "github.com/dreamsxin/go-kit/transport/http/server"
	httpclient "github.com/dreamsxin/go-kit/transport/http/client"
)

// ── SKILL.md: 30-Second Service ──────────────────────────────────────────────

type helloReq  struct{ Name string `json:"name"` }
type helloResp struct{ Message string `json:"message"` }

func TestSKILL_30SecondService(t *testing.T) {
	handler := httpserver.NewJSONServer[helloReq](
		func(_ context.Context, req helloReq) (any, error) {
			return helloResp{Message: "Hello, " + req.Name + "!"}, nil
		},
	)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/hello",
		strings.NewReader(`{"name":"world"}`))
	r.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Hello, world!") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

// ── SKILL.md: Production Service Pattern ─────────────────────────────────────

type createUserReq  struct{ Name string `json:"name"` }
type createUserResp struct{ ID uint `json:"id"` }

func createUserLogic(_ context.Context, req createUserReq) (createUserResp, error) {
	if req.Name == "" {
		return createUserResp{}, errors.New("name required")
	}
	return createUserResp{ID: 1}, nil
}

func TestSKILL_ProductionServicePattern(t *testing.T) {
	logger := kitlog.NewNopLogger()
	var metrics endpoint.Metrics

	base := endpoint.TypedEndpoint[createUserReq, createUserResp](createUserLogic)

	ep := endpoint.Unwrap[createUserReq, createUserResp](
		endpoint.NewTypedBuilder(base).
			WithMetrics(&metrics).
			WithErrorHandling("CreateUser").
			WithTimeout(5 * time.Second).
			WithTracing().
			WithBackpressure(200).
			Use(circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(
				gobreaker.Settings{Name: "CreateUser"},
			))).
			Use(ratelimit.NewErroringLimiter(
				rate.NewLimiter(rate.Every(time.Second), 100),
			)).
			Build(),
	)
	_ = logger

	resp, err := ep(context.Background(), createUserReq{Name: "alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != 1 {
		t.Errorf("want ID=1, got %d", resp.ID)
	}
	if metrics.RequestCount != 1 {
		t.Errorf("want 1 request, got %d", metrics.RequestCount)
	}
}

// ── SKILL.md: Key APIs — endpoint ────────────────────────────────────────────

type myReq  struct{ V int }
type myResp struct{ V int }

func TestSKILL_EndpointAPI_Untyped(t *testing.T) {
	var ep endpoint.Endpoint = func(_ context.Context, req any) (any, error) {
		return "result", nil
	}
	resp, err := ep(context.Background(), nil)
	if err != nil || resp != "result" {
		t.Errorf("want 'result', got %v %v", resp, err)
	}
}

func TestSKILL_EndpointAPI_Typed(t *testing.T) {
	var typedEp endpoint.TypedEndpoint[myReq, myResp] = func(_ context.Context, req myReq) (myResp, error) {
		return myResp{V: req.V * 2}, nil
	}

	// Wrap → plain → Unwrap
	plain := typedEp.Wrap()
	typed := endpoint.Unwrap[myReq, myResp](plain)
	resp, err := typed(context.Background(), myReq{V: 21})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.V != 42 {
		t.Errorf("want 42, got %d", resp.V)
	}
}

func TestSKILL_EndpointAPI_Builder(t *testing.T) {
	logger := kitlog.NewNopLogger()
	var metrics endpoint.Metrics

	base := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	})

	myMiddleware := func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			return next(ctx, req)
		}
	}

	ep := endpoint.NewBuilder(base).
		WithMetrics(&metrics).
		WithErrorHandling("op").
		WithTimeout(5 * time.Second).
		WithTracing().
		WithBackpressure(200).
		WithLogging(logger, "op").
		Use(myMiddleware).
		Build()

	resp, err := ep(context.Background(), nil)
	if err != nil || resp != "ok" {
		t.Errorf("want 'ok', got %v %v", resp, err)
	}
	if metrics.RequestCount != 1 {
		t.Errorf("want 1 request, got %d", metrics.RequestCount)
	}
}

// ── SKILL.md: transport/http/server ──────────────────────────────────────────

func TestSKILL_HTTPServer_NewJSONServer(t *testing.T) {
	type req  struct{ Name string `json:"name"` }
	type resp struct{ Msg  string `json:"msg"`  }

	handler := httpserver.NewJSONServer[req](func(_ context.Context, r req) (any, error) {
		return resp{Msg: "hi " + r.Name}, nil
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/",
			strings.NewReader(`{"name":"test"}`)))

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "hi test") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestSKILL_HTTPServer_NewJSONServerWithMiddleware(t *testing.T) {
	type req  struct{ V int `json:"v"` }

	var mwCalled bool
	handler := httpserver.NewJSONServerWithMiddleware[req](
		func(_ context.Context, r req) (any, error) {
			return map[string]int{"doubled": r.V * 2}, nil
		},
		func(b *endpoint.Builder) *endpoint.Builder {
			return b.Use(func(next endpoint.Endpoint) endpoint.Endpoint {
				return func(ctx context.Context, req any) (any, error) {
					mwCalled = true
					return next(ctx, req)
				}
			})
		},
	)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/",
			strings.NewReader(`{"v":21}`)))

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	if !mwCalled {
		t.Error("middleware should have been called")
	}
}

func TestSKILL_HTTPServer_Hooks(t *testing.T) {
	type req struct{}

	var beforeCalled, afterCalled, finalizerCalled bool

	handler := httpserver.NewJSONServer[req](
		func(_ context.Context, _ req) (any, error) { return "ok", nil },
		httpserver.ServerBefore(func(ctx context.Context, r *http.Request) context.Context {
			beforeCalled = true
			return ctx
		}),
		httpserver.ServerAfter(func(ctx context.Context, r *http.Request, w *httpserver.InterceptingWriter) context.Context {
			afterCalled = true
			return ctx
		}),
		httpserver.ServerFinalizer(func(ctx context.Context, r *http.Request, w *httpserver.InterceptingWriter) {
			finalizerCalled = true
		}),
	)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))

	if !beforeCalled   { t.Error("ServerBefore not called") }
	if !afterCalled    { t.Error("ServerAfter not called") }
	if !finalizerCalled { t.Error("ServerFinalizer not called") }
}

func TestSKILL_HTTPServer_JSONErrorEncoder(t *testing.T) {
	handler := httpserver.NewJSONServer[struct{}](
		func(_ context.Context, _ struct{}) (any, error) {
			return nil, errors.New("something failed")
		},
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error"`) {
		t.Errorf("want JSON error body, got: %s", rec.Body.String())
	}
}

// ── SKILL.md: transport/http/client ──────────────────────────────────────────

func TestSKILL_HTTPClient_NewJSONClient(t *testing.T) {
	type respType struct{ Echo string `json:"echo"` }

	// Start an echo server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"echo":"pong"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	ep, err := httpclient.NewJSONClient[respType](http.MethodPost, srv.URL)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}

	raw, err := ep(context.Background(), struct{}{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	resp := raw.(respType)
	if resp.Echo != "pong" {
		t.Errorf("want 'pong', got %q", resp.Echo)
	}
}

func TestSKILL_HTTPClient_ClientBefore(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Write([]byte("{}")) //nolint:errcheck
	}))
	defer srv.Close()

	token := "my-token"
	ep, err := httpclient.NewJSONClient[struct{}](http.MethodPost, srv.URL,
		httpclient.ClientBefore(func(ctx context.Context, r *http.Request) context.Context {
			r.Header.Set("Authorization", "Bearer "+token)
			return ctx
		}),
	)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}

	ep(context.Background(), struct{}{}) //nolint:errcheck
	if capturedAuth != "Bearer my-token" {
		t.Errorf("want 'Bearer my-token', got %q", capturedAuth)
	}
}

// ── SKILL.md: sd package ─────────────────────────────────────────────────────

func TestSKILL_SD_InMemory(t *testing.T) {
	logger := kitlog.NewNopLogger()

	factory := endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			return addr, nil
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"host1:8080", "host2:8080"}})
	time.Sleep(10 * time.Millisecond)

	ep := sd.NewEndpointWithDefaults(cache, factory, logger)
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "host1:8080" && resp != "host2:8080" {
		t.Errorf("unexpected response: %v", resp)
	}
}

func TestSKILL_SD_CustomSettings(t *testing.T) {
	logger := kitlog.NewNopLogger()
	factory := endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
		return endpoint.Nop, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := sd.NewEndpoint(cache, factory, logger,
		sd.WithMaxRetries(3),
		sd.WithTimeout(500*time.Millisecond),
		sd.WithInvalidateOnError(5*time.Second),
	)
	_, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── SKILL.md: log package ─────────────────────────────────────────────────────

func TestSKILL_Log_Development(t *testing.T) {
	logger, err := kitlog.NewDevelopment()
	if err != nil {
		t.Fatalf("NewDevelopment: %v", err)
	}
	defer logger.Sync() //nolint:errcheck
	logger.Sugar().Infof("test log: %s", "ok")
}

func TestSKILL_Log_Nop(t *testing.T) {
	logger := kitlog.NewNopLogger()
	logger.Sugar().Infof("this should not panic")
}

// ── SKILL.md: Hystrix (built-in) ─────────────────────────────────────────────

func TestSKILL_Hystrix_ConfigureAndUse(t *testing.T) {
	circuitbreaker.HystrixConfigureCommand("skill-test", circuitbreaker.HystrixConfig{
		Timeout:                time.Second,
		MaxConcurrentRequests:  100,
		RequestVolumeThreshold: 20,
		SleepWindow:            5 * time.Second,
		ErrorPercentThreshold:  50,
	})

	ep := circuitbreaker.Hystrix("skill-test")(func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	})

	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("want 'ok', got %v", resp)
	}
}

// ── SKILL.md: Testing Patterns ───────────────────────────────────────────────

func TestSKILL_TestingPatterns_UnitEndpoint(t *testing.T) {
	ep := endpoint.Endpoint(func(_ context.Context, req any) (any, error) {
		return "result", nil
	})
	resp, err := ep(context.Background(), "input")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "result" {
		t.Errorf("want 'result', got %v", resp)
	}
}

func TestSKILL_TestingPatterns_TypedEndpoint(t *testing.T) {
	type Req  struct{ Name string }
	type Resp struct{ Msg  string }

	ep := endpoint.TypedEndpoint[Req, Resp](func(_ context.Context, req Req) (Resp, error) {
		return Resp{Msg: "hello " + req.Name}, nil
	})
	resp, err := ep(context.Background(), Req{Name: "world"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg != "hello world" {
		t.Errorf("got %q", resp.Msg)
	}
}

func TestSKILL_TestingPatterns_HTTPHandler(t *testing.T) {
	type Req  struct{ Name string `json:"name"` }
	type Resp struct{ Msg  string `json:"msg"`  }

	handler := httpserver.NewJSONServer[Req](func(_ context.Context, req Req) (any, error) {
		return Resp{Msg: "hi " + req.Name}, nil
	})

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"test"}`))
	r.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestSKILL_TestingPatterns_InMemorySD(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"127.0.0.1:8080"}})
	time.Sleep(10 * time.Millisecond)

	factory := endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
		return endpoint.Nop, io.NopCloser(nil), nil
	})

	ep := sd.NewEndpoint(cache, factory, kitlog.NewNopLogger())
	_, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
}

// ── SKILL.md: Common Mistakes ────────────────────────────────────────────────

func TestSKILL_CommonMistakes_ErrBackpressure(t *testing.T) {
	// Verify ErrBackpressure is exported and can be checked with errors.Is
	ep := endpoint.BackpressureMiddleware(0)(endpoint.Nop) // max=0 → always reject
	_, err := ep(context.Background(), nil)
	if !errors.Is(err, endpoint.ErrBackpressure) {
		t.Errorf("want ErrBackpressure, got %v", err)
	}
}

func TestSKILL_CommonMistakes_LoggerSync(t *testing.T) {
	// Verify logger.Sync() does not panic
	logger, err := kitlog.NewDevelopment()
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Sync(); err != nil {
		// Sync may return an error on some platforms (e.g. /dev/stderr) — not fatal
		t.Logf("logger.Sync: %v (non-fatal)", err)
	}
}
