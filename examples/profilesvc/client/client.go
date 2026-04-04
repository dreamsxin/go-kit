// Package client provides a profilesvc client backed by Consul service
// discovery, round-robin load balancing, and automatic retry.
package client

import (
	"io"
	"time"

	consulapi "github.com/hashicorp/consul/api"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/examples/profilesvc"
	"github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd"
	"github.com/dreamsxin/go-kit/sd/consul"
)

// New returns a profilesvc.Service that is load-balanced over all healthy
// Consul instances tagged "prod".
func New(consulAddr string, logger *log.Logger) (profilesvc.Service, error) {
	apiclient, err := consulapi.NewClient(&consulapi.Config{Address: consulAddr})
	if err != nil {
		return nil, err
	}

	const (
		consulService = "profilesvc"
		passingOnly   = true
	)
	sdclient  := consul.NewClient(apiclient)
	instancer := consul.NewInstancer(sdclient, logger, consulService, passingOnly,
		consul.TagsInstancerOptions([]string{"prod"}))

	sdOpts := []sd.Option{
		sd.WithMaxRetries(3),
		sd.WithTimeout(500 * time.Millisecond),
	}

	return profilesvc.Endpoints{
		PostProfileEndpoint: sd.NewEndpoint(instancer, factoryFor(profilesvc.MakePostProfileEndpoint), logger, sdOpts...),
		GetProfileEndpoint:  sd.NewEndpoint(instancer, factoryFor(profilesvc.MakeGetProfileEndpoint),  logger, sdOpts...),
		PutProfileEndpoint:  sd.NewEndpoint(instancer, factoryFor(profilesvc.MakePutProfileEndpoint),  logger, sdOpts...),
	}, nil
}

func factoryFor(makeEndpoint func(profilesvc.Service) endpoint.Endpoint) endpoint.Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		svc, err := profilesvc.MakeClientEndpoints(instance)
		if err != nil {
			return nil, nil, err
		}
		return makeEndpoint(svc), nil, nil
	}
}
