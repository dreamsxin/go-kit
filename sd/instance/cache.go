package instance

import (
	"sort"
	"sync"

	"github.com/dreamsxin/go-kit/sd/events"

	"github.com/google/go-cmp/cmp"
)

// 缓存服务实例
type Cache struct {
	mtx   sync.RWMutex
	state events.Event
	reg   registry
}

func NewCache() *Cache {
	return &Cache{
		reg: registry{},
	}
}

// 服务实例事件，并发布通知
func (c *Cache) Update(event events.Event) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if event.Instances != nil {
		sort.Strings(event.Instances)
	}
	if cmp.Equal(c.state, event) {
		return
	}

	c.state = event
	c.reg.broadcast(event)
}

// 返回当前服务实例状态
func (c *Cache) State() events.Event {
	c.mtx.RLock()
	event := c.state
	c.mtx.RUnlock()
	eventCopy := copyEvent(event)
	return eventCopy
}

// 预留
func (c *Cache) Stop() {}

// 注册实例
func (c *Cache) Register(ch chan<- events.Event) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.reg.register(ch)
	event := c.state
	eventCopy := copyEvent(event)
	// 保证通道在读取或者有容量
	ch <- eventCopy
}

// 注销
func (c *Cache) Deregister(ch chan<- events.Event) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.reg.deregister(ch)
}
