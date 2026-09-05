package slogadapter

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// Signals selects which telemetry dimensions NewTelemetry assembles. The zero
// value assembles all of them, so a config that says nothing about signals gets
// the whole chain.
//
// Select individually when a dimension belongs somewhere else: a service whose
// metrics come from an OpenTelemetry meter still wants this package's logging,
// and asking for both would record every call twice.
type Signals uint8

// The dimensions NewTelemetry can assemble.
const (
	SignalTracing Signals = 1 << iota
	SignalMetrics
	SignalLogging
)

// AllSignals is every dimension, which is also what the zero value selects.
const AllSignals = SignalTracing | SignalMetrics | SignalLogging

func (s Signals) has(signal Signals) bool {
	if s == 0 {
		return true
	}
	return s&signal != 0
}

// TelemetryConfig controls NewTelemetry.
type TelemetryConfig struct {
	// Operation is the bounded operation name recorded by every dimension.
	// Required when logging is assembled.
	Operation string
	// Logger receives access records. nil falls back to slog.Default().
	Logger *slog.Logger
	// Metrics receives the built-in request counters. nil creates a private
	// collector; keep the returned Telemetry.Metrics to expose snapshots,
	// for example through kit.WithMetrics semantics or /health.
	Metrics *endpoint.Metrics
	// LogOptions tune the logging middleware.
	LogOptions []Option
	// Signals selects the dimensions to assemble. The zero value assembles all.
	Signals Signals
}

// NamedMiddleware is one middleware together with the label it reports under in
// endpoint.Builder.Describe. The name travels with the middleware rather than
// being matched by position, so a chain missing a dimension — or carrying one a
// caller appended — still describes itself correctly.
type NamedMiddleware struct {
	Name       string
	Middleware endpoint.Middleware
}

// Telemetry is the assembled observability middleware chain together with the
// collector it feeds.
type Telemetry struct {
	// Middlewares holds the assembled dimensions in canonical order, outermost
	// first: tracing, then metrics, then logging. A dimension that was not
	// selected is absent.
	Middlewares []NamedMiddleware
	// Metrics is the collector feeding the metrics middleware, or the
	// collector the config supplied when metrics were not assembled.
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
	logging := cfg.Signals.has(SignalLogging)
	if logging && strings.TrimSpace(cfg.Operation) == "" {
		return nil, errors.New("slogadapter: telemetry operation cannot be empty")
	}
	telemetry := &Telemetry{Metrics: cfg.Metrics}
	if cfg.Signals.has(SignalTracing) {
		telemetry.Middlewares = append(telemetry.Middlewares, NamedMiddleware{
			Name:       "telemetry:tracing",
			Middleware: endpoint.TracingMiddleware(),
		})
	}
	if cfg.Signals.has(SignalMetrics) {
		if telemetry.Metrics == nil {
			telemetry.Metrics = &endpoint.Metrics{}
		}
		telemetry.Middlewares = append(telemetry.Middlewares, NamedMiddleware{
			Name:       "telemetry:metrics",
			Middleware: endpoint.MetricsMiddleware(telemetry.Metrics),
		})
	}
	if logging {
		logger := cfg.Logger
		if logger == nil {
			logger = slog.Default()
		}
		telemetry.Middlewares = append(telemetry.Middlewares, NamedMiddleware{
			Name:       "telemetry:logging",
			Middleware: LoggingMiddleware(logger, cfg.Operation, cfg.LogOptions...),
		})
	}
	return telemetry, nil
}

// Apply installs the chain on a Builder in canonical order, each middleware
// under its own name.
func (t *Telemetry) Apply(b *endpoint.Builder) *endpoint.Builder {
	if t == nil || b == nil {
		panic("slogadapter: telemetry and builder cannot be nil")
	}
	for _, middleware := range t.Middlewares {
		b.UseNamed(middleware.Name, middleware.Middleware)
	}
	return b
}
