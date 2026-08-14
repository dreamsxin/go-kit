package endpointer

import (
	"errors"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/log"
	"github.com/dreamsxin/go-kit/v2/sd"
)

// Factory creates an endpoint for a discovered service instance. The closer,
// when non-nil, is owned by Cache and released when the instance disappears.
type Factory func(instance string) (endpoint.Endpoint, io.Closer, error)

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
	endpoints          []endpoint.Endpoint
	logger             *log.Logger
	invalidateDeadline time.Time
	timeNow            func() time.Time
	closed             bool
}

// NewCache creates an endpoint cache owned by the service-discovery layer.
func NewCache(factory Factory, logger *log.Logger, options Options) *Cache {
	if factory == nil {
		panic("endpointer: nil endpoint factory")
	}
	if logger == nil {
		logger = log.NewNopLogger()
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

	c.logger.Sugar().Debugln("err", event.Err)
	if !c.options.InvalidateOnError || c.err != nil {
		c.mtx.Unlock()
		return
	}
	c.err = event.Err
	c.invalidateDeadline = c.timeNow().Add(c.options.InvalidateTimeout)
	c.mtx.Unlock()
}

func (c *Cache) updateCacheLocked(instances []string) []io.Closer {
	instances = append([]string(nil), instances...)
	sort.Strings(instances)

	cache := make(map[string]endpointCloser, len(instances))
	stale := make([]io.Closer, 0, len(c.cache))
	for _, instance := range instances {
		if item, ok := c.cache[instance]; ok {
			cache[instance] = item
			delete(c.cache, instance)
			continue
		}

		service, closer, err := c.factory(instance)
		if err != nil {
			c.logger.Sugar().Debugln("instance", instance, "err", err)
			if closer != nil {
				stale = append(stale, closer)
			}
			continue
		}
		if service == nil {
			c.logger.Sugar().Debugln("instance", instance, "err", "factory returned nil endpoint")
			if closer != nil {
				stale = append(stale, closer)
			}
			continue
		}
		cache[instance] = endpointCloser{Endpoint: service, Closer: closer}
	}

	for _, item := range c.cache {
		if item.Closer != nil {
			stale = append(stale, item.Closer)
		}
	}

	endpoints := make([]endpoint.Endpoint, 0, len(cache))
	for _, instance := range instances {
		item, ok := cache[instance]
		if !ok {
			continue
		}
		endpoints = append(endpoints, item.Endpoint)
	}

	c.endpoints = endpoints
	c.cache = cache
	return stale
}

// Endpoints returns a snapshot of the active endpoints.
func (c *Cache) Endpoints() ([]endpoint.Endpoint, error) {
	c.mtx.RLock()
	if c.closed {
		c.mtx.RUnlock()
		return nil, ErrCacheClosed
	}

	if c.err == nil || c.timeNow().Before(c.invalidateDeadline) {
		endpoints := append([]endpoint.Endpoint(nil), c.endpoints...)
		c.mtx.RUnlock()
		return endpoints, nil
	}
	c.mtx.RUnlock()

	c.mtx.Lock()
	if c.closed {
		c.mtx.Unlock()
		return nil, ErrCacheClosed
	}
	if c.err == nil || c.timeNow().Before(c.invalidateDeadline) {
		endpoints := append([]endpoint.Endpoint(nil), c.endpoints...)
		c.mtx.Unlock()
		return endpoints, nil
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
	c.endpoints = nil
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
		c.logger.Sugar().Warnln("close stale endpoint resources", err)
	}
}
