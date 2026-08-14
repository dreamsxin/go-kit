// Package client composes discovery, balancing, and retry into one endpoint.
package client

import (
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/retry"
)

// Options controls NewEndpoint.
type Options struct {
	MaxAttempts       int
	Timeout           time.Duration
	InvalidateOnError time.Duration
	Retryable         retry.Classifier
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

// NewEndpoint composes an Endpointer, round-robin Balancer, and retry executor.
func NewEndpoint(src sd.Instancer, factory endpointer.Factory, logger *slog.Logger, opts ...Option) (endpoint.Endpoint, io.Closer, error) {
	options := Options{MaxAttempts: 1, Timeout: 500 * time.Millisecond}
	for i, option := range opts {
		if option == nil {
			return nil, nil, fmt.Errorf("sd/client: option %d is nil", i)
		}
		option(&options)
	}
	if err := validate(src, factory, logger, options); err != nil {
		return nil, nil, err
	}

	var endpointerOptions []endpointer.Option
	if options.InvalidateOnError > 0 {
		endpointerOptions = append(endpointerOptions, endpointer.InvalidateOnError(options.InvalidateOnError))
	}
	endpointSet := endpointer.NewEndpointer(src, factory, logger, endpointerOptions...)
	balanced := balancer.NewRoundRobin(endpointSet)
	call := retry.WithClassifier(options.Timeout, balanced, attemptLimit(options.MaxAttempts), options.Retryable)
	return call, endpointSet, nil
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

func validate(src sd.Instancer, factory endpointer.Factory, logger *slog.Logger, options Options) error {
	switch {
	case isNil(src):
		return fmt.Errorf("sd/client: instancer is nil")
	case factory == nil:
		return fmt.Errorf("sd/client: endpoint factory is nil")
	case logger == nil:
		return fmt.Errorf("sd/client: logger is nil")
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
