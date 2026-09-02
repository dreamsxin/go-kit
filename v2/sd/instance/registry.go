package instance

import (
	"maps"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// registry stores event listeners and broadcasts events to all of them.
type registry map[chan sd.Event]struct{}

func broadcast(subscribers []chan sd.Event, event sd.Event) {
	for _, c := range subscribers {
		sendLatest(c, event)
	}
}

func sendLatest(ch chan sd.Event, event sd.Event) {
	select {
	case ch <- copyEvent(event):
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- copyEvent(event):
	default:
	}
}

func (r registry) register(c chan sd.Event) {
	r[c] = struct{}{}
}

func (r registry) deregister(c chan sd.Event) {
	delete(r, c)
}

func (r registry) subscribers() []chan sd.Event {
	out := make([]chan sd.Event, 0, len(r))
	for c := range r {
		out = append(out, c)
	}
	return out
}

func copyEvent(e sd.Event) sd.Event {
	if e.Instances == nil {
		return e
	}
	instances := make([]sd.Instance, len(e.Instances))
	for i, item := range e.Instances {
		instances[i] = sd.Instance{Address: item.Address, Metadata: maps.Clone(item.Metadata)}
	}
	e.Instances = instances
	return e
}
