package endpoint

import (
	"errors"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd/events"

	"github.com/dreamsxin/go-kit/v2/log"
)

// EndpointCache maps service instance addresses to Endpoints.
// It is updated by Endpointer as service-discovery events arrive and
// optionally invalidates stale entries after a configurable grace period.
type EndpointCache struct {
	options            EndpointerOptions
	mtx                sync.RWMutex
	factory            Factory
	cache              map[string]EndpointCloser
	err                error
	endpoints          []Endpoint
	logger             *log.Logger
	invalidateDeadline time.Time
	timeNow            func() time.Time
	closed             bool
}

// ErrEndpointCacheClosed is returned after an EndpointCache has been closed.
var ErrEndpointCacheClosed = errors.New("endpoint cache closed")

// EndpointCloser pairs an Endpoint with an optional io.Closer so the cache
// can release resources when a service instance is removed.
type EndpointCloser struct {
	Endpoint
	io.Closer
}

// NewEndpointCache returns an EndpointCache that uses factory to create
// Endpoints for each discovered service instance.
func NewEndpointCache(factory Factory, logger *log.Logger, options EndpointerOptions) *EndpointCache {
	if factory == nil {
		panic("endpoint: nil endpoint factory")
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &EndpointCache{
		options: options,
		factory: factory,
		cache:   map[string]EndpointCloser{},
		logger:  logger,
		timeNow: time.Now,
	}
}

// Update reconciles the cache with the latest service-discovery event.
// Instances that appear in the event are kept (or created via the factory);
// stale instances have their Closers invoked. If event.Err is non-nil and
// InvalidateOnError is set, the cache begins returning the error after the
// configured grace period.
func (c *EndpointCache) Update(event events.Event) {
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
	if !c.options.InvalidateOnError {
		c.mtx.Unlock()
		return
	}
	if c.err != nil {
		c.mtx.Unlock()
		return
	}
	c.err = event.Err
	c.invalidateDeadline = c.timeNow().Add(c.options.InvalidateTimeout)
	c.mtx.Unlock()
}

func (c *EndpointCache) updateCacheLocked(instances []string) []io.Closer {
	instances = append([]string(nil), instances...)
	sort.Strings(instances)

	cache := make(map[string]EndpointCloser, len(instances))
	stale := make([]io.Closer, 0, len(c.cache))
	for _, instance := range instances {
		if sc, ok := c.cache[instance]; ok {
			cache[instance] = sc
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
		cache[instance] = EndpointCloser{service, closer}
	}

	// close stale endpoints
	for _, sc := range c.cache {
		if sc.Closer != nil {
			stale = append(stale, sc.Closer)
		}
	}

	endpoints := make([]Endpoint, 0, len(cache))
	for _, instance := range instances {
		if _, ok := cache[instance]; !ok {
			continue
		}
		endpoints = append(endpoints, cache[instance].Endpoint)
	}

	c.endpoints = endpoints
	c.cache = cache
	return stale
}

// Endpoints returns the current set of active Endpoints. If a discovery
// error is pending and the invalidation grace period has elapsed, the cache
// is cleared and the error is returned.
func (c *EndpointCache) Endpoints() ([]Endpoint, error) {
	c.mtx.RLock()
	if c.closed {
		c.mtx.RUnlock()
		return nil, ErrEndpointCacheClosed
	}

	if c.err == nil || c.timeNow().Before(c.invalidateDeadline) {
		endpoints := append([]Endpoint(nil), c.endpoints...)
		c.mtx.RUnlock()
		return endpoints, nil
	}

	c.mtx.RUnlock()

	c.mtx.Lock()
	if c.closed {
		c.mtx.Unlock()
		return nil, ErrEndpointCacheClosed
	}

	if c.err == nil || c.timeNow().Before(c.invalidateDeadline) {
		endpoints := append([]Endpoint(nil), c.endpoints...)
		c.mtx.Unlock()
		return endpoints, nil
	}

	stale := c.updateCacheLocked(nil)
	err := c.err
	c.mtx.Unlock()
	c.closeStale(stale)
	return nil, err
}

// Close releases every endpoint resource currently owned by the cache.
// It is safe to call more than once. Updates after Close are ignored.
func (c *EndpointCache) Close() error {
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
	c.cache = map[string]EndpointCloser{}
	c.endpoints = nil
	c.err = ErrEndpointCacheClosed
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

func (c *EndpointCache) closeStale(closers []io.Closer) {
	if err := closeEndpointClosers(closers); err != nil {
		c.logger.Sugar().Warnln("close stale endpoint resources", err)
	}
}
