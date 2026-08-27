package kit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/kit"
)

type helloReq struct {
	Name string `json:"name"`
}
type helloResp struct {
	Message string `json:"message"`
}

func helloHandler(_ context.Context, req helloReq) (any, error) {
	if req.Name == "" {
		return nil, errors.New("name required")
	}
	return helloResp{Message: "Hello, " + req.Name + "!"}, nil
}

// newSvc creates an HTTP component with /hello registered and returns an httptest.Server.
// This mirrors the recommended kit.HandleJSON pattern.
func newSvc(t *testing.T, opts ...kit.Option) (*kit.HTTP, *httptest.Server) {
	t.Helper()
	svc := kit.MustNewHTTP(":0", opts...)
	kit.HandleJSON[helloReq](svc, "/hello", helloHandler)
	ts := httptest.NewServer(svc) // HTTP implements http.Handler
	t.Cleanup(ts.Close)
	return svc, ts
}

// ── README Quick Start pattern ────────────────────────────────────────────────

// TestReadme_QuickStart verifies the exact pattern shown in README.md works.
func TestReadme_QuickStart(t *testing.T) {
	svc := kit.MustNewHTTP(":0")
	kit.HandleJSON[helloReq](svc, "/hello", func(_ context.Context, req helloReq) (any, error) {
		return helloResp{Message: "Hello, " + req.Name + "!"}, nil
	})

	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "world"})
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /hello: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var result helloResp
	json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
	if result.Message != "Hello, world!" {
		t.Errorf("message: got %q, want %q", result.Message, "Hello, world!")
	}
}

// TestReadme_WithMiddleware verifies the middleware options shown in README.md.
func TestReadme_WithMiddleware(t *testing.T) {
	var metrics endpoint.Metrics
	var calls int
	middleware := func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			calls++
			return next(ctx, request)
		}
	}

	svc := kit.MustNewHTTP(":0",
		kit.WithEndpointMiddleware(middleware),
		kit.WithTimeout(5*time.Second),
		kit.WithRequestID(),
		kit.WithMetrics(&metrics),
	)
	kit.HandleJSON[helloReq](svc, "/hello", helloHandler)

	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "test"})
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /hello: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := metrics.Snapshot().RequestCount; got != 1 {
		t.Errorf("RequestCount: got %d, want 1", got)
	}
	if calls != 1 {
		t.Errorf("middleware calls: got %d, want 1", calls)
	}
}

// ── Service implements http.Handler ──────────────────────────────────────────

func TestHTTP_ImplementsHTTPHandler(t *testing.T) {
	svc := kit.MustNewHTTP(":0")
	var _ http.Handler = svc // compile-time check
}

// ── /health endpoint (always registered) ─────────────────────────────────────

func TestService_HealthEndpoint(t *testing.T) {
	_, ts := newSvc(t)

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body) //nolint:errcheck
	if body["status"] != "ok" {
		t.Errorf("health status: got %v", body["status"])
	}
}

func TestService_HealthEndpoint_WithMetrics(t *testing.T) {
	var m endpoint.Metrics
	_, ts := newSvc(t, kit.WithMetrics(&m))

	// make a request to increment counter
	body, _ := json.Marshal(helloReq{Name: "x"})
	http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body)) //nolint:errcheck

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	var health map[string]any
	json.NewDecoder(resp.Body).Decode(&health) //nolint:errcheck
	// health should include request count when metrics are enabled
	if _, ok := health["requests"]; !ok {
		t.Error("health response should include 'requests' when WithMetrics is set")
	}
}

func TestService_HealthEndpoint_WithMetricsIncludesZeroRequests(t *testing.T) {
	var m endpoint.Metrics
	_, ts := newSvc(t, kit.WithMetrics(&m))

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	var health map[string]any
	json.NewDecoder(resp.Body).Decode(&health) //nolint:errcheck
	if got, ok := health["requests"]; !ok {
		t.Fatal("health response should include 'requests' when WithMetrics is set")
	} else if got != float64(0) {
		t.Fatalf("requests: got %v, want 0", got)
	}
}

