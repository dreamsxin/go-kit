package kit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// DefaultRegistrarName is the diagnostic name of a registration component that
// was not given one.
const DefaultRegistrarName = "registrar"

// RegistrarOption configures a registration component.
type RegistrarOption func(*registrarLifecycle)

// WithRegistrarName names the component in startup and shutdown diagnostics.
// Give each registration its own name when a Host publishes more than one.
func WithRegistrarName(name string) RegistrarOption {
	return func(r *registrarLifecycle) {
		r.name = name
	}
}

// WithRegistrar publishes a service instance for as long as the Host runs.
//
// # Attach it last
//
// Components start in declaration order and stop in reverse, so attaching the
// registration after the server that serves the traffic is what makes the
// sequence correct: the instance appears in discovery only once the listener is
// accepting, and disappears before the listener goes away. Attached first, a
// rollout answers connection refused for as long as it takes the server to bind,
// and drops in-flight calls on the way down.
//
//	host := kit.MustNewHost(
//	    kit.WithLifecycle(api),            // binds the listener
//	    kit.WithRegistrar(registrar),      // then publishes the address
//	)
//
// # Do not deregister before registering
//
// A registration written against go-kit's sd/etcd usually reads
//
//	registrar.Deregister()
//	registrar.Register()
//	defer registrar.Deregister()
//
// The leading Deregister is a workaround for that implementation: without a TTL
// it registers with etcd's Create, which fails when the key already exists, so a
// restart after an unclean exit could not re-register — and its Register only
// logged the failure, leaving the process running and invisible to callers.
//
// It is not needed here, and this option does not do it. An sd.Registrar owns one
// instance key, Register overwrites that key, and the providers hold it on a
// lease that expires when the process dies. Register also returns its error, so a
// registration that fails stops startup rather than producing a service nobody
// can reach.
//
// Carrying the idiom over is worth a second look for another reason: it only
// helps when every instance shares one key, and sharing a key means discovery
// holds a single address per service — starting a second instance evicts the
// first, and load balancing has nothing to balance. If your old key was a bare
// service name, check whether you were ever running more than one.
func WithRegistrar(registrar sd.Registrar, options ...RegistrarOption) HostOption {
	return func(h *Host) error {
		component, err := newRegistrarLifecycle(registrar, options...)
		if err != nil {
			return err
		}
		h.components = append(h.components, component)
		return nil
	}
}

// RegistrarLifecycle adapts an sd.Registrar to Lifecycle, for a Host assembled
// through WithLifecycle so that registration is ordered among other components
// explicitly. WithRegistrar is the same thing with the attachment done for you.
//
// It returns an error rather than panicking on a nil Registrar, so a
// misconfiguration is reported where the Host is built.
func RegistrarLifecycle(registrar sd.Registrar, options ...RegistrarOption) (NamedLifecycle, error) {
	return newRegistrarLifecycle(registrar, options...)
}

func newRegistrarLifecycle(registrar sd.Registrar, options ...RegistrarOption) (*registrarLifecycle, error) {
	if isNilRegistrar(registrar) {
		return nil, errors.New("kit: registrar is nil")
	}
	component := &registrarLifecycle{registrar: registrar, name: DefaultRegistrarName}
	for i, option := range options {
		if option == nil {
			return nil, fmt.Errorf("kit: registrar option %d is nil", i)
		}
		option(component)
	}
	if component.name == "" {
		return nil, errors.New("kit: registrar name is empty")
	}
	return component, nil
}

// registrarLifecycle publishes one instance for the lifetime of the Host.
type registrarLifecycle struct {
	registrar sd.Registrar
	name      string

	mu         sync.Mutex
	registered bool
}

func (r *registrarLifecycle) Name() string { return r.name }

// Start registers the instance. A failure here stops the Host: an unregistered
// instance is running where no caller will find it, which is worse than not
// starting.
func (r *registrarLifecycle) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.registered {
		return nil
	}
	if err := r.registrar.Register(); err != nil {
		return err
	}
	r.registered = true
	return nil
}

// Errors reports no asynchronous failures. Keeping a registration alive —
// renewing a lease, recovering from a lost one — belongs to the sd.Registrar,
// which owns the mechanism; surfacing it here would mean guessing at a policy
// this adapter has no way to carry out.
func (r *registrarLifecycle) Errors() <-chan error { return nil }

// Shutdown deregisters the instance so callers stop being handed an address that
// is about to stop answering.
//
// ctx is unused: sd.Registrar.Deregister takes no context, and its
// implementations are expected to release a lease promptly. A provider that
// could block indefinitely would need the deadline threaded through the sd
// contract, not enforced here.
func (r *registrarLifecycle) Shutdown(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.registered {
		// Start never succeeded, so there is nothing published to withdraw.
		return nil
	}
	r.registered = false
	return r.registrar.Deregister()
}

func isNilRegistrar(registrar sd.Registrar) bool {
	if registrar == nil {
		return true
	}
	value := reflect.ValueOf(registrar)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
