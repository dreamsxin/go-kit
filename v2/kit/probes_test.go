package kit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/health"
	"github.com/dreamsxin/go-kit/v2/kit"
)

func TestProbesServeTheDefaultPaths(t *testing.T) {
	component := kit.MustNewHTTP(":0", kit.WithReadinessCheck("self", kit.Healthy))
	for _, path := range []string{"/health", "/livez", "/readyz"} {
		recorder := httptest.NewRecorder()
		component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("%s status = %d, want %d", path, recorder.Code, http.StatusOK)
		}
	}
}

// TestProbePathsAreConfigurable covers the deployment whose orchestrator probes
// routes of its own choosing.
func TestProbePathsAreConfigurable(t *testing.T) {
	component := kit.MustNewHTTP(":0", kit.WithProbePaths(health.Paths{Readiness: "/internal/ready"}))

	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/internal/ready", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("configured path status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("default path status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

// TestProbesCanLeaveTheTrafficPort: the registry is reachable, so an
// application can serve the probes on an administrative listener of its own.
func TestProbesCanLeaveTheTrafficPort(t *testing.T) {
	component := kit.MustNewHTTP(":0",
		kit.WithoutProbes(),
		kit.WithReadinessCheck("database", func(context.Context) error {
			return errors.New("down")
		}),
	)

	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("traffic port status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	admin := http.NewServeMux()
	component.Probes().Mount(admin, health.DefaultPaths())
	recorder = httptest.NewRecorder()
	admin.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("admin status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

// TestProbeReportCarriesTheRequestCount pins the wiring between the metrics
// collector and the probe payload.
func TestProbeReportCarriesTheRequestCount(t *testing.T) {
	component := kit.MustNewHTTP(":0", kit.WithMetrics(&endpoint.Metrics{}))
	report := component.Probes().Report(context.Background(), health.ScopeAll)
	if report.Requests == nil {
		t.Fatal("requests missing from the probe report")
	}
}

// warming is a lifecycle component that reports readiness, which is what a Host
// bridges into a probe surface.
type warming struct{ ready atomic.Bool }

func (w *warming) Name() string                    { return "warming" }
func (w *warming) Start() error                    { return nil }
func (w *warming) Errors() <-chan error            { return nil }
func (w *warming) Shutdown(context.Context) error  { return nil }
func (w *warming) Ready(context.Context) error {
	if w.ready.Load() {
		return nil
	}
	return errors.New("still warming up")
}

// TestHostRefusesReadinessWithNowhereToReport: the point of implementing
// ReadinessProvider is that an orchestrator can see the answer. Collecting the
// provider and discarding it, which is what a Host without a probe surface used
// to do, is worse than a component that never claimed to warm up.
func TestHostRefusesReadinessWithNowhereToReport(t *testing.T) {
	_, err := kit.NewHost(kit.WithLifecycle(&warming{}))
	if err == nil {
		t.Fatal("expected an error for a readiness provider with no probe surface")
	}
	if !strings.Contains(err.Error(), "readiness") {
		t.Fatalf("error = %v, want it to mention readiness", err)
	}
}

func TestHostBridgesReadinessIntoTheProbes(t *testing.T) {
	component := kit.MustNewHTTP(":0")
	warmup := &warming{}
	if _, err := kit.NewHost(kit.WithLifecycle(component), kit.WithLifecycle(warmup)); err != nil {
		t.Fatalf("NewHost: %v", err)
	}

	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status while warming = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	warmup.ready.Store(true)
	recorder = httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status once ready = %d, want %d", recorder.Code, http.StatusOK)
	}
}