func TestService_LivezReadyz_DefaultOK(t *testing.T) {
	_, ts := newSvc(t)

	for _, path := range []string{"/livez", "/readyz"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status: got %d, want %d", path, resp.StatusCode, http.StatusOK)
		}
		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body) //nolint:errcheck
		if body["status"] != "ok" {
			t.Fatalf("%s status body: got %v", path, body["status"])
		}
	}
}

func TestService_ReadyzReportsReadinessFailure(t *testing.T) {
	_, ts := newSvc(t, kit.WithReadinessCheck("db", func(context.Context) error {
		return errors.New("db unavailable")
	}))

	resp, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	var body struct {
		Status string `json:"status"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode health body: %v", err)
	}
	if body.Status != "unavailable" {
		t.Fatalf("health status: got %q, want unavailable", body.Status)
	}
	if len(body.Checks) != 1 || body.Checks[0].Name != "db" || body.Checks[0].Status != "error" {
		t.Fatalf("checks: got %#v", body.Checks)
	}
	if body.Checks[0].Error != "check failed" {
		t.Fatalf("check error: got %q, want check failed", body.Checks[0].Error)
	}
}

func TestService_ReadyzTimesOutSlowReadinessCheck(t *testing.T) {
	_, ts := newSvc(t,
		kit.WithHealthCheckTimeout(10*time.Millisecond),
		kit.WithReadinessCheck("db", func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}),
	)

	resp, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	var body struct {
		Checks []struct {
			Error string `json:"error"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode health body: %v", err)
	}
	if len(body.Checks) != 1 || body.Checks[0].Error != "check timed out" {
		t.Fatalf("checks: got %#v", body.Checks)
	}
}

