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
//	    sd.WithMaxRetries(3),
//	    sd.WithTimeout(500*time.Millisecond),
//	    sd.WithInvalidateOnError(5*time.Second),
//	)
//	resp, err := ep(ctx, request)
package sd

import (
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd/endpointer"
	"github.com/dreamsxin/go-kit/sd/endpointer/balancer"
	"github.com/dreamsxin/go-kit/sd/endpointer/executor"
	"github.com/dreamsxin/go-kit/sd/interfaces"
)

// Options controls the behaviour of NewEndpoint.
type Options struct {
	// MaxRetries is the maximum number of retry attempts (default 3).
	// Set to 0 for unlimited retries within Timeout.
	MaxRetries int

	// Timeout is the total time budget for one call including all retries
	// (default 500ms).
	Timeout time.Duration

	// InvalidateOnError, when > 0, causes the endpoint cache to be cleared
	// after the given duration following a service-discovery error.
	InvalidateOnError time.Duration
}

// Option is a functional option for NewEndpoint.
type Option func(*Options)

// WithMaxRetries sets the maximum retry count.  0 means retry until Timeout.
func WithMaxRetries(n int) Option {
	return func(o *Options) { o.MaxRetries = n }
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

// NewEndpoint wires together an Instancer → Endpointer → RoundRobin balancer
// → Retry executor and returns a single endpoint.Endpoint ready to call.
//
// The returned endpoint automatically distributes requests across healthy
// instances and retries on transient failures.
func NewEndpoint(
	src interfaces.Instancer,
	factory endpoint.Factory,
	logger *kitlog.Logger,
	opts ...Option,
) endpoint.Endpoint {
	o := Options{
		MaxRetries: 3,
		Timeout:    500 * time.Millisecond,
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

	if o.MaxRetries <= 0 {
		return executor.RetryAlways(o.Timeout, lb)
	}
	return executor.Retry(o.MaxRetries, o.Timeout, lb)
}

// NewEndpointWithDefaults is identical to NewEndpoint but uses sensible
// production defaults without requiring any options:
//   - MaxRetries: 3
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
		WithMaxRetries(3),
		WithTimeout(500*time.Millisecond),
		WithInvalidateOnError(5*time.Second),
	)
}
