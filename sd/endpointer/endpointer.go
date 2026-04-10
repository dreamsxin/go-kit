package endpointer

import (
	"io"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/interfaces"
)

// Endpointer resolves a set of live Endpoints from a service-discovery source.
// It subscribes to an Instancer and keeps an EndpointCache up to date.
// Close must be called to stop the background goroutine and release resources.
type Endpointer interface {
	io.Closer
	Endpoints() ([]endpoint.Endpoint, error)
}

// NewEndpointer creates an Endpointer that subscribes to src and builds
// Endpoints using f.  It starts a background goroutine to process events;
// call Close() on the returned value to stop it.
func NewEndpointer(src interfaces.Instancer, f endpoint.Factory, logger *log.Logger, options ...endpoint.EndpointerOption) Endpointer {
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

func (de *DefaultEndpointer) Close() error {
	de.instancer.Deregister(de.ch)
	close(de.ch)
	return nil
}

func (de *DefaultEndpointer) Endpoints() ([]endpoint.Endpoint, error) {
	return de.cache.Endpoints()
}
