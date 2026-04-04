package instance

import "github.com/dreamsxin/go-kit/sd/events"

// registry stores event listeners and broadcasts events to all of them.
type registry map[chan<- events.Event]struct{}

func (r registry) broadcast(event events.Event) {
	for c := range r {
		c <- copyEvent(event)
	}
}

func (r registry) register(c chan<- events.Event) {
	r[c] = struct{}{}
}

func (r registry) deregister(c chan<- events.Event) {
	delete(r, c)
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
