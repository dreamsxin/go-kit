package endpointer

import (
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
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
type Cache struct {
	options            Options
	mtx                sync.RWMutex
	factory            Factory
	cache              map[string]endpointCloser
	err                error
	instances          []InstanceEndpoint
	logger             *slog.Logger
	invalidateDeadline time.Time
	timeNow            func() time.Time
	closed             bool
}

// NewCache creates an endpoint cache owned by the service-discovery layer.
func NewCache(factory Factory, logger *slog.Logger, options Options) *Cache {
	if factory == nil {
		panic("endpointer: nil endpoint factory")
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Cache{
		options: options,
		factory: factory,
		cache:   map[string]endpointCloser{},
		logger:  logger,
		timeNow: time.Now,
	}
}

// Update reconciles the cache with a service-discovery event.
func (c *Cache) Update(event sd.Event) {
	c.mtx.Lock()
	if c.closed {
		c.mtx.Unlock()
		return
	}

	if event.Err == nil {
		stale := c.updateCacheLocked(event.Instances)
		c.err = nil
		c.mtx.Unlock()
		c.closeStale(stale)
		return
	}

	c.logger.Debug("service discovery update failed", "err", event.Err)
	if !c.options.InvalidateOnError || c.err != nil {
		c.mtx.Unlock()
		return
	}
	c.err = event.Err
	c.invalidateDeadline = c.timeNow().Add(c.options.InvalidateTimeout)
	c.mtx.Unlock()
}

func (c *Cache) updateCacheLocked(instances []sd.Instance) []io.Closer {
	instances = append([]sd.Instance(nil), instances...)
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Address < instances[j].Address
	})

	cache := make(map[string]endpointCloser, len(instances))
	stale := make([]io.Closer, 0, len(c.cache))
	endpoints := make([]InstanceEndpoint, 0, len(instances))
	for _, instance := range instances {
		if _, duplicate := cache[instance.Address]; duplicate {
			// Building a second endpoint for the same address would leak the
			// first, because the cache holds one closer per address.
			c.logger.Debug("duplicate instance in snapshot", "instance", instance.Address)
			continue
		}
		if item, ok := c.cache[instance.Address]; ok {
			// Labels can change without the address changing. Reuse the live
			// endpoint and publish the new labels; rebuilding would drop a
			// working connection over a relabel.
			cache[instance.Address] = item
			delete(c.cache, instance.Address)
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
		cache[instance.Address] = endpointCloser{Endpoint: service, Closer: closer}
		endpoints = append(endpoints, InstanceEndpoint{Instance: instance, Endpoint: service})
	}

	for _, item := range c.cache {
		if item.Closer != nil {
			stale = append(stale, item.Closer)
		}
	}

	c.instances = endpoints
	c.cache = cache
	return stale
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
func (c *Cache) InstanceEndpoints() ([]InstanceEndpoint, error) {
	c.mtx.RLock()
	if c.closed {
		c.mtx.RUnlock()
		return nil, ErrCacheClosed
	}

	if c.err == nil || c.timeNow().Before(c.invalidateDeadline) {
		instances := append([]InstanceEndpoint(nil), c.instances...)
		c.mtx.RUnlock()
		return instances, nil
	}
	c.mtx.RUnlock()

	c.mtx.Lock()
	if c.closed {
		c.mtx.Unlock()
		return nil, ErrCacheClosed
	}
	if c.err == nil || c.timeNow().Before(c.invalidateDeadline) {
		instances := append([]InstanceEndpoint(nil), c.instances...)
		c.mtx.Unlock()
		return instances, nil
	}

	stale := c.updateCacheLocked(nil)
	err := c.err
	c.mtx.Unlock()
	c.closeStale(stale)
	return nil, err
}

// Close releases all endpoint resources owned by the cache.
func (c *Cache) Close() error {
	c.mtx.Lock()
	if c.closed {
		c.mtx.Unlock()
		return nil
	}
	c.closed = true
	closers := make([]io.Closer, 0, len(c.cache))
	for _, item := range c.cache {
		if item.Closer != nil {
			closers = append(closers, item.Closer)
		}
	}
	c.cache = map[string]endpointCloser{}
	c.instances = nil
	c.err = ErrCacheClosed
	c.mtx.Unlock()
	return closeEndpointClosers(closers)
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

func (c *Cache) closeStale(closers []io.Closer) {
	if err := closeEndpointClosers(closers); err != nil {
		c.logger.Warn("close stale endpoint resources", "err", err)
	}
}
