package kit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// HealthCheck reports whether a runtime dependency is healthy.
type HealthCheck func(context.Context) error

// DefaultHealthCheckTimeout is the per-check timeout used by /health, /livez,
// and /readyz unless WithHealthCheckTimeout overrides it.
const DefaultHealthCheckTimeout = 2 * time.Second

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
		status, results := runHealthChecks(r.Context(), checks, s.healthTimeout)
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

func runHealthChecks(ctx context.Context, checks []namedHealthCheck, timeout time.Duration) (string, []healthCheckResult) {
	if len(checks) == 0 {
		return "ok", nil
	}
	results := make([]healthCheckResult, 0, len(checks))
	status := "ok"
	for _, hc := range checks {
		result := healthCheckResult{Name: hc.name, Status: "ok"}
		if err := runHealthCheck(ctx, hc.check, timeout); err != nil {
			status = "unavailable"
			result.Status = "error"
			result.Error = healthCheckErrorMessage(ctx, err)
		}
		results = append(results, result)
	}
	return status, results
}

func healthCheckErrorMessage(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "check timed out"
	}
	return "check failed"
}

func runHealthCheck(ctx context.Context, check HealthCheck, timeout time.Duration) error {
	if timeout <= 0 {
		return check(ctx)
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- check(checkCtx)
	}()

	select {
	case err := <-done:
		return err
	case <-checkCtx.Done():
		return checkCtx.Err()
	}
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
