package kit_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/dreamsxin/go-kit/v2/sd"
)

// recordingRegistrar records the registration calls in the shared event log, so
// a test can assert their order against the other components.
type recordingRegistrar struct {
	events       *[]string
	name         string
	registerErr  error
	deregisterEr error
}

func (r *recordingRegistrar) Register() error {
	*r.events = append(*r.events, "register "+r.name)
	return r.registerErr
}

func (r *recordingRegistrar) Deregister() error {
	*r.events = append(*r.events, "deregister "+r.name)
	return r.deregisterEr
}

// The whole point of the ordering: publish the address only after the listener
// is accepting, and withdraw it before the listener goes away.
func TestRegistrarPublishesAfterTheServerAndWithdrawsBeforeIt(t *testing.T) {
	var events []string
	server := &recordingLifecycle{name: "api", events: &events}
	registrar := &recordingRegistrar{events: &events, name: "api"}

	host := kit.MustNewHost(
		kit.WithLifecycle(server),
		kit.WithRegistrar(registrar),
	)
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	want := []string{"start api", "register api", "deregister api", "stop api"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

// A registered-but-unreachable instance is worse than one that did not start, so
// a failed registration has to stop the Host.
func TestRegistrarStartFailureStopsTheHost(t *testing.T) {
	var events []string
	registerErr := errors.New("etcd unavailable")
	registrar := &recordingRegistrar{events: &events, name: "api", registerErr: registerErr}

	host := kit.MustNewHost(kit.WithRegistrar(registrar))
	if err := host.Start(); !errors.Is(err, registerErr) {
		t.Fatalf("Start error = %v, want the registration failure", err)
	}
}

// Nothing was published, so there is nothing to withdraw. Deregistering anyway
// would send a spurious delete for a key this process never owned.
func TestRegistrarShutdownIsANoOpWhenRegistrationFailed(t *testing.T) {
	var events []string
	registrar := &recordingRegistrar{
		events:      &events,
		name:        "api",
		registerErr: errors.New("etcd unavailable"),
	}

	host := kit.MustNewHost(kit.WithRegistrar(registrar), kit.WithShutdownTimeout(time.Second))
	if err := host.Start(); err == nil {
		t.Fatal("Start should have failed")
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	for _, event := range events {
		if strings.HasPrefix(event, "deregister") {
			t.Fatalf("deregistered without a successful registration: %v", events)
		}
	}
}

func TestRegistrarDeregisterFailureIsReported(t *testing.T) {
	var events []string
	deregisterErr := errors.New("lease already revoked")
	registrar := &recordingRegistrar{events: &events, name: "api", deregisterEr: deregisterErr}

	host := kit.MustNewHost(kit.WithRegistrar(registrar))
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := host.Shutdown(context.Background()); !errors.Is(err, deregisterErr) {
		t.Fatalf("Shutdown error = %v, want the deregistration failure", err)
	}
}

func TestRegistrarNameAppearsInDiagnostics(t *testing.T) {
	var events []string
	registrar := &recordingRegistrar{events: &events, name: "api", deregisterEr: errors.New("boom")}

	host := kit.MustNewHost(kit.WithRegistrar(registrar, kit.WithRegistrarName("usercenter")))
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	err := host.Shutdown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "usercenter") {
		t.Fatalf("Shutdown error = %v, want it to name the component", err)
	}
}

func TestWithRegistrarRejectsNil(t *testing.T) {
	t.Run("interface nil", func(t *testing.T) {
		if _, err := kit.NewHost(kit.WithRegistrar(nil)); err == nil {
			t.Fatal("expected a nil registrar error")
		}
	})
	t.Run("typed nil", func(t *testing.T) {
		var registrar *recordingRegistrar
		if _, err := kit.NewHost(kit.WithRegistrar(registrar)); err == nil {
			t.Fatal("expected a typed nil registrar error")
		}
	})
	t.Run("nil option", func(t *testing.T) {
		var events []string
		registrar := &recordingRegistrar{events: &events, name: "api"}
		if _, err := kit.NewHost(kit.WithRegistrar(registrar, nil)); err == nil {
			t.Fatal("expected a nil option error")
		}
	})
}

// RegistrarLifecycle is the seam for a Host assembled entirely through
// WithLifecycle, where the registration's position among the other components is
// stated explicitly.
func TestRegistrarLifecycleComposesThroughWithLifecycle(t *testing.T) {
	var events []string
	server := &recordingLifecycle{name: "api", events: &events}
	registrar := &recordingRegistrar{events: &events, name: "api"}

	registration, err := kit.RegistrarLifecycle(registrar)
	if err != nil {
		t.Fatalf("RegistrarLifecycle: %v", err)
	}
	if got := registration.Name(); got != kit.DefaultRegistrarName {
		t.Fatalf("Name = %q, want %q", got, kit.DefaultRegistrarName)
	}
	if registration.Errors() != nil {
		t.Fatal("registration reports no asynchronous failures")
	}

	host := kit.MustNewHost(kit.WithLifecycle(server, registration))
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	want := []string{"start api", "register api", "deregister api", "stop api"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestRegistrarLifecycleRejectsNil(t *testing.T) {
	if _, err := kit.RegistrarLifecycle(nil); err == nil {
		t.Fatal("expected a nil registrar error")
	}
	if _, err := kit.RegistrarLifecycle(&recordingRegistrar{}, kit.WithRegistrarName("")); err == nil {
		t.Fatal("expected an empty name error")
	}
}

// The adapter must satisfy the sd contract it claims to adapt.
var _ sd.Registrar = (*recordingRegistrar)(nil)
