package kit

import (
	"context"
	"time"

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/v2/endpoint/ratelimit"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// HTTPServerConfig controls the production HTTP server created by Start.
// Zero values retain net/http defaults.
type HTTPServerConfig struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

// DefaultJSONMaxBodyBytes is the default strict JSON body limit used by
// HandleJSON.
const DefaultJSONMaxBodyBytes = httpserver.DefaultMaxJSONBodyBytes

// WithHTTPServerConfig configures timeouts and header limits for the HTTP
// server created by Service.Start.
func WithHTTPServerConfig(config HTTPServerConfig) Option {
	if config.ReadHeaderTimeout < 0 || config.ReadTimeout < 0 ||
		config.WriteTimeout < 0 || config.IdleTimeout < 0 {
		panic("kit: HTTP server durations cannot be negative")
	}
	if config.MaxHeaderBytes < 0 {
		panic("kit: HTTP max header bytes cannot be negative")
	}
	return func(s *Service) {
		s.httpConfig = config
	}
}

// WithJSONMaxBodyBytes configures the strict JSON body limit used by
// HandleJSON. A value <= 0 disables the size limit while keeping strict field
// and trailing-data checks.
func WithJSONMaxBodyBytes(maxBodyBytes int64) Option {
	if maxBodyBytes < 0 {
		panic("kit: JSON max body bytes cannot be negative")
	}
	return func(s *Service) {
		s.jsonMaxBodyBytes = maxBodyBytes
	}
}

// WithLivenessCheck adds a check used by /livez and /health.
func WithLivenessCheck(name string, check HealthCheck) Option {
	validateHealthCheck(name, check)
	return func(s *Service) {
		s.livenessChecks = append(s.livenessChecks, namedHealthCheck{name: name, check: check})
	}
}

// WithReadinessCheck adds a check used by /readyz and /health.
func WithReadinessCheck(name string, check HealthCheck) Option {
	validateHealthCheck(name, check)
	return func(s *Service) {
		s.readinessChecks = append(s.readinessChecks, namedHealthCheck{name: name, check: check})
	}
}

// WithHealthCheckTimeout configures the per-check timeout for /health, /livez,
// and /readyz. A value <= 0 disables the timeout.
func WithHealthCheckTimeout(timeout time.Duration) Option {
	return func(s *Service) {
		s.healthTimeout = timeout
	}
}

func validateHealthCheck(name string, check HealthCheck) {
	if name == "" {
		panic("kit: health check name cannot be empty")
	}
	if check == nil {
		panic("kit: health check cannot be nil")
	}
}

// Healthy is a convenience health check that always succeeds.
func Healthy(context.Context) error {
	return nil
}

// WithRateLimit adds a token-bucket rate limiter (rps = requests per second).
func WithRateLimit(rps float64) Option {
	if rps <= 0 {
		panic("kit: rate limit must be > 0")
	}
	return func(s *Service) {
		burst := int(rps)
		if burst < 1 {
			burst = 1
		}
		lim := rate.NewLimiter(rate.Limit(rps), burst)
		s.middleware = append(s.middleware, ratelimit.NewErroringLimiter(lim))
	}
}

// WithCircuitBreaker adds a Gobreaker circuit breaker that opens after
// consecutiveFailures consecutive errors.
func WithCircuitBreaker(consecutiveFailures uint32) Option {
	if consecutiveFailures == 0 {
		panic("kit: circuit breaker threshold must be > 0")
	}
	return func(s *Service) {
		s.routeMiddleware = append(s.routeMiddleware, func(route string) endpoint.Middleware {
			cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
				Name: route,
				ReadyToTrip: func(c gobreaker.Counts) bool {
					return c.ConsecutiveFailures >= consecutiveFailures
				},
			})
			return circuitbreaker.Gobreaker(cb)
		})
	}
}

// WithTimeout adds a per-request context deadline.
func WithTimeout(d time.Duration) Option {
	if d <= 0 {
		panic("kit: timeout must be > 0")
	}
	return func(s *Service) {
		s.middleware = append(s.middleware, endpoint.TimeoutMiddleware(d))
	}
}

// WithLogging adds structured request logging.
func WithLogging(logger *kitlog.Logger) Option {
	return func(s *Service) {
		if logger == nil {
			logger = kitlog.NewNopLogger()
		}
		s.logger = logger
		s.middleware = append(s.middleware, endpoint.LoggingMiddleware(logger, "request"))
	}
}

// WithMetrics attaches a Metrics collector.
// The /health endpoint includes the request count when this option is set.
func WithMetrics(m *endpoint.Metrics) Option {
	if m == nil {
		panic("kit: metrics cannot be nil")
	}
	return func(s *Service) {
		s.metrics = m
		s.middleware = append(s.middleware, endpoint.MetricsMiddleware(m))
	}
}

// WithRequestID injects a request ID into the context and response headers.
// The ID is taken from X-Request-ID if present, otherwise generated.
func WithRequestID() Option {
	return func(s *Service) {
		s.requestID = true
		s.middleware = append(s.middleware, requestIDMiddleware())
	}
}

// WithGRPC enables a gRPC server on the given address (for example ":8081").
// Call GRPCServer() to register proto services before calling Run or Start.
func WithGRPC(addr string, opts ...grpc.ServerOption) Option {
	if addr == "" {
		panic("kit: grpc address cannot be empty")
	}
	return func(s *Service) {
		s.grpcAddr = addr
		s.grpcOpts = opts
	}
}
