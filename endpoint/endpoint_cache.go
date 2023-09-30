package endpoint

import (
	"io"
	"sort"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/sd/events"

	"go.uber.org/zap"
)

// 缓存端点实例
type EndpointCache struct {
	options            EndpointerOptions
	mtx                sync.RWMutex
	factory            Factory
	cache              map[string]EndpointCloser
	err                error
	endpoints          []Endpoint
	logger             *zap.SugaredLogger
	invalidateDeadline time.Time
	timeNow            func() time.Time
}

type EndpointCloser struct {
	Endpoint
	io.Closer
}

func NewEndpointCache(factory Factory, logger *zap.SugaredLogger, options EndpointerOptions) *EndpointCache {
	return &EndpointCache{
		options: options,
		factory: factory,
		cache:   map[string]EndpointCloser{},
		logger:  logger,
		timeNow: time.Now,
	}
}

func (c *EndpointCache) Update(event events.Event) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if event.Err == nil {
		c.updateCache(event.Instances)
		c.err = nil
		return
	}

	c.logger.Debugln("err", event.Err)
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
			c.logger.Debugln("instance", instance, "err", err)
			continue
		}
		cache[instance] = EndpointCloser{service, closer}
	}

	// 关闭端点
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
