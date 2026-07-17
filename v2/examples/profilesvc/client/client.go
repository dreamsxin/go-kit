// Package client provides a profilesvc client backed by Consul service
// discovery and round-robin load balancing.
package client

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	consulapi "github.com/hashicorp/consul/api"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/examples/profilesvc"
	"github.com/dreamsxin/go-kit/v2/log"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/consul"
)

// New returns a profilesvc.Service that is load-balanced over all healthy
// Consul instances tagged "prod". The caller must close the returned closer.
func New(consulAddr string, logger *log.Logger) (profilesvc.Service, io.Closer, error) {
	if logger == nil {
		return nil, nil, fmt.Errorf("profilesvc client: logger is nil")
	}
	apiclient, err := consulapi.NewClient(&consulapi.Config{Address: consulAddr})
	if err != nil {
		return nil, nil, err
	}

	const (
		consulService = "profilesvc"
		passingOnly   = true
	)
	sdclient := consul.NewClient(apiclient)
	instancer := consul.NewInstancer(sdclient, logger, consulService, passingOnly,
		consul.TagsInstancerOptions([]string{"prod"}))
	resources := &clientResources{stopInstancer: instancer.Stop}

	sdOpts := []sd.Option{
		sd.WithTimeout(500 * time.Millisecond),
	}

	newEndpoint := func(factory endpoint.Factory) (endpoint.Endpoint, error) {
		ep, closer, err := sd.NewEndpoint(instancer, factory, logger, sdOpts...)
		if err != nil {
			return nil, err
		}
		resources.closers = append(resources.closers, closer)
		return ep, nil
	}

	post, err := newEndpoint(factoryFor(profilesvc.MakePostProfileEndpoint))
	if err != nil {
		_ = resources.Close()
		return nil, nil, err
	}
	get, err := newEndpoint(factoryFor(profilesvc.MakeGetProfileEndpoint))
	if err != nil {
		_ = resources.Close()
		return nil, nil, err
	}
	put, err := newEndpoint(factoryFor(profilesvc.MakePutProfileEndpoint))
	if err != nil {
		_ = resources.Close()
		return nil, nil, err
	}

	return profilesvc.Endpoints{
		PostProfileEndpoint: post,
		GetProfileEndpoint:  get,
		PutProfileEndpoint:  put,
	}, resources, nil
}

type clientResources struct {
	once          sync.Once
	closers       []io.Closer
	stopInstancer func()
	err           error
}

func (r *clientResources) Close() error {
	r.once.Do(func() {
		errs := make([]error, 0, len(r.closers))
		for i := len(r.closers) - 1; i >= 0; i-- {
			if err := r.closers[i].Close(); err != nil {
				errs = append(errs, err)
			}
		}
		if r.stopInstancer != nil {
			r.stopInstancer()
		}
		r.err = errors.Join(errs...)
	})
	return r.err
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
