package basic

import (
	"sync"
	"testing"

	"github.com/dreamsxin/go-kit/sd/consul"
	"github.com/dreamsxin/go-kit/sd/events"

	capi "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

// go test -v -count=1 -run TestInstancer .\instancer_test.go
func TestInstancer(t *testing.T) {
	// Get a new client
	client, err := capi.NewClient(capi.DefaultConfig())
	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewDevelopment()

	ch := make(chan events.Event)
	instrancer := consul.NewInstancer(consul.NewClient(client), logger, "test", true)

	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		for event := range ch {
			logger.Sugar().Debugln("-----------event--------", event)
		}
	}()
	instrancer.Register(ch)
	wait.Wait()
}
