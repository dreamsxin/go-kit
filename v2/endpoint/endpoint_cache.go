package endpoint

import (
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
}

// EndpointCloser pairs an Endpoint with an optional io.Closer so the cache
// can release resources when a service instance is removed.
type EndpointCloser struct {
	Endpoint
	io.Closer
}

// NewEndpointCache returns an EndpointCache that uses factory to create
// Endpoints for each discovered service instance.
func NewEndpointCache(factory Factory, logger *log.Logger, options EndpointerOptions) *EndpointCache {
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
	defer c.mtx.Unlock()

	if event.Err == nil {
		c.updateCache(event.Instances)
		c.err = nil
		return
	}

	c.logger.Sugar().Debugln("err", event.Err)
	if !c.options.InvalidateOnError {
		return
	}
	if c.err != nil {
		return
	}
	c.err = event.Err
	c.invalidateDeadline = c.timeNow().Add(c.options.InvalidateTimeout)
}

func (c *EndpointCache) updateCache(instances []string) {
	sort.Strings(instances)

	cache := make(map[string]EndpointCloser, len(instances))
	for _, instance := range instances {
		if sc, ok := c.cache[instance]; ok {
			cache[instance] = sc
			delete(c.cache, instance)
			continue
		}

		service, closer, err := c.factory(instance)
		if err != nil {
			c.logger.Sugar().Debugln("instance", instance, "err", err)
			continue
		}
		cache[instance] = EndpointCloser{service, closer}
	}

	// close stale endpoints
	for _, sc := range c.cache {
		if sc.Closer != nil {
			sc.Closer.Close()
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
}

// Endpoints returns the current set of active Endpoints. If a discovery
// error is pending and the invalidation grace period has elapsed, the cache
// is cleared and the error is returned.
func (c *EndpointCache) Endpoints() ([]Endpoint, error) {
	c.mtx.RLock()

	if c.err == nil || c.timeNow().Before(c.invalidateDeadline) {
		defer c.mtx.RUnlock()
		return c.endpoints, nil
	}

	c.mtx.RUnlock()

	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.err == nil || c.timeNow().Before(c.invalidateDeadline) {
		return c.endpoints, nil
	}

	c.updateCache(nil)
	return nil, c.err
}
