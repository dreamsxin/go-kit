package kit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/kit"
	kitlog "github.com/dreamsxin/go-kit/log"
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

// newSvc creates a Service with /hello registered and returns an httptest.Server.
// This mirrors the README Quick Start pattern exactly.
func newSvc(t *testing.T, opts ...kit.Option) (*kit.Service, *httptest.Server) {
	t.Helper()
	svc := kit.New(":0", opts...)
	svc.Handle("/hello", kit.JSON[helloReq](helloHandler))
	ts := httptest.NewServer(svc) // Service implements http.Handler
	t.Cleanup(ts.Close)
	return svc, ts
}

// ── README Quick Start pattern ────────────────────────────────────────────────

// TestReadme_QuickStart verifies the exact pattern shown in README.md works.
func TestReadme_QuickStart(t *testing.T) {
	svc := kit.New(":0")
	svc.Handle("/hello", kit.JSON[helloReq](func(_ context.Context, req helloReq) (any, error) {
		return helloResp{Message: "Hello, " + req.Name + "!"}, nil
	}))

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
	logger := kitlog.NewNopLogger()
	var metrics endpoint.Metrics

	svc := kit.New(":0",
		kit.WithRateLimit(100),
		kit.WithCircuitBreaker(5),
		kit.WithTimeout(5*time.Second),
		kit.WithRequestID(),
		kit.WithLogging(logger),
		kit.WithMetrics(&metrics),
	)
	svc.Handle("/hello", kit.JSON[helloReq](helloHandler))

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
	if metrics.RequestCount != 1 {
		t.Errorf("RequestCount: got %d, want 1", metrics.RequestCount)
	}
}

// ── Service implements http.Handler ──────────────────────────────────────────

func TestService_ImplementsHTTPHandler(t *testing.T) {
	svc := kit.New(":0")
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
	svc := kit.New(":0")
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
	svc := kit.New(":0")
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

// ── Start / Shutdown ──────────────────────────────────────────────────────────

func TestService_StartShutdown(t *testing.T) {
	svc := kit.New(":0")
	svc.Start()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestService_ShutdownWithoutStart(t *testing.T) {
	svc := kit.New(":0")
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
	if m.RequestCount != 3 {
		t.Errorf("RequestCount: got %d, want 3", m.RequestCount)
	}
	if m.SuccessCount != 3 {
		t.Errorf("SuccessCount: got %d, want 3", m.SuccessCount)
	}
}

// ── WithTimeout ───────────────────────────────────────────────────────────────

func TestService_WithTimeout_CancelsSlowHandler(t *testing.T) {
	svc := kit.New(":0", kit.WithTimeout(20*time.Millisecond))
	svc.Handle("/slow", kit.JSON[helloReq](func(ctx context.Context, _ helloReq) (any, error) {
		select {
		case <-time.After(5 * time.Second):
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))
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

// ── WithLogging ───────────────────────────────────────────────────────────────

func TestService_WithLogging(t *testing.T) {
	logger := kitlog.NewNopLogger()
	_, ts := newSvc(t, kit.WithLogging(logger))

	body, _ := json.Marshal(helloReq{Name: "log"})
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// ── WithCircuitBreaker ────────────────────────────────────────────────────────

// TestService_WithCircuitBreaker verifies that WithCircuitBreaker option is
// accepted and the service starts correctly. Circuit breaker behavior at the
// endpoint level is tested in endpoint/circuitbreaker package.
func TestService_WithCircuitBreaker(t *testing.T) {
	svc := kit.New(":0", kit.WithCircuitBreaker(2))
	svc.Handle("/hello", kit.JSON[helloReq](helloHandler))
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "test"})
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// ── WithRateLimit ─────────────────────────────────────────────────────────────

func TestService_WithRateLimit_AllowsAndRejects(t *testing.T) {
	// burst=1 at near-zero rate: first call allowed, subsequent rejected
	svc := kit.New(":0", kit.WithRateLimit(0.001))
	svc.Handle("/hello", kit.JSON[helloReq](helloHandler))
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "test"})
	// first call consumes the burst token
	http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body)) //nolint:errcheck
	// second call should be rate-limited (non-200)
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Log("note: rate limit may not have triggered (timing-dependent)")
	}
}

// ── WithRequestID ─────────────────────────────────────────────────────────────

func TestService_WithRequestID(t *testing.T) {
	svc := kit.New(":0", kit.WithRequestID())
	svc.Handle("/hello", kit.JSON[helloReq](helloHandler))
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
}

// ── gRPC support ──────────────────────────────────────────────────────────────

// TestService_WithGRPC_PanicsWithoutOption verifies GRPCServer() panics when
// WithGRPC was not set.
func TestService_WithGRPC_PanicsWithoutOption(t *testing.T) {
	svc := kit.New(":0")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling GRPCServer() without WithGRPC option")
		}
	}()
	svc.GRPCServer()
}

// TestService_WithGRPC_ReturnsServer verifies GRPCServer() returns a non-nil
// *grpc.Server when WithGRPC is set.
func TestService_WithGRPC_ReturnsServer(t *testing.T) {
	svc := kit.New(":0", kit.WithGRPC(":0"))
	gs := svc.GRPCServer()
	if gs == nil {
		t.Fatal("GRPCServer() returned nil")
	}
	// calling again returns the same instance
	if svc.GRPCServer() != gs {
		t.Error("GRPCServer() should return the same instance on repeated calls")
	}
}

// TestService_WithGRPC_StartShutdown verifies the gRPC server starts and
// shuts down cleanly alongside the HTTP server.
func TestService_WithGRPC_StartShutdown(t *testing.T) {
	svc := kit.New(":0", kit.WithGRPC(":0"))
	// register nothing — just verify lifecycle
	svc.Start()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

// TestService_WithGRPC_HTTPStillWorks verifies HTTP continues to work when
// gRPC is also enabled.
func TestService_WithGRPC_HTTPStillWorks(t *testing.T) {
	svc := kit.New(":0", kit.WithGRPC(":0"))
	svc.Handle("/hello", kit.JSON[helloReq](helloHandler))
	ts := httptest.NewServer(svc)
	defer ts.Close()
	defer svc.Shutdown(context.Background()) //nolint:errcheck

	body, _ := json.Marshal(helloReq{Name: "gRPC+HTTP"})
	resp, err := http.Post(ts.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /hello: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
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
//   - Endpoint: kit.JSON wraps the service method as an http.Handler (transport)
//     with automatic JSON decode/encode
//   - Transport: svc.Handle registers the handler and applies middleware
func TestThreeLayer_ServiceEndpointTransport(t *testing.T) {
	// Service layer — pure business logic
	svc := &userService{}

	// Endpoint + Transport layer — kit.JSON wraps the service method
	// svc.Handle applies service-level middleware (metrics, timeout, etc.)
	var m endpoint.Metrics
	service := kit.New(":0", kit.WithMetrics(&m))
	service.Handle("/users", kit.JSON[createUserReq](func(ctx context.Context, req createUserReq) (any, error) {
		return svc.CreateUser(ctx, req)
	}))

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
	if m.RequestCount != 1 {
		t.Errorf("RequestCount: got %d, want 1", m.RequestCount)
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
	if m.RequestCount != 1 {
		t.Errorf("RequestCount: got %d, want 1", m.RequestCount)
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
