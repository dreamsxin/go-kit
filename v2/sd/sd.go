// Package sd provides service-discovery helpers that wire together an
// Instancer, EndpointCache, Balancer, and Retry executor into a single
// callable endpoint.Endpoint.
//
// Typical usage:
//
//	instancer := consul.NewInstancer(consulClient, logger, "my-service", true)
//	defer instancer.Stop()
//
//	ep, closer, err := sd.NewEndpoint(instancer, factory, logger,
//	    sd.WithMaxAttempts(3),
//	    sd.WithTimeout(500*time.Millisecond),
//	    sd.WithInvalidateOnError(5*time.Second),
//	)
//	if err != nil { return err }
//	defer closer.Close()
//	resp, err := ep(ctx, request)
package sd

import (
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer/executor"
	"github.com/dreamsxin/go-kit/v2/sd/interfaces"
)

// Options controls the behaviour of NewEndpoint.
type Options struct {
	// MaxAttempts is the total number of call attempts (default 1).
	MaxAttempts int

	// Timeout is the total time budget for one call including all retries
	// (default 500ms).
	Timeout time.Duration

	// InvalidateOnError, when > 0, causes the endpoint cache to be cleared
	// after the given duration following a service-discovery error.
	InvalidateOnError time.Duration

	// Retryable classifies which endpoint errors should be retried.
	// Nil uses executor.DefaultRetryable.
	Retryable executor.RetryableFunc
}

// Option is a functional option for NewEndpoint.
type Option func(*Options)

// WithMaxAttempts sets the total number of call attempts. It must be at least 1.
func WithMaxAttempts(n int) Option {
	return func(o *Options) { o.MaxAttempts = n }
}

// WithTimeout sets the total call timeout (including retries).
func WithTimeout(d time.Duration) Option {
	return func(o *Options) { o.Timeout = d }
}

// WithInvalidateOnError enables cache invalidation after a SD error.
// The cache is cleared once the given grace period has elapsed.
func WithInvalidateOnError(d time.Duration) Option {
	return func(o *Options) { o.InvalidateOnError = d }
}

// WithRetryable customizes retry error classification.
func WithRetryable(fn executor.RetryableFunc) Option {
	return func(o *Options) { o.Retryable = fn }
}

// NewEndpoint wires together an Instancer, Endpointer, RoundRobin balancer, and
// Retry executor. The returned closer owns the background Endpointer and every
// endpoint resource created by factory. Close it before stopping the Instancer.
func NewEndpoint(
	src interfaces.Instancer,
	factory endpoint.Factory,
	logger *kitlog.Logger,
	opts ...Option,
) (endpoint.Endpoint, io.Closer, error) {
	o := Options{
		MaxAttempts: 1,
		Timeout:     500 * time.Millisecond,
	}
	for i, opt := range opts {
		if opt == nil {
			return nil, nil, fmt.Errorf("sd: option %d is nil", i)
		}
		opt(&o)
	}
	if err := validateEndpointConfig(src, factory, logger, o); err != nil {
		return nil, nil, err
	}

	var epOpts []endpoint.EndpointerOption
	if o.InvalidateOnError > 0 {
		epOpts = append(epOpts, endpoint.InvalidateOnError(o.InvalidateOnError))
	}

	ep := endpointer.NewEndpointer(src, factory, logger, epOpts...)
	lb := balancer.NewRoundRobin(ep)

	call := executor.RetryWithRetryable(o.Timeout, lb, attemptLimit(o.MaxAttempts), o.Retryable)
	return call, ep, nil
}

// NewEndpointWithDefaults is identical to NewEndpoint but uses sensible
// production defaults without requiring any options:
//   - MaxAttempts: 1
//   - Timeout:    500ms
//   - InvalidateOnError: 5s
//
// Use NewEndpoint when you need to customise these values.
func NewEndpointWithDefaults(
	src interfaces.Instancer,
	factory endpoint.Factory,
	logger *kitlog.Logger,
) (endpoint.Endpoint, io.Closer, error) {
	return NewEndpoint(src, factory, logger,
		WithMaxAttempts(1),
		WithTimeout(500*time.Millisecond),
		WithInvalidateOnError(5*time.Second),
	)
}

func validateEndpointConfig(src interfaces.Instancer, factory endpoint.Factory, logger *kitlog.Logger, o Options) error {
	switch {
	case isNil(src):
		return fmt.Errorf("sd: instancer is nil")
	case factory == nil:
		return fmt.Errorf("sd: endpoint factory is nil")
	case logger == nil:
		return fmt.Errorf("sd: logger is nil")
	case o.MaxAttempts < 1:
		return fmt.Errorf("sd: max attempts must be at least 1")
	case o.Timeout <= 0:
		return fmt.Errorf("sd: timeout must be greater than zero")
	case o.InvalidateOnError < 0:
		return fmt.Errorf("sd: invalidate-on-error duration cannot be negative")
	default:
		return nil
	}
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func attemptLimit(max int) executor.RetryCallback {
	return func(n int, err error) (keepTrying bool, replacement error) {
		return n < max, nil
	}
}
