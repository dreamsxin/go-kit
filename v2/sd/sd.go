// Package sd provides service-discovery helpers that wire together an
// Instancer, EndpointCache, Balancer, and Retry executor into a single
// callable endpoint.Endpoint.
//
// Typical usage:
//
//	instancer := consul.NewInstancer(consulClient, logger, "my-service", true)
//	defer instancer.Stop()
//
//	ep := sd.NewEndpoint(instancer, factory, logger,
//	    sd.WithMaxAttempts(3),
//	    sd.WithTimeout(500*time.Millisecond),
//	    sd.WithInvalidateOnError(5*time.Second),
//	)
//	resp, err := ep(ctx, request)
package sd

import (
	"io"
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

// WithMaxAttempts sets the total number of call attempts. Values below 1 are
// normalized to one attempt.
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

// NewEndpoint wires together an Instancer → Endpointer → RoundRobin balancer
// → Retry executor and returns a single endpoint.Endpoint ready to call.
//
// The returned endpoint automatically distributes requests across healthy
// instances and can retry classified transient failures when MaxAttempts is
// greater than one.
func NewEndpoint(
	src interfaces.Instancer,
	factory endpoint.Factory,
	logger *kitlog.Logger,
	opts ...Option,
) endpoint.Endpoint {
	o := Options{
		MaxAttempts: 1,
		Timeout:     500 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(&o)
	}

	var epOpts []endpoint.EndpointerOption
	if o.InvalidateOnError > 0 {
		epOpts = append(epOpts, endpoint.InvalidateOnError(o.InvalidateOnError))
	}

	ep := endpointer.NewEndpointer(src, factory, logger, epOpts...)
	lb := balancer.NewRoundRobin(ep)

	if o.MaxAttempts < 1 {
		o.MaxAttempts = 1
	}
	return executor.RetryWithRetryable(o.Timeout, lb, attemptLimit(o.MaxAttempts), o.Retryable)
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
) endpoint.Endpoint {
	return NewEndpoint(src, factory, logger,
		WithMaxAttempts(1),
		WithTimeout(500*time.Millisecond),
		WithInvalidateOnError(5*time.Second),
	)
}

// NewEndpointCloser is like NewEndpoint but also returns an io.Closer that
// must be called to stop the background goroutine started by the Endpointer.
//
//	ep, closer := sd.NewEndpointCloser(instancer, factory, logger)
//	defer closer.Close()
func NewEndpointCloser(
	src interfaces.Instancer,
	factory endpoint.Factory,
	logger *kitlog.Logger,
	opts ...Option,
) (endpoint.Endpoint, io.Closer) {
	o := Options{
		MaxAttempts: 1,
		Timeout:     500 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(&o)
	}

	var epOpts []endpoint.EndpointerOption
	if o.InvalidateOnError > 0 {
		epOpts = append(epOpts, endpoint.InvalidateOnError(o.InvalidateOnError))
	}

	ep := endpointer.NewEndpointer(src, factory, logger, epOpts...)
	lb := balancer.NewRoundRobin(ep)

	if o.MaxAttempts < 1 {
		o.MaxAttempts = 1
	}
	e := executor.RetryWithRetryable(o.Timeout, lb, attemptLimit(o.MaxAttempts), o.Retryable)
	return e, ep
}

func attemptLimit(max int) executor.RetryCallback {
	return func(n int, err error) (keepTrying bool, replacement error) {
		return n < max, nil
	}
}
