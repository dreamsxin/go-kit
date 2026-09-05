// Package health answers whether this process is alive and whether it is ready
// to serve.
//
// A Registry holds the checks and evaluates them; a transport mounts it. That
// split is the point: readiness is an operational property of the process, not
// of one protocol, so an HTTP service, a gRPC service, and a process serving
// both report it the same way.
//
// Liveness answers "should this process be restarted"; readiness answers
// "should it receive traffic". Keep dependency checks in readiness: a database
// that is briefly unreachable should stop traffic, not restart the process.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Check reports whether a runtime dependency is healthy. Implementations must
// stop promptly when ctx is canceled.
type Check func(context.Context) error

// Healthy is a Check that always passes. It is the honest answer for a process
// with no dependencies to report: the probe endpoint exists and the process is
// answering it.
func Healthy(context.Context) error { return nil }

// DefaultTimeout is the per-check timeout a Registry uses unless WithTimeout
// overrides it.
const DefaultTimeout = 2 * time.Second

// Scope selects which checks a report covers.
type Scope int

const (
	// ScopeAll covers liveness and readiness together.
	ScopeAll Scope = iota
	// ScopeLiveness covers liveness checks only.
	ScopeLiveness
	// ScopeReadiness covers readiness checks only.
	ScopeReadiness
)

// StatusOK and StatusUnavailable are the two values a report carries. A report
// is unavailable when any check in its scope failed, timed out, panicked, or was
// already running.
const (
	StatusOK          = "ok"
	StatusUnavailable = "unavailable"
)

// Report is the result of evaluating one scope. It is the JSON body the HTTP
// handler writes, so its shape is part of the contract an operator's probe
// configuration depends on.
type Report struct {
	Status string `json:"status"`
	// Requests is the number of requests the process has served, when the
	// assembly supplied a counter through WithRequestCount. It gives an
	// operator reading a probe response one number that says the process is
	// doing work rather than merely answering.
	Requests *int64 `json:"requests,omitempty"`
	Checks   []CheckResult `json:"checks,omitempty"`
}

// CheckResult is one check's outcome. Error is a fixed phrase rather than the
// check's own message: a probe endpoint is unauthenticated, so it says that a
// dependency is unhealthy without saying which host refused the connection.
type CheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Paths are the routes Mount registers.
type Paths struct {
	All       string
	Liveness  string
	Readiness string
}

// DefaultPaths are the conventional Kubernetes probe routes.
func DefaultPaths() Paths {
	return Paths{All: "/health", Liveness: "/livez", Readiness: "/readyz"}
}

// Option configures a Registry.
type Option func(*Registry)

// WithTimeout bounds how long one evaluation may take. A non-positive timeout
// means the caller's context is the only deadline.
func WithTimeout(timeout time.Duration) Option {
	return func(r *Registry) { r.timeout = timeout }
}

// WithRequestCount supplies the request counter reported alongside the checks.
// The function runs on the probe path, so it must not block.
func WithRequestCount(count func() int64) Option {
	return func(r *Registry) { r.requestCount = count }
}

// Registry holds a process's liveness and readiness checks.
//
// Checks may be added after construction, because an assembly learns about some
// of them later: a lifecycle component that reports readiness is registered when
// it is attached.
type Registry struct {
	timeout      time.Duration
	requestCount func() int64

	mu        sync.Mutex
	liveness  []namedCheck
	readiness []namedCheck
}

type namedCheck struct {
	name  string
	check Check
	// gate admits one evaluation of this check at a time. A probe that arrives
	// while the previous one is still running reports the check as busy instead
	// of starting a second connection attempt against a dependency that is
	// already struggling.
	gate chan struct{}
}

// NewRegistry creates an empty registry.
func NewRegistry(options ...Option) *Registry {
	registry := &Registry{timeout: DefaultTimeout}
	for _, option := range options {
		if option != nil {
			option(registry)
		}
	}
	return registry
}

// AddLiveness registers a liveness check. A failing liveness check means the
// process should be restarted.
func (r *Registry) AddLiveness(name string, check Check) error {
	entry, err := newNamedCheck(name, check)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.liveness = append(r.liveness, entry)
	return nil
}

// AddReadiness registers a readiness check. A failing readiness check means the
// process should stop receiving traffic.
func (r *Registry) AddReadiness(name string, check Check) error {
	entry, err := newNamedCheck(name, check)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readiness = append(r.readiness, entry)
	return nil
}

