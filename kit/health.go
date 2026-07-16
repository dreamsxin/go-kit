package kit

import (
	"context"
	"encoding/json"
	"net/http"
)

// HealthCheck reports whether a runtime dependency is healthy.
type HealthCheck func(context.Context) error

type namedHealthCheck struct {
	name  string
	check HealthCheck
}

type healthResponse struct {
	Status   string              `json:"status"`
	Requests *int64              `json:"requests,omitempty"`
	Checks   []healthCheckResult `json:"checks,omitempty"`
}

type healthCheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (s *Service) registerHealthEndpoints() {
	s.mux.HandleFunc("/health", s.healthHandler(appendHealthChecks(s.livenessChecks, s.readinessChecks)))
	s.mux.HandleFunc("/livez", s.healthHandler(s.livenessChecks))
	s.mux.HandleFunc("/readyz", s.healthHandler(s.readinessChecks))
}

func (s *Service) healthHandler(checks []namedHealthCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, results := runHealthChecks(r.Context(), checks)
		resp := healthResponse{
			Status: status,
			Checks: results,
		}
		if s.metrics != nil {
			requests := s.metrics.Snapshot().RequestCount
			resp.Requests = &requests
		}

		w.Header().Set("Content-Type", "application/json")
		if status != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func runHealthChecks(ctx context.Context, checks []namedHealthCheck) (string, []healthCheckResult) {
	if len(checks) == 0 {
		return "ok", nil
	}
	results := make([]healthCheckResult, 0, len(checks))
	status := "ok"
	for _, hc := range checks {
		result := healthCheckResult{Name: hc.name, Status: "ok"}
		if err := hc.check(ctx); err != nil {
			status = "unavailable"
			result.Status = "error"
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return status, results
}

func appendHealthChecks(a, b []namedHealthCheck) []namedHealthCheck {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	out := make([]namedHealthCheck, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}
