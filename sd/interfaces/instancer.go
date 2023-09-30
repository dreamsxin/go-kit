package interfaces

import (
	"github.com/dreamsxin/go-kit/sd/events"
)

// 服务发现类接口
type Instancer interface {
	Register(chan<- events.Event)
	Deregister(chan<- events.Event)
	Stop()
}
