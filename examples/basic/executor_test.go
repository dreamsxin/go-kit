package basic

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/examples/common"
	"github.com/dreamsxin/go-kit/sd/consul"
	"github.com/dreamsxin/go-kit/sd/endpointer"
	"github.com/dreamsxin/go-kit/sd/endpointer/balancer"
	"github.com/dreamsxin/go-kit/sd/endpointer/executor"

	capi "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

// go test -v -count=1 -run TestExecutorRetry .\executor_test.go
func TestExecutorRetry(t *testing.T) {

	serverName := "test2"
	factory := func(instance string) (endpoint.Endpoint, io.Closer, error) {
		service := common.NewTestServer(instance)
		ep := common.MakeTestHelloEndpoint(service)
		return ep, nil, nil
	}

	logger, _ := zap.NewDevelopment()

	client, err := capi.NewClient(capi.DefaultConfig())
	if err != nil {
		panic(err)
	}
	instrancer := consul.NewInstancer(consul.NewClient(client), logger.Sugar(), serverName, true)

	endpointer := endpointer.NewEndpointer(instrancer, factory, logger.Sugar())

	robin := balancer.NewRoundRobin(endpointer)
	retry := executor.Retry(5, time.Duration(1*time.Second), robin)
	ret, err := retry(context.TODO(), "test")
	if err != nil {
		rettyErr := err.(executor.RetryError)
		logger.Sugar().Debugln("-----------TestExecutorRetry--------", ret, len(rettyErr.RawErrors), rettyErr.Error())
	} else {
		logger.Sugar().Debugln("-----------TestExecutorRetry--------", ret)
	}
}
