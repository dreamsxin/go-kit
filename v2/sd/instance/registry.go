package instance

import "github.com/dreamsxin/go-kit/v2/sd/events"

// registry stores event listeners and broadcasts events to all of them.
type registry map[chan<- events.Event]struct{}

func broadcast(subscribers []chan<- events.Event, event events.Event) {
	for _, c := range subscribers {
		select {
		case c <- copyEvent(event):
		default:
		}
	}
}

func (r registry) register(c chan<- events.Event) {
	r[c] = struct{}{}
}

func (r registry) deregister(c chan<- events.Event) {
	delete(r, c)
}

func (r registry) subscribers() []chan<- events.Event {
	out := make([]chan<- events.Event, 0, len(r))
	for c := range r {
		out = append(out, c)
	}
	return out
}

func copyEvent(e events.Event) events.Event {
	if e.Instances == nil {
		return e
	}
	instances := make([]string, len(e.Instances))
	copy(instances, e.Instances)
	e.Instances = instances
	return e
}
