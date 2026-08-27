package kit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// HealthCheck reports whether a runtime dependency is healthy. Implementations
// must stop promptly when ctx is canceled.
type HealthCheck func(context.Context) error

// DefaultHealthCheckTimeout is the per-check timeout used by /health, /livez,
// and /readyz unless WithHealthCheckTimeout overrides it.
const DefaultHealthCheckTimeout = 2 * time.Second

type namedHealthCheck struct {
	name  string
	check HealthCheck
	gate  chan struct{}
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

func (h *HTTP) registerHealthEndpoints() {
	h.mux.HandleFunc("/health", h.healthHandler(healthScopeAll))
	h.mux.HandleFunc("/livez", h.healthHandler(healthScopeLiveness))
	h.mux.HandleFunc("/readyz", h.healthHandler(healthScopeReadiness))
}

type healthScope int

const (
	healthScopeAll healthScope = iota
	healthScopeLiveness
	healthScopeReadiness
)

// snapshotChecks returns copies of the current checks for the scope. Checks
// are read under the mutex because Host may bridge lifecycle readiness after
// construction.
func (h *HTTP) snapshotChecks(scope healthScope) []namedHealthCheck {
	h.checksMu.Lock()
	defer h.checksMu.Unlock()
	var liveness, readiness []namedHealthCheck
	switch scope {
	case healthScopeAll:
		liveness = append(liveness, h.livenessChecks...)
		readiness = append(readiness, h.readinessChecks...)
	case healthScopeLiveness:
		liveness = append(liveness, h.livenessChecks...)
	case healthScopeReadiness:
		readiness = append(readiness, h.readinessChecks...)
	}
	return appendHealthChecks(liveness, readiness)
}

func (h *HTTP) healthHandler(scope healthScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, results := runHealthChecks(r.Context(), h.snapshotChecks(scope), h.healthTimeout)
		resp := healthResponse{
			Status: status,
			Checks: results,
		}
		if h.metrics != nil {
			requests := h.metrics.Snapshot().RequestCount
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
	checksCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		checksCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	type outcome struct {
		index int
		err   error
	}
	results := make([]healthCheckResult, len(checks))
	completed := make([]bool, len(checks))
	outcomes := make(chan outcome, len(checks))
	for i, hc := range checks {
		results[i] = healthCheckResult{Name: hc.name, Status: "ok"}
		go func(index int, check namedHealthCheck) {
			outcomes <- outcome{index: index, err: runHealthCheck(checksCtx, check)}
		}(i, hc)
	}

	remaining := len(checks)
	for remaining > 0 {
		select {
		case result := <-outcomes:
			remaining--
			completed[result.index] = true
			applyHealthCheckOutcome(checksCtx, &results[result.index], result.err)
		case <-checksCtx.Done():
			draining := true
			for draining {
				select {
				case result := <-outcomes:
					remaining--
					completed[result.index] = true
					applyHealthCheckOutcome(checksCtx, &results[result.index], result.err)
				default:
					draining = false
				}
			}
			for i := range results {
				if !completed[i] {
					results[i].Status = "error"
					results[i].Error = healthCheckErrorMessage(checksCtx, checksCtx.Err())
				}
			}
			return "unavailable", results
		}
	}

	for _, result := range results {
		if result.Status != "ok" {
			return "unavailable", results
		}
	}
	return "ok", results
}

func applyHealthCheckOutcome(ctx context.Context, result *healthCheckResult, err error) {
	if err == nil {
		return
	}
	result.Status = "error"
	result.Error = healthCheckErrorMessage(ctx, err)
}

func healthCheckErrorMessage(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "check timed out"
	}
	if errors.Is(err, errHealthCheckInProgress) {
		return "check already running"
	}
	return "check failed"
}

var errHealthCheckInProgress = errors.New("health check is already running")

func runHealthCheck(ctx context.Context, check namedHealthCheck) error {
	if check.gate != nil {
		select {
		case check.gate <- struct{}{}:
			defer func() { <-check.gate }()
		default:
			return errHealthCheckInProgress
		}
	}

	return check.check(ctx)
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
