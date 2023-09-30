package endpointer

import (
	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/interfaces"

	"go.uber.org/zap"
)

// 端点生成器：根据服务发现类获取的服务地址以及端点构建工厂创建端点
type Endpointer interface {
	Endpoints() ([]endpoint.Endpoint, error)
}

func NewEndpointer(src interfaces.Instancer, f endpoint.Factory, logger *zap.SugaredLogger, options ...endpoint.EndpointerOption) Endpointer {
	opts := endpoint.EndpointerOptions{}
	for _, opt := range options {
		opt(&opts)
	}
	se := &DefaultEndpointer{
		cache:     endpoint.NewEndpointCache(f, logger, opts),
		instancer: src,
		ch:        make(chan events.Event),
	}
	go se.receive()
	src.Register(se.ch)
	return se
}

type DefaultEndpointer struct {
	cache     *endpoint.EndpointCache
	instancer interfaces.Instancer
	ch        chan events.Event
}

func (de *DefaultEndpointer) receive() {
	for event := range de.ch {
		de.cache.Update(event)
	}
}

func (de *DefaultEndpointer) Close() {
	de.instancer.Deregister(de.ch)
	close(de.ch)
}

func (de *DefaultEndpointer) Endpoints() ([]endpoint.Endpoint, error) {
	return de.cache.Endpoints()
}
