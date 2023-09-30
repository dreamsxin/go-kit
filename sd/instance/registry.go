package instance

import (
	"log"

	"github.com/dreamsxin/go-kit/sd/events"
)

// 存储实例事件监听器，以及进行事件广播
type registry map[chan<- events.Event]struct{}

func (r registry) broadcast(event events.Event) {
	log.Println("registry.broadcast", event)
	for c := range r {
		eventCopy := copyEvent(event)
		c <- eventCopy
	}
	log.Println("---------registry.broadcast end-----------")
}

func (r registry) register(c chan<- events.Event) {
	log.Println("registry.register")
	r[c] = struct{}{}
}

func (r registry) deregister(c chan<- events.Event) {
	log.Println("registry.deregister")
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
