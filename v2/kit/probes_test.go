package kit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
