package kit

import (
	"time"

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
	kitlog "github.com/dreamsxin/go-kit/log"
)

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
		cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "service",
			ReadyToTrip: func(c gobreaker.Counts) bool {
				return c.ConsecutiveFailures >= consecutiveFailures
			},
		})
		s.middleware = append(s.middleware, circuitbreaker.Gobreaker(cb))
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
	return func(s *Service) {
		s.metrics = m
		s.middleware = append(s.middleware, endpoint.MetricsMiddleware(m))
	}
}

// WithRequestID injects a request ID into the context and response headers.
// The ID is taken from X-Request-ID if present, otherwise generated.
func WithRequestID() Option {
	return func(s *Service) {
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
