// Package client composes discovery, balancing, and retry into one endpoint.
package client

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/retry"
)

// BalancerFactory builds the Balancer that selects among discovered endpoints.
// It receives the live endpoint set, so a factory may pick any strategy in
// sd/balancer or supply its own.
type BalancerFactory func(endpointer.InstanceEndpointer) sd.Balancer

// Options controls NewEndpoint.
type Options struct {
	MaxAttempts       int
	Timeout           time.Duration
	InvalidateOnError time.Duration
	Retryable         retry.Classifier
	Balancer          BalancerFactory
}

// Option configures NewEndpoint.
type Option func(*Options)

// WithMaxAttempts sets the total number of attempts, including the first call.
func WithMaxAttempts(attempts int) Option {
	return func(options *Options) { options.MaxAttempts = attempts }
}

// WithTimeout sets the total time budget across all attempts and backoff.
func WithTimeout(timeout time.Duration) Option {
	return func(options *Options) { options.Timeout = timeout }
}

// WithInvalidateOnError sets the discovery-error cache grace period.
func WithInvalidateOnError(timeout time.Duration) Option {
	return func(options *Options) { options.InvalidateOnError = timeout }
}

// WithRetryable installs an application or protocol-specific classifier.
func WithRetryable(classifier retry.Classifier) Option {
	return func(options *Options) { options.Retryable = classifier }
}

// WithBalancer replaces the default round-robin selection strategy.
//
//	client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
//		return balancer.NewConsistentHash(set, tenantKey)
//	})
func WithBalancer(factory BalancerFactory) Option {
	return func(options *Options) { options.Balancer = factory }
}

// NewEndpoint composes an Endpointer, a Balancer, and a retry executor. The
// Balancer defaults to round robin; override it with WithBalancer.
// A nil logger falls back to slog.Default().
func NewEndpoint(src sd.Instancer, factory endpointer.Factory, logger *slog.Logger, opts ...Option) (endpoint.Endpoint, io.Closer, error) {
	options := Options{
		MaxAttempts: 1,
		Timeout:     500 * time.Millisecond,
		Balancer:    defaultBalancer,
	}
	for i, option := range opts {
		if option == nil {
			return nil, nil, fmt.Errorf("sd/client: option %d is nil", i)
		}
		option(&options)
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := validate(src, factory, options); err != nil {
		return nil, nil, err
	}

	var endpointerOptions []endpointer.Option
	if options.InvalidateOnError > 0 {
		endpointerOptions = append(endpointerOptions, endpointer.InvalidateOnError(options.InvalidateOnError))
	}
	endpointSet := endpointer.NewEndpointer(src, factory, logger, endpointerOptions...)
	balanced := options.Balancer(endpointSet)
	if balanced == nil {
		// The endpointer already started its update goroutine, so release it
		// before reporting the misconfiguration.
		_ = endpointSet.Close()
		return nil, nil, fmt.Errorf("sd/client: balancer factory returned nil")
	}
	call := retry.WithClassifier(options.Timeout, balanced, attemptLimit(options.MaxAttempts), options.Retryable)
	return call, &resources{balancer: balanced, endpoints: endpointSet}, nil
}

// NewEndpointWithDefaults uses one attempt, a 500ms total timeout, and a five
// second invalidation grace period.
func NewEndpointWithDefaults(src sd.Instancer, factory endpointer.Factory, logger *slog.Logger) (endpoint.Endpoint, io.Closer, error) {
	return NewEndpoint(src, factory, logger,
		WithMaxAttempts(1),
		WithTimeout(500*time.Millisecond),
		WithInvalidateOnError(5*time.Second),
	)
}

func defaultBalancer(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewRoundRobin(set)
}

func validate(src sd.Instancer, factory endpointer.Factory, options Options) error {
	switch {
	case isNil(src):
		return fmt.Errorf("sd/client: instancer is nil")
	case factory == nil:
		return fmt.Errorf("sd/client: endpoint factory is nil")
	case options.Balancer == nil:
		return fmt.Errorf("sd/client: balancer factory is nil")
	case options.MaxAttempts < 1:
		return fmt.Errorf("sd/client: max attempts must be at least 1")
	case options.Timeout <= 0:
		return fmt.Errorf("sd/client: timeout must be greater than zero")
	case options.InvalidateOnError < 0:
		return fmt.Errorf("sd/client: invalidate-on-error duration cannot be negative")
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

func attemptLimit(max int) retry.Callback {
	return func(attempt int, _ error) (bool, error) { return attempt < max, nil }
}

type resources struct {
	balancer  sd.Balancer
	endpoints io.Closer
	once      sync.Once
	err       error
}

func (r *resources) Close() error {
	r.once.Do(func() {
		balancerErr := r.balancer.Close()
		endpointErr := r.endpoints.Close()
		r.err = errors.Join(balancerErr, endpointErr)
	})
	return r.err
}
