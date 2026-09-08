// Package endpointer turns a discovery source into a live set of endpoints.
//
// It is the layer between discovery and selection. sd.Instancer reports which
// instances exist; an Endpointer subscribes to it and maintains the
// endpoint.Endpoint for each one, creating endpoints for instances that appear
// and closing them for instances that leave. Selection and balancing read the
// result and never talk to a registry.
//
// Two contracts, because they answer different questions. Endpointer answers
// "which endpoints can I call" and is what application code should hold.
// InstanceEndpointer also reports the address behind each endpoint, which is the
// identity weighted, hash-based and feedback-driven strategies need in order to
// score an instance. Everything constructed here returns an InstanceEndpointer.
//
// An Endpointer owns a background goroutine, so Close is not optional: it stops
// the subscription and closes the endpoints the cache is holding.
//
// A Factory builds and tears down one endpoint for one address. That is the seam
// an application implements — the transport, the codec, the per-instance
// middleware are all decided there, and this package never assumes a protocol.
package endpointer

import (
	"io"
	"log/slog"
	"sync"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/internal/subscription"
)

// Endpointer resolves a set of live Endpoints from a service-discovery source.
// It subscribes to an Instancer and keeps a Cache up to date.
// Close must be called to stop the background goroutine and release resources.
type Endpointer interface {
	io.Closer
	Endpoints() ([]endpoint.Endpoint, error)
}

// InstanceEndpointer also reports the instance address behind each endpoint,
// the identity weighted, hash-based, and feedback-driven strategies need.
// Everything this module builds returns an InstanceEndpointer; Endpointer stays
// separate for an application that only consumes endpoints and wants to say so
// in its own signatures.
type InstanceEndpointer interface {
	Endpointer
	InstanceEndpoints() ([]InstanceEndpoint, error)
}

// NewEndpointer creates an Endpointer that subscribes to src and builds
// Endpoints using f.  It starts a background goroutine to process events;
// call Close() on the returned value to stop it.
func NewEndpointer(src sd.Instancer, f Factory, logger *slog.Logger, options ...Option) InstanceEndpointer {
	opts := Options{}
	for _, opt := range options {
		opt(&opts)
	}
	se := &DefaultEndpointer{cache: NewCache(f, logger, opts)}
	se.feed = subscription.Start(src, se.cache.Update)
	return se
}

type DefaultEndpointer struct {
	cache     *Cache
	feed      *subscription.Feed
	closeOnce sync.Once
	closeErr  error
}

// Close stops the subscription and releases every endpoint resource the cache
// owns. The pump is joined before the cache closes, so an event already in
// flight is applied rather than dropped half-way.
func (de *DefaultEndpointer) Close() error {
	de.closeOnce.Do(func() {
		de.feed.Stop()
		de.closeErr = de.cache.Close()
	})
	return de.closeErr
}

func (de *DefaultEndpointer) Endpoints() ([]endpoint.Endpoint, error) {
	return de.cache.Endpoints()
}

// InstanceEndpoints reports the active endpoints with their instance addresses.
func (de *DefaultEndpointer) InstanceEndpoints() ([]InstanceEndpoint, error) {
	return de.cache.InstanceEndpoints()
}
