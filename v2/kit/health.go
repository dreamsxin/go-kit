package kit

import (
	"context"

	"github.com/dreamsxin/go-kit/v2/health"
)

// HealthCheck reports whether a runtime dependency is healthy. Implementations
// must stop promptly when ctx is canceled.
//
// It is health.Check: the probe engine lives in the health package so that
// gRPC assemblies and services built directly on the transport packages report
// readiness the same way this one does.
type HealthCheck = health.Check

// DefaultHealthCheckTimeout is the per-check timeout used by the probe routes
// unless WithHealthCheckTimeout overrides it.
const DefaultHealthCheckTimeout = health.DefaultTimeout

// Healthy is a convenience health check that always succeeds.
func Healthy(ctx context.Context) error { return health.Healthy(ctx) }

// DefaultProbePaths are the routes an HTTP component serves unless
// WithProbePaths or WithoutProbes says otherwise.
func DefaultProbePaths() health.Paths { return health.DefaultPaths() }

// Probes returns the component's probe registry, so an application can add a
// check after construction or mount the same registry somewhere else.
func (h *HTTP) Probes() *health.Registry { return h.probes }

// buildProbes assembles the registry from the options that were applied and
// mounts it. It runs after the options so the per-check timeout and the request
// counter are known, whatever order the options came in.
func (h *HTTP) buildProbes() error {
	options := []health.Option{health.WithTimeout(h.healthTimeout)}
	if h.metrics != nil {
		metrics := h.metrics
		options = append(options, health.WithRequestCount(func() int64 {
			return metrics.Snapshot().RequestCount
		}))
	}
	h.probes = health.NewRegistry(options...)
	for _, pending := range h.pendingLiveness {
		if err := h.probes.AddLiveness(pending.name, pending.check); err != nil {
			return err
		}
	}
	for _, pending := range h.pendingReadiness {
		if err := h.probes.AddReadiness(pending.name, pending.check); err != nil {
			return err
		}
	}
	h.probes.Mount(h.mux, h.probePaths)
	return nil
}

type pendingProbe struct {
	name  string
	check health.Check
}