func newNamedCheck(name string, check Check) (namedCheck, error) {
	if name == "" {
		return namedCheck{}, errors.New("health: check name cannot be empty")
	}
	if check == nil {
		return namedCheck{}, fmt.Errorf("health: check %q cannot be nil", name)
	}
	return namedCheck{name: name, check: check, gate: make(chan struct{}, 1)}, nil
}

// Report evaluates the checks in scope. Checks run concurrently, so one slow
// dependency does not add its latency to the others.
func (r *Registry) Report(ctx context.Context, scope Scope) Report {
	status, results := evaluate(ctx, r.snapshot(scope), r.timeout)
	report := Report{Status: status, Checks: results}
	if r.requestCount != nil {
		requests := r.requestCount()
		report.Requests = &requests
	}
	return report
}

// Handler serves the report for one scope as JSON, with 200 when the status is
// ok and 503 when it is not.
func (r *Registry) Handler(scope Scope) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		report := r.Report(request.Context(), scope)
		w.Header().Set("Content-Type", "application/json")
		if report.Status != StatusOK {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(report)
	})
}

// Mount registers the three probe routes on mux. An empty path in paths is
// skipped, so an assembly can expose readiness alone.
func (r *Registry) Mount(mux *http.ServeMux, paths Paths) {
	for path, scope := range map[string]Scope{
		paths.All:       ScopeAll,
		paths.Liveness:  ScopeLiveness,
		paths.Readiness: ScopeReadiness,
	} {
		if path == "" {
			continue
		}
		mux.Handle(path, r.Handler(scope))
	}
}

// snapshot copies the checks for a scope. Checks are read under the mutex
// because an assembly may register more after the registry is mounted.
func (r *Registry) snapshot(scope Scope) []namedCheck {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch scope {
	case ScopeLiveness:
		return append([]namedCheck(nil), r.liveness...)
	case ScopeReadiness:
		return append([]namedCheck(nil), r.readiness...)
	default:
		checks := make([]namedCheck, 0, len(r.liveness)+len(r.readiness))
		checks = append(checks, r.liveness...)
		return append(checks, r.readiness...)
	}
}

func evaluate(ctx context.Context, checks []namedCheck, timeout time.Duration) (string, []CheckResult) {
	if len(checks) == 0 {
		return StatusOK, nil
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
	results := make([]CheckResult, len(checks))
	completed := make([]bool, len(checks))
	outcomes := make(chan outcome, len(checks))
	for i, entry := range checks {
		results[i] = CheckResult{Name: entry.name, Status: StatusOK}
		go func(index int, entry namedCheck) {
			outcomes <- outcome{index: index, err: run(checksCtx, entry)}
		}(i, entry)
	}

	remaining := len(checks)
	for remaining > 0 {
		select {
		case result := <-outcomes:
			remaining--
			completed[result.index] = true
			apply(checksCtx, &results[result.index], result.err)
		case <-checksCtx.Done():
			draining := true
			for draining {
				select {
				case result := <-outcomes:
					remaining--
					completed[result.index] = true
					apply(checksCtx, &results[result.index], result.err)
				default:
					draining = false
				}
			}
			for i := range results {
				if !completed[i] {
					results[i].Status = "error"
					results[i].Error = message(checksCtx, checksCtx.Err())
				}
			}
			return StatusUnavailable, results
		}
	}

	for _, result := range results {
		if result.Status != StatusOK {
			return StatusUnavailable, results
		}
	}
	return StatusOK, results
}

func apply(ctx context.Context, result *CheckResult, err error) {
	if err == nil {
		return
	}
	result.Status = "error"
	result.Error = message(ctx, err)
}

func message(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "check timed out"
	}
	if errors.Is(err, errInProgress) {
		return "check already running"
	}
	if errors.Is(err, errPanicked) {
		return "check panicked"
	}
	return "check failed"
}

var errInProgress = errors.New("health check is already running")

// errPanicked reports a check that panicked. The panic is converted rather than
// propagated: checks run in their own goroutines, where an unrecovered panic
// takes the process down, and a probe is an unauthenticated request — a buggy
// check must not become a remote kill switch. Reporting the check as unhealthy
// is the honest answer.
var errPanicked = errors.New("health check panicked")

func run(ctx context.Context, entry namedCheck) (err error) {
	if entry.gate != nil {
		select {
		case entry.gate <- struct{}{}:
			defer func() { <-entry.gate }()
		default:
			return errInProgress
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", errPanicked, recovered)
		}
	}()

	return entry.check(ctx)
}
