package basic

import (
	"context"
	"io"
	"testing"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/examples/common"
	"github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd/consul"
	"github.com/dreamsxin/go-kit/sd/endpointer"

	capi "github.com/hashicorp/consul/api"
)

// go test -v -count=1 -run TestFactory .\factory_test.go
func TestFactory(t *testing.T) {

	serverName := "test"
	factory := func(instance string) (endpoint.Endpoint, io.Closer, error) {
		service := common.NewTestServer(instance)
		ep := common.MakeTestHelloEndpoint(service)
		return ep, nil, nil
	}

	logger, _ := log.NewDevelopment()

	client, err := capi.NewClient(capi.DefaultConfig())
	if err != nil {
		panic(err)
	}
	instrancer := consul.NewInstancer(consul.NewClient(client), logger, serverName, true)

	endpointer := endpointer.NewEndpointer(instrancer, factory, logger)
	endpoints, err := endpointer.Endpoints()
	logger.Sugar().Debugln("-----------TestFactory--------", endpoints, err)
	if len(endpoints) > 0 {
		ret, err := endpoints[0](context.Background(), "test")
		logger.Sugar().Debugln("-----------TestFactory--------", ret, err)
	}
}