func TestService_LivezIgnoresReadinessFailure(t *testing.T) {
	_, ts := newSvc(t, kit.WithReadinessCheck("db", func(context.Context) error {
		return errors.New("db unavailable")
	}))

	resp, err := http.Get(ts.URL + "/livez")
	if err != nil {
		t.Fatalf("GET /livez: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestService_HealthIncludesLivenessAndReadiness(t *testing.T) {
	_, ts := newSvc(t,
		kit.WithLivenessCheck("process", kit.Healthy),
		kit.WithReadinessCheck("db", kit.Healthy),
	)

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body struct {
		Checks []struct {
			Name string `json:"name"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode health body: %v", err)
	}
	if len(body.Checks) != 2 {
		t.Fatalf("checks len: got %d, want 2", len(body.Checks))
	}
}

func TestService_HealthChecksRunConcurrently(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	check := func(name string) kit.HealthCheck {
		return func(ctx context.Context) error {
			started <- name
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	_, ts := newSvc(t,
		kit.WithHealthCheckTimeout(time.Second),
		kit.WithReadinessCheck("db", check("db")),
		kit.WithReadinessCheck("cache", check("cache")),
	)

	type response struct {
		resp *http.Response
		err  error
	}
	done := make(chan response, 1)
	go func() {
		resp, err := http.Get(ts.URL + "/readyz")
		done <- response{resp: resp, err: err}
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case name := <-started:
			seen[name] = true
		case <-time.After(250 * time.Millisecond):
			close(release)
			t.Fatalf("checks did not start concurrently: %#v", seen)
		}
	}
	close(release)
	result := <-done
	if result.err != nil {
		t.Fatal(result.err)
	}
	defer result.resp.Body.Close()
	if result.resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", result.resp.StatusCode, http.StatusOK)
	}
}

func TestService_HealthCheckDoesNotOverlapAfterTimeout(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	_, ts := newSvc(t,
		kit.WithHealthCheckTimeout(10*time.Millisecond),
		kit.WithReadinessCheck("stuck", func(context.Context) error {
			if calls.Add(1) == 1 {
				close(started)
			}
			<-release
			return nil
		}),
	)

	first, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	<-started

	second, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	defer second.Body.Close()
	close(release)
	if calls.Load() != 1 {
		t.Fatalf("check calls = %d, want 1", calls.Load())
	}
	var body struct {
		Checks []struct {
			Error string `json:"error"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(second.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Checks) != 1 || body.Checks[0].Error != "check already running" {
		t.Fatalf("checks = %#v", body.Checks)
	}
}

// ── kit.JSON (package-level function) ────────────────────────────────────────

func TestKitJSON_Success(t *testing.T) {
	h := kit.JSON[helloReq](helloHandler)
	body, _ := json.Marshal(helloReq{Name: "World"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	var resp helloResp
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp.Message != "Hello, World!" {
		t.Errorf("message: got %q, want %q", resp.Message, "Hello, World!")
	}
}

func TestKitJSON_HandlerError_Returns500(t *testing.T) {
	h := kit.JSON[helloReq](helloHandler)
	body, _ := json.Marshal(helloReq{Name: ""})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestKitJSON_MultipleRequests(t *testing.T) {
	h := kit.JSON[helloReq](helloHandler)
	ts := httptest.NewServer(h)
	defer ts.Close()

	for _, name := range []string{"Alice", "Bob", "Charlie"} {
		body, _ := json.Marshal(helloReq{Name: name})
		resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		var result helloResp
		json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
		if result.Message != "Hello, "+name+"!" {
			t.Errorf("name=%s: got %q", name, result.Message)
		}
	}
}

// ── Handle / HandleFunc ───────────────────────────────────────────────────────

func TestService_Handle(t *testing.T) {
	svc := kit.MustNewHTTP(":0")
	svc.Handle("/hello", kit.JSON[helloReq](helloHandler))
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "Handle"})
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestService_HandleFunc(t *testing.T) {
	svc := kit.MustNewHTTP(":0")
	svc.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("pong")) //nolint:errcheck
	})
	ts := httptest.NewServer(svc)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestService_HandleFunc_DoesNotApplyEndpointMiddleware(t *testing.T) {
	var m endpoint.Metrics
	svc := kit.MustNewHTTP(":0", kit.WithMetrics(&m))
	svc.HandleFunc("/plain", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "plain failure", http.StatusInternalServerError)
	})
	ts := httptest.NewServer(svc)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/plain")
	if err != nil {
		t.Fatalf("GET /plain: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if got := m.Snapshot().RequestCount; got != 0 {
		t.Fatalf("endpoint metrics should not count plain HTTP handlers, got %d", got)
	}
}

// ── HandleJSON ───────────────────────────────────────────────────────────────

func TestService_HandleJSON(t *testing.T) {
	svc := kit.MustNewHTTP(":0")
	kit.HandleJSON[helloReq](svc, "/hello", helloHandler)
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "HandleJSON"})
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestService_HandleJSONTyped(t *testing.T) {
	svc := kit.MustNewHTTP(":0")
	kit.HandleJSONTyped(svc, "/hello", func(_ context.Context, req helloReq) (helloResp, error) {
		return helloResp{Message: "Hello, " + req.Name + "!"}, nil
	})
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "Typed"})
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	var result helloResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Message != "Hello, Typed!" {
		t.Fatalf("message: got %q, want %q", result.Message, "Hello, Typed!")
	}
}

