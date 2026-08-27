package kit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// DefaultShutdownTimeout is the graceful shutdown deadline used by Run.
const DefaultShutdownTimeout = 10 * time.Second

// Lifecycle is an optional component managed alongside the HTTP server.
// Start must report listener or configuration failures synchronously. Errors
// reports asynchronous failures after Start; it may return nil when the
// component has no asynchronous failure path.
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
// asynchronously. Ready is bridged into the /readyz and /health readiness
// checks, so the service only reports ready once every provider reports
// ready. Ready must stop promptly when ctx is canceled.
type ReadinessProvider interface {
	Ready(ctx context.Context) error
}

// Run starts the configured servers and blocks until ctx is cancelled or a
// server fails. Signal handling belongs to the calling main package.
func (s *Service) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("kit: nil run context")
	}
	if err := s.Start(); err != nil {
		return err
	}

	var runErr error
	select {
	case <-ctx.Done():
		cause := context.Cause(ctx)
		if cause != nil && !errors.Is(cause, context.Canceled) {
			runErr = cause
		}
	case runErr = <-s.Errors():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	return errors.Join(runErr, s.Shutdown(shutdownCtx))
}

// Start starts the HTTP server and attached lifecycle components in the
// background. Listener and component startup failures are returned directly.
func (s *Service) Start() error {
	if s == nil {
		return fmt.Errorf("kit: nil Service")
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.started {
		return fmt.Errorf("kit: service already started")
	}
	if s.stopped {
		return fmt.Errorf("kit: service cannot be restarted after shutdown")
	}

	httpLis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("http listen: %w", err)
	}

	startedComponents := 0
	for i, component := range s.lifecycles {
		if err := component.Start(); err != nil {
			_ = httpLis.Close()
			cleanupCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
			cleanupErr := shutdownLifecycles(cleanupCtx, s.lifecycles[:startedComponents])
			cancel()
			if startedComponents > 0 {
				s.stopped = true
			}
			return errors.Join(fmt.Errorf("start lifecycle component %s: %w", lifecycleLabel(i, component), err), cleanupErr)
		}
		startedComponents++
	}

	s.srv = &http.Server{
		Addr:              s.addr,
		Handler:           s.httpHandler,
		ReadHeaderTimeout: s.httpConfig.ReadHeaderTimeout,
		ReadTimeout:       s.httpConfig.ReadTimeout,
		WriteTimeout:      s.httpConfig.WriteTimeout,
		IdleTimeout:       s.httpConfig.IdleTimeout,
		MaxHeaderBytes:    s.httpConfig.MaxHeaderBytes,
	}
	s.lifecycleDone = make(chan struct{})
	s.started = true
	go func() {
		if err := s.srv.Serve(httpLis); err != nil && err != http.ErrServerClosed {
			s.reportServeError(fmt.Errorf("http serve: %w", err))
		}
	}()

	for i, component := range s.lifecycles {
		errors := component.Errors()
		if errors != nil {
			go s.watchLifecycle(lifecycleLabel(i, component), errors, s.lifecycleDone)
		}
	}
	return nil
}

// Errors reports asynchronous HTTP or lifecycle component failures after Start.
// Listener bind failures are still returned directly from Start.
func (s *Service) Errors() <-chan error {
	if s == nil {
		return nil
	}
	return s.serveErrors
}

func (s *Service) reportServeError(err error) {
	select {
	case s.serveErrors <- err:
	default:
	}
}

func (s *Service) watchLifecycle(label string, errors <-chan error, done <-chan struct{}) {
	for {
		select {
		case err, ok := <-errors:
			if !ok {
				return
			}
			if err != nil {
				s.reportServeError(fmt.Errorf("lifecycle component %s: %w", label, err))
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

// Shutdown gracefully stops the HTTP server and attached components.
func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("kit: nil shutdown context")
	}
	s.lifecycleMu.Lock()
	if !s.started {
		s.lifecycleMu.Unlock()
		return nil
	}
	srv := s.srv
	lifecycleDone := s.lifecycleDone
	components := append([]Lifecycle(nil), s.lifecycles...)
	s.started = false
	s.stopped = true
	s.lifecycleMu.Unlock()

	if lifecycleDone != nil {
		close(lifecycleDone)
	}
	var httpErr error
	if srv != nil {
		httpErr = srv.Shutdown(ctx)
	}
	return errors.Join(httpErr, shutdownLifecycles(ctx, components))
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
