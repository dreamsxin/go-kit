package consul

import (
	"errors"
	"sync"
	"testing"
	"time"

	stdconsul "github.com/hashicorp/consul/api"
)

type fakeClient struct {
	mu              sync.Mutex
	calls           int
	firstTag        string
	blockingStarted chan struct{}
	registerErr     error
	deregisterErr   error
}

func (f *fakeClient) Register(*stdconsul.AgentServiceRegistration) error {
	return f.registerErr
}

func (f *fakeClient) Deregister(*stdconsul.AgentServiceRegistration) error {
	return f.deregisterErr
}

func (f *fakeClient) Service(_ string, tag string, _ bool, opts *stdconsul.QueryOptions) ([]*stdconsul.ServiceEntry, *stdconsul.QueryMeta, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	if call == 1 {
		f.firstTag = tag
	}
	f.mu.Unlock()
	if call == 1 {
		return []*stdconsul.ServiceEntry{{
			Node:    &stdconsul.Node{Address: "127.0.0.1"},
			Service: &stdconsul.AgentService{Port: 8080, Tags: []string{"blue", "v2"}},
		}}, &stdconsul.QueryMeta{LastIndex: 1}, nil
	}
	select {
	case f.blockingStarted <- struct{}{}:
	default:
	}
	<-opts.Context().Done()
	return nil, nil, opts.Context().Err()
}

func TestInstancerAppliesOptionsBeforeInitialQueryAndStopsBlockingQuery(t *testing.T) {
	client := &fakeClient{blockingStarted: make(chan struct{}, 1)}
	instancer := NewInstancer(client, nil, "users", true, TagsInstancerOptions([]string{"blue", "v2"}))

	select {
	case <-client.blockingStarted:
	case <-time.After(time.Second):
		t.Fatal("background blocking query did not start")
	}

	client.mu.Lock()
	firstTag := client.firstTag
	client.mu.Unlock()
	if firstTag != "blue" {
		t.Fatalf("first query tag = %q, want blue", firstTag)
	}

	done := make(chan struct{})
	go func() {
		instancer.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel and join the blocking Consul query")
	}
}

func TestRegistrarReturnsClientErrors(t *testing.T) {
	wantRegister := errors.New("register failed")
	wantDeregister := errors.New("deregister failed")
	client := &fakeClient{registerErr: wantRegister, deregisterErr: wantDeregister}
	registrar := NewRegistrar(client, nil, "users", "127.0.0.1", 8080)

	if err := registrar.Register(); !errors.Is(err, wantRegister) {
		t.Fatalf("Register error = %v, want %v", err, wantRegister)
	}
	if err := registrar.Deregister(); !errors.Is(err, wantDeregister) {
		t.Fatalf("Deregister error = %v, want %v", err, wantDeregister)
	}
}