func TestJSONTyped(t *testing.T) {
	h := kit.JSONTyped(func(_ context.Context, req helloReq) (helloResp, error) {
		return helloResp{Message: req.Name}, nil
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"name":"standalone"}`))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	var result helloResp
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Message != "standalone" {
		t.Fatalf("message: got %q, want standalone", result.Message)
	}
}

func TestService_HandleJSON_AppliesEndpointMiddlewareToBusinessErrors(t *testing.T) {
	var m endpoint.Metrics
	svc := kit.MustNewHTTP(":0", kit.WithMetrics(&m))
	kit.HandleJSON[helloReq](svc, "/hello", helloHandler)
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{})
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	snapshot := m.Snapshot()
	if snapshot.RequestCount != 1 {
		t.Errorf("RequestCount: got %d, want 1", snapshot.RequestCount)
	}
	if snapshot.SuccessCount != 0 {
		t.Errorf("SuccessCount: got %d, want 0", snapshot.SuccessCount)
	}
	if snapshot.ErrorCount != 1 {
		t.Errorf("ErrorCount: got %d, want 1", snapshot.ErrorCount)
	}
}

func TestService_HandleJSON_UsesStrictDecode(t *testing.T) {
	called := false
	svc := kit.MustNewHTTP(":0")
	kit.HandleJSON[helloReq](svc, "/hello", func(_ context.Context, _ helloReq) (any, error) {
		called = true
		return helloResp{Message: "ok"}, nil
	})
	ts := httptest.NewServer(svc)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewBufferString(`{"name":"x","extra":true}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if called {
		t.Fatal("handler should not run for invalid strict JSON")
	}
}

func TestService_HandleJSON_WithRequestID(t *testing.T) {
	svc := kit.MustNewHTTP(":0", kit.WithRequestID())
	kit.HandleJSON[helloReq](svc, "/id", func(ctx context.Context, req helloReq) (any, error) {
		return map[string]string{
			"id":      endpoint.RequestIDFromContext(ctx),
			"message": req.Name,
		}, nil
	})
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "rid"})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/id", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "incoming-id")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("X-Request-ID"); got != "incoming-id" {
		t.Fatalf("response header: got %q, want %q", got, "incoming-id")
	}

	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got := payload["id"]; got != "incoming-id" {
		t.Fatalf("request id in context: got %q, want %q", got, "incoming-id")
	}
}

// ── Start / Shutdown ──────────────────────────────────────────────────────────

func TestService_StartShutdown(t *testing.T) {
	svc := kit.MustNewHTTP(":0")
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestService_ShutdownWithoutStart(t *testing.T) {
	svc := kit.MustNewHTTP(":0")
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown without Start: %v", err)
	}
}

// ── WithMetrics ───────────────────────────────────────────────────────────────

func TestService_WithMetrics_TracksRequests(t *testing.T) {
	var m endpoint.Metrics
	_, ts := newSvc(t, kit.WithMetrics(&m))

	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(helloReq{Name: "test"})
		http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body)) //nolint:errcheck
	}
	snapshot := m.Snapshot()
	if snapshot.RequestCount != 3 {
		t.Errorf("RequestCount: got %d, want 3", snapshot.RequestCount)
	}
	if snapshot.SuccessCount != 3 {
		t.Errorf("SuccessCount: got %d, want 3", snapshot.SuccessCount)
	}
}

// ── WithTimeout ───────────────────────────────────────────────────────────────

func TestService_WithTimeout_CancelsSlowHandler(t *testing.T) {
	svc := kit.MustNewHTTP(":0", kit.WithTimeout(20*time.Millisecond))
	kit.HandleJSON[helloReq](svc, "/slow", func(ctx context.Context, _ helloReq) (any, error) {
		select {
		case <-time.After(5 * time.Second):
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "x"})
	resp, err := http.Post(ts.URL+"/slow", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("expected non-200 for timed-out request")
	}
}

// ── WithRequestID ─────────────────────────────────────────────────────────────

func TestService_WithRequestID(t *testing.T) {
	svc := kit.MustNewHTTP(":0", kit.WithRequestID())
	kit.HandleJSON[helloReq](svc, "/hello", helloHandler)
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "rid"})
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("X-Request-ID"); got == "" {
		t.Error("expected generated X-Request-ID response header")
	}
}

func TestService_WithRequestID_PreservesIncomingHeader(t *testing.T) {
	svc := kit.MustNewHTTP(":0", kit.WithRequestID())
	kit.HandleJSON[helloReq](svc, "/id", func(ctx context.Context, req helloReq) (any, error) {
		return map[string]string{
			"id":      endpoint.RequestIDFromContext(ctx),
			"message": req.Name,
		}, nil
	})
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "rid"})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/id", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "incoming-id")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("X-Request-ID"); got != "incoming-id" {
		t.Fatalf("response header: got %q, want %q", got, "incoming-id")
	}

	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got := payload["id"]; got != "incoming-id" {
		t.Fatalf("request id in context: got %q, want %q", got, "incoming-id")
	}
}

