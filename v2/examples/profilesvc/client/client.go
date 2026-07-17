// Package client provides a profilesvc client backed by Consul service
// discovery and round-robin load balancing.
package client

import (
	"io"
	"time"

	consulapi "github.com/hashicorp/consul/api"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/examples/profilesvc"
	"github.com/dreamsxin/go-kit/v2/log"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/consul"
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
	sdclient := consul.NewClient(apiclient)
	instancer := consul.NewInstancer(sdclient, logger, consulService, passingOnly,
		consul.TagsInstancerOptions([]string{"prod"}))

	sdOpts := []sd.Option{
		sd.WithTimeout(500 * time.Millisecond),
	}

	return profilesvc.Endpoints{
		PostProfileEndpoint: sd.NewEndpoint(instancer, factoryFor(profilesvc.MakePostProfileEndpoint), logger, sdOpts...),
		GetProfileEndpoint:  sd.NewEndpoint(instancer, factoryFor(profilesvc.MakeGetProfileEndpoint), logger, sdOpts...),
		PutProfileEndpoint:  sd.NewEndpoint(instancer, factoryFor(profilesvc.MakePutProfileEndpoint), logger, sdOpts...),
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
