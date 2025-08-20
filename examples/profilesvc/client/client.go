// Package client provides a profilesvc client based on a predefined Consul
// service name and relevant tags. Users must only provide the address of a
// Consul server.
package client

import (
	"io"
	"time"

	"github.com/dreamsxin/go-kit/sd/endpointer"
	"github.com/dreamsxin/go-kit/sd/endpointer/balancer"
	"github.com/dreamsxin/go-kit/sd/endpointer/executor"
	consulapi "github.com/hashicorp/consul/api"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/examples/profilesvc"
	"github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd/consul"
)

// New returns a service that's load-balanced over instances of profilesvc found
// in the provided Consul server. The mechanism of looking up profilesvc
// instances in Consul is hard-coded into the client.
func New(consulAddr string, logger *log.Logger) (profilesvc.Service, error) {
	apiclient, err := consulapi.NewClient(&consulapi.Config{
		Address: consulAddr,
	})
	if err != nil {
		return nil, err
	}

	// As the implementer of profilesvc, we declare and enforce these
	// parameters for all of the profilesvc consumers.
	var (
		consulService = "profilesvc"
		consulTags    = []string{"prod"}
		passingOnly   = true
		retryMax      = 3
		retryTimeout  = 500 * time.Millisecond
	)

	var (
		sdclient  = consul.NewClient(apiclient)
		instancer = consul.NewInstancer(sdclient, logger, consulService, passingOnly, consul.TagsInstancerOptions(consulTags))
		endpoints profilesvc.Endpoints
	)
	{
		factory := factoryFor(profilesvc.MakePostProfileEndpoint)
		endpointer := endpointer.NewEndpointer(instancer, factory, logger)
		balancer := balancer.NewRoundRobin(endpointer)
		retry := executor.Retry(retryMax, retryTimeout, balancer)
		endpoints.PostProfileEndpoint = retry
	}
	{
		factory := factoryFor(profilesvc.MakeGetProfileEndpoint)
		endpointer := endpointer.NewEndpointer(instancer, factory, logger)
		balancer := balancer.NewRoundRobin(endpointer)
		retry := executor.Retry(retryMax, retryTimeout, balancer)
		endpoints.GetProfileEndpoint = retry
	}
	{
		factory := factoryFor(profilesvc.MakePutProfileEndpoint)
		endpointer := endpointer.NewEndpointer(instancer, factory, logger)
		balancer := balancer.NewRoundRobin(endpointer)
		retry := executor.Retry(retryMax, retryTimeout, balancer)
		endpoints.PutProfileEndpoint = retry
	}

	return endpoints, nil
}

func factoryFor(makeEndpoint func(profilesvc.Service) endpoint.Endpoint) endpoint.Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		service, err := profilesvc.MakeClientEndpoints(instance)
		if err != nil {
			return nil, nil, err
		}
		return makeEndpoint(service), nil, nil
	}
}
