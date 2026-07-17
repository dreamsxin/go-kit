package endpointer

import (
	"io"
	"sync"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/log"
	"github.com/dreamsxin/go-kit/v2/sd/events"
	"github.com/dreamsxin/go-kit/v2/sd/interfaces"
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
		ch:        make(chan events.Event, 1),
		done:      make(chan struct{}),
	}
	initial := src.Register(se.ch)
	se.cache.Update(initial)
	go se.receive()
	return se
}

type DefaultEndpointer struct {
	cache     *endpoint.EndpointCache
	instancer interfaces.Instancer
	ch        chan events.Event
	done      chan struct{}
	closeOnce sync.Once
}

func (de *DefaultEndpointer) receive() {
	for {
		select {
		case event := <-de.ch:
			de.cache.Update(event)
		case <-de.done:
			return
		}
	}
}

func (de *DefaultEndpointer) Close() error {
	de.closeOnce.Do(func() {
		de.instancer.Deregister(de.ch)
		close(de.done)
	})
	return nil
}

func (de *DefaultEndpointer) Endpoints() ([]endpoint.Endpoint, error) {
	return de.cache.Endpoints()
}