func TestService_WithRequestID_RejectsInvalidIncomingHeader(t *testing.T) {
	svc := kit.MustNewHTTP(":0", kit.WithRequestID())
	kit.HandleJSON[helloReq](svc, "/id", func(ctx context.Context, _ helloReq) (any, error) {
		return map[string]string{"id": endpoint.RequestIDFromContext(ctx)}, nil
	})
	ts := httptest.NewServer(svc)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/id", strings.NewReader(`{"name":"rid"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "untrusted request id")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got := resp.Header.Get("X-Request-ID")
	if got == "" || got == "untrusted request id" || !kit.DefaultRequestIDValidator(got) {
		t.Fatalf("generated request ID = %q", got)
	}
}

func TestService_WithRequestIDValidator_IsOrderIndependent(t *testing.T) {
	svc := kit.MustNewHTTP(":0",
		kit.WithRequestID(),
		kit.WithRequestIDValidator(func(id string) bool { return id == "tenant/request" }),
	)
	kit.HandleJSON[helloReq](svc, "/id", func(ctx context.Context, _ helloReq) (any, error) {
		return map[string]string{"id": endpoint.RequestIDFromContext(ctx)}, nil
	})
	ts := httptest.NewServer(svc)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/id", strings.NewReader(`{"name":"rid"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "tenant/request")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("X-Request-ID"); got != "tenant/request" {
		t.Fatalf("request ID = %q", got)
	}
}

func TestDefaultRequestIDValidator(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"request-123_ABC.xyz:1", true},
		{"", false},
		{"contains space", false},
		{"contains/slash", false},
		{strings.Repeat("a", kit.MaxRequestIDLength+1), false},
		{"\xff", false},
	}
	for _, tt := range tests {
		if got := kit.DefaultRequestIDValidator(tt.id); got != tt.want {
			t.Errorf("DefaultRequestIDValidator(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestKitOptions_ReturnErrorsForInvalidConfiguration(t *testing.T) {
	httpTests := []struct {
		name   string
		option kit.Option
	}{
		{
			name:   "timeout <= 0",
			option: kit.WithTimeout(0),
		},
		{
			name:   "endpoint middleware nil",
			option: kit.WithEndpointMiddleware(nil),
		},
		{
			name:   "json max body bytes negative",
			option: kit.WithJSONMaxBodyBytes(-1),
		},
		{
			name:   "readiness check empty name",
			option: kit.WithReadinessCheck("", kit.Healthy),
		},
		{
			name:   "readiness check nil",
			option: kit.WithReadinessCheck("db", nil),
		},
		{
			name:   "liveness check empty name",
			option: kit.WithLivenessCheck("", kit.Healthy),
		},
		{
			name:   "liveness check nil",
			option: kit.WithLivenessCheck("process", nil),
		},
		{
			name:   "metrics nil",
			option: kit.WithMetrics(nil),
		},
		{
			name:   "request ID validator nil",
			option: kit.WithRequestIDValidator(nil),
		},
	}

	for _, tt := range httpTests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := kit.NewHTTP(":0", tt.option); err == nil {
				t.Fatal("expected invalid option error")
			}
		})
	}

	hostTests := []struct {
		name   string
		option kit.HostOption
	}{
		{
			name:   "lifecycle component nil",
			option: kit.WithLifecycle(nil),
		},
		{
			name:   "shutdown timeout <= 0",
			option: kit.WithShutdownTimeout(0),
		},
	}

	for _, tt := range hostTests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := kit.NewHost(tt.option); err == nil {
				t.Fatal("expected invalid option error")
			}
		})
	}
}

// ── Three-layer architecture pattern ─────────────────────────────────────────
//
// These tests verify the recommended pattern for using kit with the
// Service/Endpoint/Transport separation described in README.md.

// userService is a minimal Service-layer implementation (pure business logic,
// no framework imports).
type userService struct{}

type createUserReq struct {
	Name string `json:"name"`
}
type createUserResp struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (s *userService) CreateUser(_ context.Context, req createUserReq) (createUserResp, error) {
	if req.Name == "" {
		return createUserResp{}, errors.New("name required")
	}
	return createUserResp{ID: 1, Name: req.Name}, nil
}

