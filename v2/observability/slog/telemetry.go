package slogadapter

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// TelemetryConfig controls NewTelemetry.
type TelemetryConfig struct {
	// Operation is the bounded operation name recorded by every dimension.
	// Required.
	Operation string
	// Logger receives access records. nil falls back to slog.Default().
	Logger *slog.Logger
	// Metrics receives the built-in request counters. nil creates a private
	// collector; keep the returned Telemetry.Metrics to expose snapshots,
	// for example through kit.WithMetrics semantics or /health.
	Metrics *endpoint.Metrics
	// LogOptions tune the logging middleware.
	LogOptions []Option
}

// Telemetry is the assembled observability middleware chain together with the
// collector it feeds.
type Telemetry struct {
	// Middlewares holds the chain in canonical order, outermost first:
	// tracing, then metrics, then logging.
	Middlewares []endpoint.Middleware
	// Metrics is the collector feeding the metrics middleware.
	Metrics *endpoint.Metrics
}

// NewTelemetry assembles the built-in observability dimensions into one
// middleware chain with the canonical order: tracing outermost so every inner
// layer sees correlation IDs, metrics next, and logging innermost so the
// recorded duration excludes observability overhead.
//
// Vendor-specific adapters such as observability/otel remain additive:
// applications append them to the chain themselves, and provider setup stays
// application owned.
func NewTelemetry(cfg TelemetryConfig) (*Telemetry, error) {
	if strings.TrimSpace(cfg.Operation) == "" {
		return nil, errors.New("slogadapter: telemetry operation cannot be empty")
	}
	metrics := cfg.Metrics
	if metrics == nil {
		metrics = &endpoint.Metrics{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Telemetry{
		Middlewares: []endpoint.Middleware{
			endpoint.TracingMiddleware(),
			endpoint.MetricsMiddleware(metrics),
			LoggingMiddleware(logger, cfg.Operation, cfg.LogOptions...),
		},
		Metrics: metrics,
	}, nil
}

// Apply installs the chain on a Builder in canonical order with stable labels
// for Builder.Describe.
func (t *Telemetry) Apply(b *endpoint.Builder) *endpoint.Builder {
	if t == nil || b == nil {
		panic("slogadapter: telemetry and builder cannot be nil")
	}
	labels := []string{"telemetry:tracing", "telemetry:metrics", "telemetry:logging"}
	for i, middleware := range t.Middlewares {
		b.UseNamed(labels[i], middleware)
	}
	return b
}
