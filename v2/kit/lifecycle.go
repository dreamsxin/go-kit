package kit

import (
	"context"
	"time"
)

// DefaultShutdownTimeout is the graceful shutdown deadline used by Host.Run.
const DefaultShutdownTimeout = 10 * time.Second

// Lifecycle is a component managed by a Host. Start must report listener or
// configuration failures synchronously. Errors reports asynchronous failures
// after Start; it may return nil when the component has no asynchronous
// failure path.
type Lifecycle interface {
	Start() error
	Errors() <-chan error
	Shutdown(context.Context) error
}

// NamedLifecycle is a Lifecycle that reports a stable name used in startup,
// asynchronous failure, and shutdown diagnostics. Components that do not
// implement it are identified by their attachment index.
type NamedLifecycle interface {
	Lifecycle
	Name() string
}

// ReadinessProvider is implemented by lifecycle components that warm up
// asynchronously. Host bridges Ready into the readiness checks of the
// mounted serving component (for example the HTTP component's /readyz and
// /health). Ready must stop promptly when ctx is canceled.
type ReadinessProvider interface {
	Ready(ctx context.Context) error
}
