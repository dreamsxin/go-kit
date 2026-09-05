package consul

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	stdconsul "github.com/hashicorp/consul/api"
)

func addresses(addrs ...string) []Instance {
	instances := make([]Instance, len(addrs))
	for i, addr := range addrs {
		instances[i] = Instance{Address: addr}
	}
	return instances
}

type fakeClient struct {
	mu              sync.Mutex
	calls           int
	firstTag        string
	entryMeta       map[string]string
	registered      *stdconsul.AgentServiceRegistration
	blockingStarted chan struct{}
	registerErr     error
	deregisterErr   error
}

func (f *fakeClient) Register(registration *stdconsul.AgentServiceRegistration) error {
	f.mu.Lock()
	f.registered = registration
	f.mu.Unlock()
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
			Service: &stdconsul.AgentService{Port: 8080, Tags: []string{"blue", "v2"}, Meta: f.entryMeta},
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

func TestRegistrarReportsMetaLabels(t *testing.T) {
	client := &fakeClient{}
	registrar := NewRegistrar(client, nil, "users", "127.0.0.1", 8080,
		MetaRegistrarOptions(map[string]string{"zone": "z1", "weight": "5"}),
		MetaRegistrarOptions(map[string]string{"version": "v2"}),
	)

	if err := registrar.Register(); err != nil {
		t.Fatalf("Register error = %v", err)
	}

	client.mu.Lock()
	registered := client.registered
	client.mu.Unlock()
	if registered == nil {
		t.Fatal("Register did not reach the client")
	}
	want := map[string]string{"zone": "z1", "weight": "5", "version": "v2"}
	if !reflect.DeepEqual(registered.Meta, want) {
		t.Fatalf("registration Meta = %v, want %v (options must merge, not replace)", registered.Meta, want)
	}
}

func TestInstancerSurfacesServiceMetaAsInstanceMetadata(t *testing.T) {
	client := &fakeClient{
		blockingStarted: make(chan struct{}, 1),
		entryMeta:       map[string]string{"zone": "z1", "weight": "5"},
	}
	instancer := NewInstancer(client, nil, "users", true)
	defer instancer.Stop()

	state := instancer.Register(nil)
	if state.Err != nil {
		t.Fatalf("initial state error = %v", state.Err)
	}
	want := []Instance{{
		Address:  "127.0.0.1:8080",
		Metadata: map[string]any{"zone": "z1", "weight": "5"},
	}}
	if !reflect.DeepEqual(state.Instances, want) {
		t.Fatalf("instances = %v, want %v", state.Instances, want)
	}
}
