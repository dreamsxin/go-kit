package endpointer

import (
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/internal/subscription"
)

// Factory creates an endpoint for a discovered service instance. The closer,
// when non-nil, is owned by Cache and released when the instance disappears.
// The whole instance is passed, not just its address, so a factory can honour
// registration labels that decide how to connect — scheme, tls, protocol.
type Factory func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error)

// InstanceEndpoint pairs a discovered instance with the endpoint the Factory
// built for it. Balancers that need instance identity or labels, such as
// weighted, subset, or hash-based strategies, select over these instead of
// bare endpoints.
type InstanceEndpoint struct {
	Instance sd.Instance
	Endpoint endpoint.Endpoint
}

// Address is the discovered address of this instance.
func (i InstanceEndpoint) Address() string { return i.Instance.Address }

// Metadata returns the labels the instance reported when it registered.
func (i InstanceEndpoint) Metadata() map[string]any { return i.Instance.Metadata }

// Options controls cache invalidation after service-discovery errors.
type Options struct {
	InvalidateOnError bool
	InvalidateTimeout time.Duration
}

// Option configures an Endpointer.
type Option func(*Options)

// InvalidateOnError clears cached endpoints after timeout has elapsed from the
// first service-discovery error.
func InvalidateOnError(timeout time.Duration) Option {
	return func(options *Options) {
		options.InvalidateOnError = true
		options.InvalidateTimeout = timeout
	}
}

// ErrCacheClosed is returned after a Cache has been closed.
var ErrCacheClosed = errors.New("endpointer cache closed")

type endpointCloser struct {
	endpoint.Endpoint
	io.Closer
}

// Cache maps discovered instance addresses to live endpoints.
//
// Subscribing, holding the last good snapshot through a registry outage, and
// dropping it once the grace period has elapsed are the shared state machine in
// sd/internal/subscription, so a Cache and a selector Subscription answer an
// outage the same way. What belongs here is the projection: build one endpoint
// per address, keep the one that is already live, and close the ones a snapshot
// retired.
type Cache struct {
	factory Factory
	logger  *slog.Logger
	state   *subscription.State[[]InstanceEndpoint]

	// live maps each published address to its endpoint. Only reconcile touches
	// it, and the state machine runs reconcile under its own lock, so it needs
	// no lock here.
	live map[string]endpointCloser
}

// NewCache creates an endpoint cache owned by the service-discovery layer.
func NewCache(factory Factory, logger *slog.Logger, options Options) *Cache {
	if factory == nil {
		panic("endpointer: nil endpoint factory")
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	cache := &Cache{
		factory: factory,
		logger:  logger,
		live:    map[string]endpointCloser{},
	}
	cache.state = subscription.NewState(cache.reconcile, ErrCacheClosed, logger, subscription.Options{
		InvalidateOnError: options.InvalidateOnError,
		InvalidateTimeout: options.InvalidateTimeout,
	})
	return cache
}

// Update reconciles the cache with a service-discovery event.
func (c *Cache) Update(event sd.Event) {
	c.state.Update(event)
}

// reconcile builds the endpoint set for one snapshot and reports the resources
// the previous set owned and this one does not. A nil snapshot is how the state
// machine expresses invalidation and closing, and it falls out of the same walk:
// nothing is kept, so everything becomes stale.
func (c *Cache) reconcile(instances []sd.Instance) ([]InstanceEndpoint, func() error) {
	instances = append([]sd.Instance(nil), instances...)
	subscription.SortInstances(instances)

	live := make(map[string]endpointCloser, len(instances))
	stale := make([]io.Closer, 0, len(c.live))
	endpoints := make([]InstanceEndpoint, 0, len(instances))
	for _, instance := range instances {
		if _, duplicate := live[instance.Address]; duplicate {
			// Building a second endpoint for the same address would leak the
			// first, because the cache holds one closer per address.
			c.logger.Debug("duplicate instance in snapshot", "instance", instance.Address)
			continue
		}
		if item, ok := c.live[instance.Address]; ok {
			// Labels can change without the address changing. Reuse the live
			// endpoint and publish the new labels; rebuilding would drop a
			// working connection over a relabel.
			live[instance.Address] = item
			delete(c.live, instance.Address)
			endpoints = append(endpoints, InstanceEndpoint{Instance: instance, Endpoint: item.Endpoint})
			continue
		}

		service, closer, err := c.factory(instance)
		if err != nil {
			c.logger.Debug("create endpoint failed", "instance", instance.Address, "err", err)
			if closer != nil {
				stale = append(stale, closer)
			}
			continue
		}
		if service == nil {
			c.logger.Debug("create endpoint failed", "instance", instance.Address, "err", "factory returned nil endpoint")
			if closer != nil {
				stale = append(stale, closer)
			}
			continue
		}
		live[instance.Address] = endpointCloser{Endpoint: service, Closer: closer}
		endpoints = append(endpoints, InstanceEndpoint{Instance: instance, Endpoint: service})
	}

	for _, item := range c.live {
		if item.Closer != nil {
			stale = append(stale, item.Closer)
		}
	}
	c.live = live

	if len(stale) == 0 {
		return endpoints, nil
	}
	return endpoints, func() error { return closeEndpointClosers(stale) }
}

// Endpoints returns a snapshot of the active endpoints.
func (c *Cache) Endpoints() ([]endpoint.Endpoint, error) {
	instances, err := c.InstanceEndpoints()
	if err != nil {
		return nil, err
	}
	endpoints := make([]endpoint.Endpoint, len(instances))
	for i, item := range instances {
		endpoints[i] = item.Endpoint
	}
	return endpoints, nil
}

// InstanceEndpoints returns a snapshot of the active endpoints together with
// the instance address each one was built from, ordered by address.
//
// The result is the published snapshot itself, not a copy: it is replaced whole
// on every discovery update and never edited in place, so a reader keeps a
// consistent view for as long as it holds the slice. Do not modify it — sort or
// filter into a slice of your own.
func (c *Cache) InstanceEndpoints() ([]InstanceEndpoint, error) {
	return c.state.Value()
}

// Close releases all endpoint resources owned by the cache. It reports the
// errors closing them, joined, and returns nil on a second call.
func (c *Cache) Close() error {
	release := c.state.Close()
	if release == nil {
		return nil
	}
	return release()
}

func closeEndpointClosers(closers []io.Closer) error {
	errs := make([]error, 0, len(closers))
	for _, closer := range closers {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

