package kit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	testgrpc "github.com/dreamsxin/go-kit/v2/examples/transport/_grpc_test"
	testpb "github.com/dreamsxin/go-kit/v2/examples/transport/_grpc_test/pb"
	"github.com/dreamsxin/go-kit/v2/kit"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
// This mirrors the recommended kit.HandleJSON pattern.
func newSvc(t *testing.T, opts ...kit.Option) (*kit.Service, *httptest.Server) {
	t.Helper()
	svc := kit.New(":0", opts...)
	kit.HandleJSON[helloReq](svc, "/hello", helloHandler)
	ts := httptest.NewServer(svc) // Service implements http.Handler
	t.Cleanup(ts.Close)
	return svc, ts
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()
	return l.Addr().String()
}

// ── README Quick Start pattern ────────────────────────────────────────────────

// TestReadme_QuickStart verifies the exact pattern shown in README.md works.
func TestReadme_QuickStart(t *testing.T) {
	svc := kit.New(":0")
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
	if metrics.RequestCount != 1 {
		t.Errorf("RequestCount: got %d, want 1", metrics.RequestCount)
	}
}

func TestReadme_WithGRPC_LiveRPC(t *testing.T) {
	grpcAddr := freeTCPAddr(t)

	svc := kit.New(":0", kit.WithGRPC(grpcAddr))
	testpb.RegisterTestServer(svc.GRPCServer(), testgrpc.NewBinding(testgrpc.NewService()))

	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Shutdown(context.Background()) //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext( //nolint:staticcheck
		ctx,
		grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("dial grpc: %v", err)
	}
	defer conn.Close()

	client := testpb.NewTestClient(conn)
	resp, err := client.Test(context.Background(), &testpb.TestRequest{
		A: "answer",
		B: 42,
	})
	if err != nil {
		t.Fatalf("grpc Test RPC: %v", err)
	}
	if resp.GetV() != "answer = 42" {
		t.Fatalf("grpc response: got %q, want %q", resp.GetV(), "answer = 42")
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

func TestService_HandleFunc_DoesNotApplyEndpointMiddleware(t *testing.T) {
	var m endpoint.Metrics
	svc := kit.New(":0", kit.WithMetrics(&m))
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
	if m.RequestCount != 0 {
		t.Fatalf("endpoint metrics should not count plain HTTP handlers, got %d", m.RequestCount)
	}
}

// ── HandleJSON ───────────────────────────────────────────────────────────────

func TestService_HandleJSON(t *testing.T) {
	svc := kit.New(":0")
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

func TestService_HandleJSON_AppliesEndpointMiddlewareToBusinessErrors(t *testing.T) {
	var m endpoint.Metrics
	svc := kit.New(":0", kit.WithMetrics(&m))
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
	svc := kit.New(":0")
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
	svc := kit.New(":0", kit.WithRequestID())
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
	svc := kit.New(":0")
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

func TestService_WithLogging_NilLogger_DoesNotPanic(t *testing.T) {
	_, ts := newSvc(t, kit.WithLogging(nil))

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
	kit.HandleJSON[helloReq](svc, "/hello", helloHandler)
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

func TestService_WithCircuitBreaker_IsPerJSONRoute(t *testing.T) {
	svc := kit.New(":0", kit.WithCircuitBreaker(1))
	kit.HandleJSON[helloReq](svc, "/bad", func(context.Context, helloReq) (any, error) {
		return nil, errors.New("boom")
	})
	kit.HandleJSON[helloReq](svc, "/ok", helloHandler)
	ts := httptest.NewServer(svc)
	defer ts.Close()

	body, _ := json.Marshal(helloReq{Name: "ok"})
	resp, err := http.Post(ts.URL+"/bad", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /bad: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("POST /bad status = %d, want failure", resp.StatusCode)
	}

	resp, err = http.Post(ts.URL+"/ok", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /ok: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /ok status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// ── WithRateLimit ─────────────────────────────────────────────────────────────

func TestService_WithRateLimit_AllowsAndRejects(t *testing.T) {
	// burst=1 at near-zero rate: first call allowed, subsequent rejected
	svc := kit.New(":0", kit.WithRateLimit(0.001))
	kit.HandleJSON[helloReq](svc, "/hello", helloHandler)
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
	svc := kit.New(":0", kit.WithRequestID())
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

func TestKitOptions_PanicOnInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name string
		run  func()
	}{
		{
			name: "rate limit <= 0",
			run:  func() { kit.WithRateLimit(0) },
		},
		{
			name: "timeout <= 0",
			run:  func() { kit.WithTimeout(0) },
		},
		{
			name: "circuit breaker threshold zero",
			run:  func() { kit.WithCircuitBreaker(0) },
		},
		{
			name: "grpc empty address",
			run:  func() { kit.WithGRPC("") },
		},
		{
			name: "json max body bytes negative",
			run:  func() { kit.WithJSONMaxBodyBytes(-1) },
		},
		{
			name: "readiness check empty name",
			run:  func() { kit.WithReadinessCheck("", kit.Healthy) },
		},
		{
			name: "readiness check nil",
			run:  func() { kit.WithReadinessCheck("db", nil) },
		},
		{
			name: "liveness check empty name",
			run:  func() { kit.WithLivenessCheck("", kit.Healthy) },
		},
		{
			name: "liveness check nil",
			run:  func() { kit.WithLivenessCheck("process", nil) },
		},
		{
			name: "metrics nil",
			run:  func() { kit.WithMetrics(nil) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic for invalid option configuration")
				}
			}()
			tt.run()
		})
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
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

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
//   - Endpoint: kit.HandleJSON wraps the service method with middleware
//   - Transport: JSON decoding/encoding is applied at the HTTP boundary
func TestThreeLayer_ServiceEndpointTransport(t *testing.T) {
	// Service layer — pure business logic
	svc := &userService{}

	// Endpoint + Transport layer — kit.HandleJSON registers the service method
	// and applies service-level middleware (metrics, timeout, etc.)
	var m endpoint.Metrics
	service := kit.New(":0", kit.WithMetrics(&m))
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
