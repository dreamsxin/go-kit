package kit

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/health"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// HTTPServerConfig controls the production HTTP server created by HTTP.Start.
type HTTPServerConfig struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

// DefaultHTTPServerConfig returns streaming-safe production defaults. Read and
// write deadlines remain disabled; ReadHeaderTimeout limits slow header reads.
func DefaultHTTPServerConfig() HTTPServerConfig {
	return HTTPServerConfig{
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// DefaultJSONMaxBodyBytes is the default strict JSON body limit used by
// HandleJSON and HandleJSONTyped.
const DefaultJSONMaxBodyBytes = httpserver.DefaultMaxJSONBodyBytes

// WithHTTPServerConfig configures timeouts and header limits for the HTTP
// server created by HTTP.Start.
func WithHTTPServerConfig(config HTTPServerConfig) Option {
	return func(h *HTTP) error {
		if config.ReadHeaderTimeout < 0 || config.ReadTimeout < 0 ||
			config.WriteTimeout < 0 || config.IdleTimeout < 0 {
			return fmt.Errorf("HTTP server durations cannot be negative")
		}
		if config.MaxHeaderBytes < 0 {
			return fmt.Errorf("HTTP max header bytes cannot be negative")
		}
		h.httpConfig = config
		return nil
	}
}

// WithHTTPMiddleware installs standard-library HTTP middleware around every
// route, including health, JSON endpoint, raw HTTP, and generated routes. The
// first middleware is the outermost handler.
func WithHTTPMiddleware(middlewares ...func(http.Handler) http.Handler) Option {
	copied := append([]func(http.Handler) http.Handler(nil), middlewares...)
	return func(h *HTTP) error {
		for i, middleware := range copied {
			if middleware == nil {
				return fmt.Errorf("HTTP middleware %d is nil", i)
			}
		}
		h.httpMiddleware = append(h.httpMiddleware, copied...)
		return nil
	}
}

// WithJSONMaxBodyBytes configures the strict JSON body limit used by
// HandleJSON and HandleJSONTyped. A value <= 0 disables the size limit while
// keeping strict field and trailing-data checks.
func WithJSONMaxBodyBytes(maxBodyBytes int64) Option {
	return func(h *HTTP) error {
		if maxBodyBytes < 0 {
			return fmt.Errorf("JSON max body bytes cannot be negative")
		}
		h.jsonMaxBodyBytes = maxBodyBytes
		return nil
	}
}

// WithJSONServerOptions applies the given server options to every JSON route
// registered through HandleJSONTyped, HandleJSON, and HandleJSONEndpoint.
//
// Use it to define transport-level response assembly once for the whole
// component, for example a response envelope with ServerResponseEncoder and a
// matching error format with ServerErrorEncoder. Options passed to an
// individual registration run after these and take precedence.
func WithJSONServerOptions(opts ...httpserver.ServerOption) Option {
	copied := append([]httpserver.ServerOption(nil), opts...)
	return func(h *HTTP) error {
		for i, option := range copied {
			if option == nil {
				return fmt.Errorf("JSON server option %d is nil", i)
			}
		}
		h.jsonServerOptions = append(h.jsonServerOptions, copied...)
		return nil
	}
}

// WithLivenessCheck adds a check used by the liveness and combined probe
// routes.
func WithLivenessCheck(name string, check HealthCheck) Option {
	return func(h *HTTP) error {
		if err := validateHealthCheck(name, check); err != nil {
			return err
		}
		h.pendingLiveness = append(h.pendingLiveness, pendingProbe{name: name, check: check})
		return nil
	}
}

// WithReadinessCheck adds a check used by the readiness and combined probe
// routes.
func WithReadinessCheck(name string, check HealthCheck) Option {
	return func(h *HTTP) error {
		if err := validateHealthCheck(name, check); err != nil {
			return err
		}
		h.pendingReadiness = append(h.pendingReadiness, pendingProbe{name: name, check: check})
		return nil
	}
}

// WithHealthCheckTimeout configures the per-check timeout for the probe routes.
// A value <= 0 disables the timeout.
func WithHealthCheckTimeout(timeout time.Duration) Option {
	return func(h *HTTP) error {
		h.healthTimeout = timeout
		return nil
	}
}

// WithProbePaths serves the probes on routes of your choosing. An empty path
// omits that route, so a deployment that only orchestrates on readiness can
// expose readiness alone.
//
// The paths are registered on the component's mux, alongside the application's
// routes and behind WithHTTPMiddleware. To keep probes off the traffic port
// entirely, omit them here and mount Probes() on a second listener of your own.
func WithProbePaths(paths health.Paths) Option {
	return func(h *HTTP) error {
		h.probePaths = paths
		return nil
	}
}

// WithoutProbes serves no probe routes. Use it when the probes belong on a
// separate administrative listener, which Probes() can serve.
func WithoutProbes() Option {
	return func(h *HTTP) error {
		h.probePaths = health.Paths{}
		return nil
	}
}

func validateHealthCheck(name string, check HealthCheck) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("health check name cannot be empty")
	}
	if check == nil {
		return fmt.Errorf("health check cannot be nil")
	}
	return nil
}

