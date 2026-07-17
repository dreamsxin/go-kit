package interfaces

import (
	"github.com/dreamsxin/go-kit/v2/sd/events"
)

// Instancer is the source of service-discovery events.
// Implementations (e.g. consul.Instancer, instance.Cache) push Event values
// to all registered channels whenever the set of healthy instances changes.
type Instancer interface {
	Register(chan events.Event) events.Event
	Deregister(chan events.Event)
	Stop()
}