// TestThreeLayer_ServiceEndpointTransport verifies the full three-layer pattern:
//   - Service: pure business logic, no framework dependency
//   - Endpoint: kit.HandleJSON wraps the service method with middleware
//   - Transport: JSON decoding/encoding is applied at the HTTP boundary
func TestThreeLayer_ServiceEndpointTransport(t *testing.T) {
	// Service layer — pure business logic
	svc := &userService{}

	// Endpoint + Transport layer — kit.HandleJSON registers the service method
	// and applies service-level middleware (metrics, timeout, etc.)
	var m endpoint.Metrics
	service := kit.MustNewHTTP(":0", kit.WithMetrics(&m))
	kit.HandleJSON[createUserReq](service, "/users", func(ctx context.Context, req createUserReq) (any, error) {
		return svc.CreateUser(ctx, req)
	})

	ts := httptest.NewServer(service)
	defer ts.Close()

	// Verify the handler works end-to-end
	body, _ := json.Marshal(createUserReq{Name: "Alice"})
	resp, err := http.Post(ts.URL+"/users", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /users: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var result createUserResp
	json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
	if result.Name != "Alice" {
		t.Errorf("name: got %q, want %q", result.Name, "Alice")
	}
	// Middleware (metrics) applied via svc.Handle
	if got := m.Snapshot().RequestCount; got != 1 {
		t.Errorf("RequestCount: got %d, want 1", got)
	}
}

// TestThreeLayer_ServiceIsolation verifies the Service layer can be tested
// completely independently of HTTP/transport concerns.
func TestThreeLayer_ServiceIsolation(t *testing.T) {
	svc := &userService{}

	// Test service directly — no HTTP, no framework
	resp, err := svc.CreateUser(context.Background(), createUserReq{Name: "Bob"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if resp.Name != "Bob" {
		t.Errorf("name: got %q, want %q", resp.Name, "Bob")
	}

	_, err = svc.CreateUser(context.Background(), createUserReq{})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

// TestThreeLayer_EndpointMiddlewareComposition verifies that endpoint-level
// middleware (the "How" layer) composes correctly around service logic.
func TestThreeLayer_EndpointMiddlewareComposition(t *testing.T) {
	svc := &userService{}

	// Build an endpoint from the service method
	var m endpoint.Metrics
	ep := endpoint.NewBuilder(
		endpoint.Endpoint(func(ctx context.Context, req any) (any, error) {
			return svc.CreateUser(ctx, req.(createUserReq))
		}),
	).
		WithMetrics(&m).
		WithErrorHandling("CreateUser").
		WithTimeout(5 * time.Second).
		Build()

	// Call the endpoint directly (no HTTP)
	resp, err := ep(context.Background(), createUserReq{Name: "Carol"})
	if err != nil {
		t.Fatalf("endpoint call: %v", err)
	}
	if resp.(createUserResp).Name != "Carol" {
		t.Errorf("name: got %q, want %q", resp.(createUserResp).Name, "Carol")
	}
	if got := m.Snapshot().RequestCount; got != 1 {
		t.Errorf("RequestCount: got %d, want 1", got)
	}
}

// TestKitJSON_IsTypedHTTPHandler verifies that kit.JSON[Req] produces a
// properly typed http.Handler that decodes JSON into Req and encodes the
// response as JSON — this is the Transport layer.
func TestKitJSON_IsTypedHTTPHandler(t *testing.T) {
	svc := &userService{}

	// kit.JSON[Req] is the Transport layer: it handles JSON decode/encode
	h := kit.JSON[createUserReq](func(ctx context.Context, req createUserReq) (any, error) {
		return svc.CreateUser(ctx, req)
	})

	// Verify it's an http.Handler
	var _ http.Handler = h

	// Verify it decodes JSON correctly
	body, _ := json.Marshal(createUserReq{Name: "Dave"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	var result createUserResp
	json.NewDecoder(w.Body).Decode(&result) //nolint:errcheck
	if result.Name != "Dave" {
		t.Errorf("name: got %q, want %q", result.Name, "Dave")
	}
}
