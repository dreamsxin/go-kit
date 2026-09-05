package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/health"
)

func TestRegistryWithoutChecksIsOK(t *testing.T) {
	registry := health.NewRegistry()
	report := registry.Report(context.Background(), health.ScopeAll)
	if report.Status != health.StatusOK {
		t.Fatalf("status = %q, want %q", report.Status, health.StatusOK)
	}
	if len(report.Checks) != 0 {
		t.Fatalf("checks = %d, want 0", len(report.Checks))
	}
}

// TestScopesAreSeparate is the distinction the two probes exist for: a
// dependency that is down should stop traffic, not restart the process.
func TestScopesAreSeparate(t *testing.T) {
	registry := health.NewRegistry()
	mustAdd(t, registry.AddLiveness("self", health.Healthy))
	mustAdd(t, registry.AddReadiness("database", func(context.Context) error {
		return errors.New("connection refused")
	}))

	if got := registry.Report(context.Background(), health.ScopeLiveness).Status; got != health.StatusOK {
		t.Errorf("liveness status = %q, want %q", got, health.StatusOK)
	}
	if got := registry.Report(context.Background(), health.ScopeReadiness).Status; got != health.StatusUnavailable {
		t.Errorf("readiness status = %q, want %q", got, health.StatusUnavailable)
	}
	if got := registry.Report(context.Background(), health.ScopeAll).Status; got != health.StatusUnavailable {
		t.Errorf("combined status = %q, want %q", got, health.StatusUnavailable)
	}
}

// TestFailureReportsAFixedPhrase keeps a dependency's own error out of an
// unauthenticated response.
func TestFailureReportsAFixedPhrase(t *testing.T) {
	registry := health.NewRegistry()
	mustAdd(t, registry.AddReadiness("database", func(context.Context) error {
		return errors.New("dial tcp 10.0.0.7:5432: connection refused")
	}))

	report := registry.Report(context.Background(), health.ScopeReadiness)
	if len(report.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(report.Checks))
	}
	if got := report.Checks[0].Error; got != "check failed" {
		t.Fatalf("error = %q, want %q", got, "check failed")
	}
}

func TestSlowCheckTimesOut(t *testing.T) {
	registry := health.NewRegistry(health.WithTimeout(10 * time.Millisecond))
	mustAdd(t, registry.AddReadiness("slow", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}))

	report := registry.Report(context.Background(), health.ScopeReadiness)
	if report.Status != health.StatusUnavailable {
		t.Fatalf("status = %q, want %q", report.Status, health.StatusUnavailable)
	}
	if got := report.Checks[0].Error; got != "check timed out" {
		t.Fatalf("error = %q, want %q", got, "check timed out")
	}
}

// TestPanickingCheckIsReportedNotPropagated: a probe is an unauthenticated
// request, so a buggy check must not become a remote kill switch.
func TestPanickingCheckIsReportedNotPropagated(t *testing.T) {
	registry := health.NewRegistry()
	mustAdd(t, registry.AddReadiness("buggy", func(context.Context) error {
		panic("nil map write")
	}))

	report := registry.Report(context.Background(), health.ScopeReadiness)
	if report.Status != health.StatusUnavailable {
		t.Fatalf("status = %q, want %q", report.Status, health.StatusUnavailable)
	}
	if got := report.Checks[0].Error; got != "check panicked" {
		t.Fatalf("error = %q, want %q", got, "check panicked")
	}
}

// TestSecondProbeDoesNotStartASecondCheck: probes arrive on a schedule, and a
// dependency that is already struggling should not receive one connection
// attempt per probe interval.
func TestSecondProbeDoesNotStartASecondCheck(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	registry := health.NewRegistry()
	mustAdd(t, registry.AddReadiness("busy", func(context.Context) error {
		entered <- struct{}{}
		<-release
		return nil
	}))

	first := make(chan health.Report, 1)
	go func() { first <- registry.Report(context.Background(), health.ScopeReadiness) }()
	<-entered

	second := registry.Report(context.Background(), health.ScopeReadiness)
	if got := second.Checks[0].Error; got != "check already running" {
		t.Fatalf("error = %q, want %q", got, "check already running")
	}

	close(release)
	if report := <-first; report.Status != health.StatusOK {
		t.Fatalf("first status = %q, want %q", report.Status, health.StatusOK)
	}
}

func TestReportCarriesTheRequestCount(t *testing.T) {
	registry := health.NewRegistry(health.WithRequestCount(func() int64 { return 42 }))
	report := registry.Report(context.Background(), health.ScopeAll)
	if report.Requests == nil {
		t.Fatal("requests missing")
	}
	if *report.Requests != 42 {
		t.Fatalf("requests = %d, want 42", *report.Requests)
	}
}

func TestRegistryRejectsUnusableChecks(t *testing.T) {
	registry := health.NewRegistry()
	if err := registry.AddLiveness("", health.Healthy); err == nil {
		t.Error("expected an error for an empty name")
	}
	if err := registry.AddReadiness("database", nil); err == nil {
		t.Error("expected an error for a nil check")
	}
}

func TestHandlerStatusCodes(t *testing.T) {
	registry := health.NewRegistry()
	mustAdd(t, registry.AddReadiness("database", func(context.Context) error {
		return errors.New("down")
	}))

	mux := http.NewServeMux()
	registry.Mount(mux, health.DefaultPaths())

	for path, want := range map[string]int{
		"/livez":  http.StatusOK,
		"/readyz": http.StatusServiceUnavailable,
		"/health": http.StatusServiceUnavailable,
	} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != want {
			t.Errorf("%s status = %d, want %d", path, recorder.Code, want)
		}
		if got := recorder.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("%s content type = %q", path, got)
		}
		var report health.Report
		if err := json.NewDecoder(recorder.Body).Decode(&report); err != nil {
			t.Errorf("%s decode: %v", path, err)
		}
	}
}

// TestMountSkipsEmptyPaths lets a deployment that orchestrates on readiness
// alone expose readiness alone.
func TestMountSkipsEmptyPaths(t *testing.T) {
	registry := health.NewRegistry()
	mux := http.NewServeMux()
	registry.Mount(mux, health.Paths{Readiness: "/readyz"})

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("readiness status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("liveness status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

// TestChecksAddedAfterMountAreEvaluated: an assembly learns about a lifecycle
// component's readiness when the component is attached, which is after the
// registry is mounted.
func TestChecksAddedAfterMountAreEvaluated(t *testing.T) {
	registry := health.NewRegistry()
	mux := http.NewServeMux()
	registry.Mount(mux, health.DefaultPaths())

	mustAdd(t, registry.AddReadiness("late", func(context.Context) error {
		return errors.New("not ready")
	}))

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func mustAdd(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("add check: %v", err)
	}
}
