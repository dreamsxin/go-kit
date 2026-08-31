package kit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// Host orchestrates lifecycle components without owning any transport. A
// pure worker, a gRPC-only service, or an HTTP service assembled from an
// HTTP component all run through the same Host contract.
//
//	host, err := kit.NewHost(kit.WithLifecycle(httpComponent, worker))
//	if err != nil {
//	    return err
//	}
//	return host.Run(ctx)
type Host struct {
	components      []Lifecycle
	serveErrors     chan error
	lifecycleDone   chan struct{}
	shutdownTimeout time.Duration

	mu      sync.Mutex
	started bool
	stopped bool
}

// HostOption configures a Host.
type HostOption func(*Host) error

// NewHost creates a Host from the given options. Components attach through
// WithLifecycle and start in declaration order.
func NewHost(opts ...HostOption) (*Host, error) {
	h := &Host{
		shutdownTimeout: DefaultShutdownTimeout,
	}
	for i, option := range opts {
		if option == nil {
			return nil, fmt.Errorf("kit: host option %d is nil", i)
		}
		if err := option(h); err != nil {
			return nil, fmt.Errorf("kit: apply host option %d: %w", i, err)
		}
	}
	h.serveErrors = make(chan error, len(h.components)+1)
	h.bridgeReadiness()
	return h, nil
}

// MustNewHost creates a Host and panics if its configuration is invalid. It
// is intended for tests and small examples; production startup should use
// NewHost.
func MustNewHost(opts ...HostOption) *Host {
	h, err := NewHost(opts...)
	if err != nil {
		panic(err)
	}
	return h
}

// WithLifecycle attaches components to the Host. Components start in
// declaration order and stop in reverse order.
func WithLifecycle(components ...Lifecycle) HostOption {
	copied := append([]Lifecycle(nil), components...)
	return func(h *Host) error {
		for i, component := range copied {
			if isNilLifecycle(component) {
				return fmt.Errorf("lifecycle component %d is nil", i)
			}
		}
		h.components = append(h.components, copied...)
		return nil
	}
}

// WithShutdownTimeout configures the graceful shutdown deadline used by Run.
func WithShutdownTimeout(timeout time.Duration) HostOption {
	return func(h *Host) error {
		if timeout <= 0 {
			return fmt.Errorf("shutdown timeout must be > 0")
		}
		h.shutdownTimeout = timeout
		return nil
	}
}

// bridgeReadiness registers every ReadinessProvider with a mounted component
// that serves readiness probes, so asynchronous warm-up is visible through
// the serving component's readiness surface.
func (h *Host) bridgeReadiness() {
	var sink readinessSink
	for _, component := range h.components {
		if candidate, ok := component.(readinessSink); ok {
			sink = candidate
			break
		}
	}
	if sink == nil {
		return
	}
	for i, component := range h.components {
		if provider, ok := component.(ReadinessProvider); ok {
			check := provider.Ready
			sink.registerLifecycleReadiness("lifecycle:"+lifecycleLabel(i, component), check)
		}
	}
}

// readinessSink is implemented by components that serve readiness probes
// (the HTTP component). It is unexported on purpose: only kit-provided
// serving surfaces aggregate lifecycle readiness.
type readinessSink interface {
	registerLifecycleReadiness(name string, check HealthCheck)
}

// Run starts the attached components and blocks until ctx is cancelled or a
// component fails. Signal handling belongs to the calling main package.
func (h *Host) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("kit: nil run context")
	}
	if err := h.Start(); err != nil {
		return err
	}

	var runErr error
	select {
	case <-ctx.Done():
		cause := context.Cause(ctx)
		if cause != nil && !errors.Is(cause, context.Canceled) {
			runErr = cause
		}
	case runErr = <-h.Errors():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), h.shutdownTimeout)
	defer cancel()
	return errors.Join(runErr, h.Shutdown(shutdownCtx))
}

// Start starts all attached components in the background. Component startup
// failures are returned directly.
func (h *Host) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return fmt.Errorf("kit: host already started")
	}
	if h.stopped {
		return fmt.Errorf("kit: host cannot be restarted after shutdown")
	}

	startedComponents := 0
	for i, component := range h.components {
		if err := component.Start(); err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), h.shutdownTimeout)
			cleanupErr := shutdownLifecycles(cleanupCtx, h.components[:startedComponents])
			cancel()
			if startedComponents > 0 {
				h.stopped = true
			}
			return errors.Join(fmt.Errorf("start lifecycle component %s: %w", lifecycleLabel(i, component), err), cleanupErr)
		}
		startedComponents++
	}

	h.lifecycleDone = make(chan struct{})
	h.started = true

	for i, component := range h.components {
		componentErrors := component.Errors()
		if componentErrors != nil {
			go h.watchLifecycle(lifecycleLabel(i, component), componentErrors, h.lifecycleDone)
		}
	}
	return nil
}

// Errors reports asynchronous component failures after Start.
func (h *Host) Errors() <-chan error {
	return h.serveErrors
}

func (h *Host) reportServeError(err error) {
	select {
	case h.serveErrors <- err:
	default:
	}
}

func (h *Host) watchLifecycle(label string, componentErrors <-chan error, done <-chan struct{}) {
	for {
		select {
		case err, ok := <-componentErrors:
			if !ok {
				return
			}
			if err != nil {
				h.reportServeError(fmt.Errorf("lifecycle component %s: %w", label, err))
			}
		case <-done:
			return
		}
	}
}

// lifecycleLabel names a component for diagnostics: the component's own name
// when it implements NamedLifecycle, otherwise its attachment index.
func lifecycleLabel(index int, component Lifecycle) string {
	if named, ok := component.(NamedLifecycle); ok {
		if name := named.Name(); name != "" {
			return name
		}
	}
	return fmt.Sprintf("%d", index)
}

// Shutdown gracefully stops all attached components in reverse attachment
// order.
func (h *Host) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("kit: nil shutdown context")
	}
	h.mu.Lock()
	if !h.started {
		h.mu.Unlock()
		return nil
	}
	lifecycleDone := h.lifecycleDone
	components := append([]Lifecycle(nil), h.components...)
	h.started = false
	h.stopped = true
	h.mu.Unlock()

	if lifecycleDone != nil {
		close(lifecycleDone)
	}
	return shutdownLifecycles(ctx, components)
}

func shutdownLifecycles(ctx context.Context, components []Lifecycle) error {
	var result error
	for i := len(components) - 1; i >= 0; i-- {
		if err := components[i].Shutdown(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("shutdown lifecycle component %s: %w", lifecycleLabel(i, components[i]), err))
		}
	}
	return result
}

func isNilLifecycle(component Lifecycle) bool {
	if component == nil {
		return true
	}
	value := reflect.ValueOf(component)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