// WithEndpointMiddleware installs middleware around every endpoint registered
// through HandleJSONTyped, HandleJSON, or HandleJSONEndpoint. The first
// middleware is outermost.
// Protocol- and dependency-specific middleware remains application owned.
func WithEndpointMiddleware(middlewares ...endpoint.Middleware) Option {
	copied := append([]endpoint.Middleware(nil), middlewares...)
	return func(h *HTTP) error {
		for i, middleware := range copied {
			if middleware == nil {
				return fmt.Errorf("endpoint middleware %d is nil", i)
			}
		}
		h.middleware = append(h.middleware, copied...)
		return nil
	}
}

// WithTimeout adds a per-request context deadline.
//
// It applies to every route, including raw handlers registered with Handle:
// the deadline is set on the request context before the handler runs, and the
// endpoint chain derives its own from it. Handlers that respect ctx therefore
// stop on time; one that ignores cancellation still runs to completion, since a
// deadline propagates but cannot interrupt.
func WithTimeout(d time.Duration) Option {
	return func(h *HTTP) error {
		if d <= 0 {
			return fmt.Errorf("timeout must be > 0")
		}
		h.timeout = d
		h.middleware = append(h.middleware, endpoint.TimeoutMiddleware(d))
		return nil
	}
}

// WithMetrics attaches the built-in in-memory Metrics collector. Each route is
// recorded under its own pattern, so Metrics.SnapshotFor("POST /users") reports
// that route alone while Metrics.Snapshot reports the total. The /health
// endpoint includes the total request count when this option is set.
//
// Use WithRecorder to report the same observations to an external metrics
// backend.
func WithMetrics(m *endpoint.Metrics) Option {
	return func(h *HTTP) error {
		if m == nil {
			return fmt.Errorf("metrics cannot be nil")
		}
		h.metrics = m
		h.recorders = append(h.recorders, m)
		return nil
	}
}

// WithRecorder reports every endpoint call to the given recorders, labeled with
// the route pattern. Use it to bridge the framework to Prometheus,
// OpenTelemetry, or any other metrics backend.
func WithRecorder(recorders ...endpoint.Recorder) Option {
	return func(h *HTTP) error {
		for i, recorder := range recorders {
			if recorder == nil {
				return fmt.Errorf("recorder %d is nil", i)
			}
		}
		h.recorders = append(h.recorders, recorders...)
		return nil
	}
}

// WithHTTPRecorder reports every request to the given HTTP recorders with the
// protocol facts an endpoint recorder cannot see: the matched route pattern,
// the method, and the response status code.
//
// It covers the application's routes — JSON endpoints, generated routes, and
// raw handlers registered with Handle. The probe routes are not recorded:
// orchestrator traffic is not application traffic, and a liveness check every
// second would dominate every rate this measures.
//
// observability/otel implements the interface against the OpenTelemetry HTTP
// semantic conventions:
//
//	metrics, err := oteladapter.NewHTTPMetrics(providers.Meter())
//	component, err := kit.NewHTTP(":8080", kit.WithHTTPRecorder(metrics))
//
// Use WithRecorder for business outcomes; use this for response status.
func WithHTTPRecorder(recorders ...httpserver.Recorder) Option {
	return func(h *HTTP) error {
		for i, recorder := range recorders {
			if recorder == nil {
				return fmt.Errorf("HTTP recorder %d is nil", i)
			}
		}
		h.httpRecorders = append(h.httpRecorders, recorders...)
		return nil
	}
}

// WithRequestID injects a request ID into the context and response headers.
// A valid ID is taken from X-Request-ID if present, otherwise a new ID is
// generated. Use WithRequestIDValidator to replace the default trust policy.
func WithRequestID() Option {
	return func(h *HTTP) error {
		h.requestID = true
		h.middleware = append(h.middleware, requestIDMiddleware(h))
		return nil
	}
}

// WithRequestIDValidator replaces the validation policy for request IDs read
// from context or X-Request-ID. Option order does not affect the result.
func WithRequestIDValidator(validator RequestIDValidator) Option {
	return func(h *HTTP) error {
		if validator == nil {
			return fmt.Errorf("request ID validator cannot be nil")
		}
		h.requestIDValidator = validator
		return nil
	}
}
