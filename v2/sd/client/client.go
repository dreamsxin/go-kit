// Package client assembles the whole client side into one endpoint.
//
// Discovery, endpoint management, selection, balancing and retry are separate
// packages so that each can be replaced. Most services do not want to replace any
// of them, and wiring five layers by hand to get the ordinary arrangement is a way
// to get it subtly wrong. NewEndpoint is the ordinary arrangement: give it an
// sd.Instancer and a Factory, and receive one endpoint.Endpoint that discovers,
// selects, calls and retries.
//
// It is a convenience, not a ceiling. Every layer stays reachable: BalancerFactory
// receives the live endpoint set, so a deployment picks any strategy in sd/balancer
// or supplies its own, and a service that wants a shape this package does not offer
// composes the packages directly — nothing here is privileged.
//
// InvalidateOnError is the one option worth understanding before setting it. The
// default — zero — keeps serving the last known instance list for as long as
// discovery keeps failing, because instances that are still up should keep
// receiving traffic: a registry that is unreachable should not take the caller
// down with it. A positive duration drops that list once the duration has elapsed
// from the *first* error of a failure streak; later errors do not push the
// deadline out, or a registry failing every second would keep the snapshot alive
// forever. NewEndpointWithDefaults chooses five seconds.
//
// The returned endpoint owns background goroutines, so the io.Closer it comes with
// is not optional.
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
	// RetryClock controls backoff waits only; nil uses real timers. Discovery
	// invalidation, total timeout and measured latency still use real time.
	RetryClock endpoint.Clock
	// RetryBackoff receives the failed attempt number, starting at one.
	// Nil uses retry.New's default schedule.
	RetryBackoff func(attempt int) time.Duration
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

// WithRetryClock selects the clock for retry backoff. Nil restores system timers.
// It does not change discovery invalidation, the total timeout or measured
// latency. The clock must support concurrent calls to the returned endpoint.
// A typed-nil clock is rejected by NewEndpoint before opening a subscription.
//
// Stable: sd.client-retry-timing-is-configurable — the assembled client passes its retry clock and backoff through to execution without changing attempt caps, wall-clock timeout, classification or resource ownership; cancellation releases the wait.
// Covered by: TestNewEndpoint_ConfiguredRetryTiming, TestNewEndpoint_RetryCancellationAndDeadline, TestNewEndpoint_RetryDefaultsAndPolicyRemainIntact
func WithRetryClock(clock endpoint.Clock) Option {
	return func(options *Options) { options.RetryClock = clock }
}

// WithRetryBackoff sets the wait after failed attempt n, numbered from one.
// Non-positive waits retry immediately, still subject to the attempt cap and
// timeout. Nil restores retry.New's default schedule. The function must return
// promptly and be safe for concurrent calls to the returned endpoint.
func WithRetryBackoff(backoff func(attempt int) time.Duration) Option {
	return func(options *Options) { options.RetryBackoff = backoff }
}

// WithBalancer replaces the default round-robin selection strategy.
//
//	client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
//		return balancer.NewConsistentHash(set, tenantKey)
//	})
func WithBalancer(factory BalancerFactory) Option {
	return func(options *Options) { options.Balancer = factory }
}

// NewEndpoint composes an InstanceEndpointer, a Balancer, and a retry executor.
// The Balancer defaults to round robin; override it with WithBalancer.
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
	// Stable: sd.client-invalid-balancer-releases-resources — a nil or typed-nil balancer result fails construction after releasing the endpoint subscription and factory resources, and cleanup failures remain reachable in the returned error.
	// Covered by: TestNewEndpoint_InvalidBalancerReleasesResourcesAndReportsCleanup
	if isNil(balanced) {
		// The endpointer already started its update goroutine, so release it
		// before reporting the misconfiguration.
		return nil, nil, errors.Join(fmt.Errorf("sd/client: balancer factory returned nil"), endpointSet.Close())
	}
	call := retry.New(balanced,
		retry.WithMaxAttempts(options.MaxAttempts),
		retry.WithTimeout(options.Timeout),
		retry.WithErrorClassifier(options.Retryable),
		retry.WithClock(options.RetryClock),
		retry.WithBackoff(options.RetryBackoff),
	)
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
	case options.RetryClock != nil && isNil(options.RetryClock):
		return fmt.Errorf("sd/client: retry clock is a typed nil")
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
