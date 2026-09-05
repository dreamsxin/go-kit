package etcd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
)

type registerCall struct {
	key      string
	value    string
	ttl      time.Duration
	conflict sd.Conflict
}

// fakeClient stands in for etcd. Every behaviour these tests care about —
// revisions, watch restarts, lost leases — is reachable from here, so the suite
// needs no server.
type fakeClient struct {
	mu sync.Mutex

	entries    map[string]string
	revision   int64
	entriesErr error

	watchErr       error
	watchRevisions []int64
	changes        chan struct{}
	watched        chan struct{}

	registerErr   error
	registers     []registerCall
	lost          chan struct{}
	deregistered  []string
	deregisterErr error
	onDeregister  func(key string)
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		entries:  map[string]string{},
		revision: 7,
		watched:  make(chan struct{}, 16),
	}
}

func (f *fakeClient) Entries(_ context.Context, _ string) (map[string]string, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.entriesErr != nil {
		return nil, 0, f.entriesErr
	}
	entries := make(map[string]string, len(f.entries))
	for key, value := range f.entries {
		entries[key] = value
	}
	return entries, f.revision, nil
}

func (f *fakeClient) Watch(_ context.Context, _ string, revision int64) (<-chan struct{}, error) {
	f.mu.Lock()
	f.watchRevisions = append(f.watchRevisions, revision)
	if f.watchErr != nil {
		err := f.watchErr
		f.mu.Unlock()
		return nil, err
	}
	changes := make(chan struct{}, 1)
	f.changes = changes
	f.mu.Unlock()

	select {
	case f.watched <- struct{}{}:
	default:
	}
	return changes, nil
}

func (f *fakeClient) Register(_ context.Context, key, value string, ttl time.Duration, conflict sd.Conflict) (<-chan struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registers = append(f.registers, registerCall{key: key, value: value, ttl: ttl, conflict: conflict})
	if f.registerErr != nil {
		return nil, f.registerErr
	}
	f.lost = make(chan struct{})
	return f.lost, nil
}

func (f *fakeClient) Deregister(_ context.Context, key string) error {
	f.mu.Lock()
	hook := f.onDeregister
	f.mu.Unlock()
	// The hook runs outside the lock so a test can attempt a concurrent
	// Register while the removal is in flight.
	if hook != nil {
		hook(key)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.deregistered = append(f.deregistered, key)
	return f.deregisterErr
}

func (f *fakeClient) set(entries map[string]string) {
	f.mu.Lock()
	f.entries = entries
	f.revision++
	f.mu.Unlock()
}

func (f *fakeClient) signal(t *testing.T) {
	t.Helper()
	select {
	case <-f.watched:
	case <-time.After(time.Second):
		t.Fatal("watch was never established")
	}

	f.mu.Lock()
	changes := f.changes
	f.mu.Unlock()
	if changes == nil {
		t.Fatal("no watch channel")
	}
	changes <- struct{}{}
}

func (f *fakeClient) registerCalls() []registerCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]registerCall(nil), f.registers...)
}

func (f *fakeClient) loseLease(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		f.mu.Lock()
		lost := f.lost
		f.mu.Unlock()
		if lost != nil {
			close(lost)
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("registration was never established")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestNewInstancerLiftsAddressesAndMetadata(t *testing.T) {
	client := newFakeClient()
	client.set(map[string]string{
		"/services/users/b": `{"address":"10.0.0.2:8080","metadata":{"zone":"eu","weight":"5"}}`,
		"/services/users/a": "10.0.0.1:8080",
	})

	instancer := NewInstancer(client, nil, "users")
	defer instancer.Stop()

	event := instancer.Register(nil)
	if event.Err != nil {
		t.Fatalf("initial event error: %v", event.Err)
	}
	if len(event.Instances) != 2 {
		t.Fatalf("instances = %d, want 2", len(event.Instances))
	}
	if got := event.Instances[0].Address; got != "10.0.0.1:8080" {
		t.Fatalf("first address = %q, want the bare-address registration", got)
	}
	if event.Instances[0].Metadata != nil {
		t.Fatalf("bare registration metadata = %v, want nil", event.Instances[0].Metadata)
	}
	if got := event.Instances[1].Metadata["zone"]; got != "eu" {
		t.Fatalf("zone = %v, want eu", got)
	}
	if got := event.Instances[1].Metadata["weight"]; got != "5" {
		t.Fatalf("weight = %v, want 5", got)
	}
}

func TestNewInstancerWatchesAfterTheRevisionItRead(t *testing.T) {
	client := newFakeClient()
	instancer := NewInstancer(client, nil, "users")
	defer instancer.Stop()

	select {
	case <-client.watched:
	case <-time.After(time.Second):
		t.Fatal("watch was never established")
	}

	client.mu.Lock()
	revisions := append([]int64(nil), client.watchRevisions...)
	client.mu.Unlock()

	if len(revisions) == 0 {
		t.Fatal("no watch call recorded")
	}
	// A change between the read and the watch must not be missed.
	if revisions[0] != 8 {
		t.Fatalf("watch revision = %d, want 8 (read revision 7 + 1)", revisions[0])
	}
}

func TestInstancerBroadcastsSetChanges(t *testing.T) {
	client := newFakeClient()
	client.set(map[string]string{"/services/users/a": "10.0.0.1:8080"})

	instancer := NewInstancer(client, nil, "users")
	defer instancer.Stop()

	events := make(chan Event, 1)
	instancer.Register(events)

	client.set(map[string]string{
		"/services/users/a": "10.0.0.1:8080",
		"/services/users/b": `{"address":"10.0.0.2:8080","metadata":{"zone":"eu"}}`,
	})
	client.signal(t)

	select {
	case event := <-events:
		if len(event.Instances) != 2 {
			t.Fatalf("instances = %d, want 2", len(event.Instances))
		}
	case <-time.After(time.Second):
		t.Fatal("no update was broadcast")
	}
}

func TestInstancerSkipsMalformedRegistrations(t *testing.T) {
	client := newFakeClient()
	client.set(map[string]string{
		"/services/users/a":      "10.0.0.1:8080",
		"/services/users/broken": "{not json",
	})

	instancer := NewInstancer(client, nil, "users")
	defer instancer.Stop()

	event := instancer.Register(nil)
	if event.Err != nil {
		t.Fatalf("one bad key must not fail the snapshot, got %v", event.Err)
	}
	if len(event.Instances) != 1 {
		t.Fatalf("instances = %d, want 1", len(event.Instances))
	}
}

func TestNewInstancerReportsReadErrors(t *testing.T) {
	client := newFakeClient()
	client.entriesErr = errors.New("etcd is down")

	instancer := NewInstancer(client, nil, "users")
	defer instancer.Stop()

	if event := instancer.Register(nil); event.Err == nil {
		t.Fatal("initial read error was swallowed")
	}
}

func TestInstancerStopDoesNotWaitForTheWatchChannel(t *testing.T) {
	// fakeClient never closes the channel it hands out, which is what a watch
	// wedged in a slow gRPC teardown looks like. Stop must still return.
	client := newFakeClient()
	instancer := NewInstancer(client, nil, "users")

	select {
	case <-client.watched:
	case <-time.After(time.Second):
		t.Fatal("watch was never established")
	}

	stopped := make(chan struct{})
	go func() {
		instancer.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop blocked on the watch channel")
	}
}

func TestRegistrarWritesLeasedRegistration(t *testing.T) {
	client := newFakeClient()
	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080,
		MetaRegistrarOptions(map[string]string{"zone": "eu"}),
		MetaRegistrarOptions(map[string]string{"weight": "5"}),
		TTLRegistrarOptions(3*time.Second),
	)

	if err := registrar.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer registrar.Deregister()

	calls := client.registerCalls()
	if len(calls) != 1 {
		t.Fatalf("register calls = %d, want 1", len(calls))
	}
	if calls[0].key != "/services/users/10.0.0.1:8080" {
		t.Fatalf("key = %q", calls[0].key)
	}
	if calls[0].ttl != 3*time.Second {
		t.Fatalf("ttl = %v, want 3s", calls[0].ttl)
	}

	instance, err := decodeRegistration(calls[0].value)
	if err != nil {
		t.Fatalf("the registrar wrote a value it cannot read back: %v", err)
	}
	if instance.Address != "10.0.0.1:8080" {
		t.Fatalf("address = %q", instance.Address)
	}
	// Repeated MetaRegistrarOptions calls merge rather than replace.
	if instance.Metadata["zone"] != "eu" || instance.Metadata["weight"] != "5" {
		t.Fatalf("metadata = %v, want zone and weight", instance.Metadata)
	}
}

func TestRegistrarRegistersAgainWhenTheLeaseIsLost(t *testing.T) {
	client := newFakeClient()
	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080)
	registrar.retryBase = time.Millisecond

	if err := registrar.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer registrar.Deregister()

	client.loseLease(t)

	waitUntil(t, time.Second, func() bool { return len(client.registerCalls()) >= 2 })

	calls := client.registerCalls()
	if calls[1].key != calls[0].key || calls[1].value != calls[0].value {
		t.Fatalf("re-registration differs from the original: %+v vs %+v", calls[1], calls[0])
	}
}

func TestRegistrarCarriesConflictSemanticsIntoEveryWrite(t *testing.T) {
	client := newFakeClient()
	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080,
		ConflictRegistrarOptions(sd.ConflictCompareAndSwap),
	)
	registrar.retryBase = time.Millisecond

	if err := registrar.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer registrar.Deregister()

	// The recovery path has to ask for the same semantics as the first write.
	// A renewal that silently fell back to overwrite would let an instance take
	// back a key another process had claimed in the meantime.
	client.loseLease(t)
	waitUntil(t, time.Second, func() bool { return len(client.registerCalls()) >= 2 })

	for i, call := range client.registerCalls() {
		if call.conflict != sd.ConflictCompareAndSwap {
			t.Fatalf("register call %d used %v, want %v", i, call.conflict, sd.ConflictCompareAndSwap)
		}
	}
}

func TestRegistrarStopsRegisteringAgainWhenTheKeyIsTaken(t *testing.T) {
	client := newFakeClient()
	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080,
		ConflictRegistrarOptions(sd.ConflictCreateOnly),
	)
	registrar.retryBase = time.Millisecond

	if err := registrar.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer registrar.Deregister()

	// Somebody else owns the key now, so every retry is refused.
	client.mu.Lock()
	client.registerErr = fmt.Errorf("register %s: %w", "/services/users/10.0.0.1:8080", sd.ErrConflict)
	client.mu.Unlock()

	client.loseLease(t)
	waitUntil(t, time.Second, func() bool { return len(client.registerCalls()) >= 2 })

	// A conflict is permanent: retrying writes the same identity and is refused
	// again, so the supervisor must stop instead of spinning for the life of the
	// process. With a 1ms base delay a running retry loop would be well past
	// two attempts by now.
	time.Sleep(50 * time.Millisecond)
	if calls := client.registerCalls(); len(calls) != 2 {
		t.Fatalf("register calls = %d, want 2: the supervisor kept retrying a refused registration", len(calls))
	}
}

func TestRegistrarReportsAConflictFromTheFirstRegistration(t *testing.T) {
	client := newFakeClient()
	client.mu.Lock()
	client.registerErr = fmt.Errorf("register %s: %w", "/services/users/users-1", sd.ErrConflict)
	client.mu.Unlock()

	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080,
		IDRegistrarOptions("users-1"),
		ConflictRegistrarOptions(sd.ConflictCreateOnly),
	)

	err := registrar.Register()
	if !errors.Is(err, sd.ErrConflict) {
		t.Fatalf("Register error = %v, want one wrapping sd.ErrConflict", err)
	}
}

func TestRegistrarDeregisterStopsRenewalAndRemovesTheKey(t *testing.T) {
	client := newFakeClient()
	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080, IDRegistrarOptions("users-1"))

	if err := registrar.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := registrar.Deregister(); err != nil {
		t.Fatalf("Deregister: %v", err)
	}

	client.mu.Lock()
	deregistered := append([]string(nil), client.deregistered...)
	client.mu.Unlock()

	if len(deregistered) != 1 || deregistered[0] != "/services/users/users-1" {
		t.Fatalf("deregistered = %v", deregistered)
	}

	// A second Deregister has nothing left to do.
	if err := registrar.Deregister(); err != nil {
		t.Fatalf("second Deregister: %v", err)
	}
	client.mu.Lock()
	total := len(client.deregistered)
	client.mu.Unlock()
	if total != 1 {
		t.Fatalf("deregister calls = %d, want 1", total)
	}
}

func TestRegistrarDeregisterRetriesAfterAFailedRemoval(t *testing.T) {
	client := newFakeClient()
	client.mu.Lock()
	client.deregisterErr = errors.New("etcd unreachable")
	client.mu.Unlock()

	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080, IDRegistrarOptions("users-1"))
	if err := registrar.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := registrar.Deregister(); err == nil {
		t.Fatal("Deregister did not report the removal failure")
	}

	// The key is still in etcd, so the registration is still pending: a retry
	// has to attempt the removal again instead of reporting a clean stop.
	client.mu.Lock()
	client.deregisterErr = nil
	attempts := len(client.deregistered)
	client.mu.Unlock()
	if attempts != 1 {
		t.Fatalf("removal attempts = %d, want 1", attempts)
	}

	if err := registrar.Deregister(); err != nil {
		t.Fatalf("retried Deregister: %v", err)
	}
	client.mu.Lock()
	deregistered := append([]string(nil), client.deregistered...)
	client.mu.Unlock()
	if len(deregistered) != 2 || deregistered[1] != "/services/users/users-1" {
		t.Fatalf("deregistered = %v, want the retry to remove the key again", deregistered)
	}

	// And once it succeeded, it stays done.
	if err := registrar.Deregister(); err != nil {
		t.Fatalf("third Deregister: %v", err)
	}
	client.mu.Lock()
	total := len(client.deregistered)
	client.mu.Unlock()
	if total != 2 {
		t.Fatalf("removal attempts after success = %d, want 2", total)
	}
}

func TestRegistrarRegisterIsIdempotent(t *testing.T) {
	client := newFakeClient()
	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080)

	if err := registrar.Register(); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := registrar.Register(); err != nil {
		t.Fatalf("second Register: %v", err)
	}
	defer registrar.Deregister()

	if calls := client.registerCalls(); len(calls) != 1 {
		t.Fatalf("register calls = %d, want 1: one Registrar owns one key", len(calls))
	}
}

func TestRegistrarReportsRegisterErrors(t *testing.T) {
	client := newFakeClient()
	client.registerErr = errors.New("no cluster")
	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080)

	if err := registrar.Register(); err == nil {
		t.Fatal("Register error was swallowed")
	}
	// A failed Register must leave nothing to tear down.
	if err := registrar.Deregister(); err != nil {
		t.Fatalf("Deregister after a failed Register: %v", err)
	}
}

// A Register must not slip between Deregister deciding to remove the key and
// the removal reaching etcd. If it does, the delete lands on the key the new
// Register just wrote: the instance disappears from discovery while the
// Registrar still reports itself as registered, and because the new lease is
// never revoked nothing ever notices.
func TestRegistrarRegisterWaitsForAnInFlightDeregister(t *testing.T) {
	client := newFakeClient()
	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080)

	removing := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	client.onDeregister = func(string) {
		once.Do(func() {
			close(removing)
			<-release
		})
	}

	if err := registrar.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}

	removed := make(chan error, 1)
	go func() { removed <- registrar.Deregister() }()
	<-removing

	registered := make(chan error, 1)
	go func() { registered <- registrar.Register() }()

	select {
	case err := <-registered:
		t.Fatalf("Register returned while the removal was in flight (err = %v)", err)
	case <-time.After(50 * time.Millisecond):
	}
	if calls := client.registerCalls(); len(calls) != 1 {
		t.Fatalf("register calls during the removal = %d, want 1: nothing new may reach etcd yet", len(calls))
	}

	close(release)
	if err := <-removed; err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if err := <-registered; err != nil {
		t.Fatalf("Register after the removal: %v", err)
	}
	defer registrar.Deregister()

	// The registration was rebuilt after the delete, so the key is live again.
	if calls := client.registerCalls(); len(calls) != 2 {
		t.Fatalf("register calls = %d, want 2", len(calls))
	}
	client.mu.Lock()
	removals := len(client.deregistered)
	client.mu.Unlock()
	if removals != 1 {
		t.Fatalf("removals = %d, want 1", removals)
	}
}

func TestRegistrarAndInstancerAgreeOnTheNamespace(t *testing.T) {
	client := newFakeClient()
	registrar := NewRegistrar(client, nil, "users", "10.0.0.1", 8080, NamespaceRegistrarOptions("/kit"))
	if err := registrar.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer registrar.Deregister()

	calls := client.registerCalls()
	client.set(map[string]string{calls[0].key: calls[0].value})

	instancer := NewInstancer(client, nil, "users", NamespaceInstancerOptions("/kit"))
	defer instancer.Stop()

	event := instancer.Register(nil)
	if len(event.Instances) != 1 || event.Instances[0].Address != "10.0.0.1:8080" {
		t.Fatalf("instances = %+v", event.Instances)
	}
}

func TestServicePrefixDoesNotMatchNeighbouringServices(t *testing.T) {
	prefix := servicePrefix("", "users")
	if prefix != "/services/users/" {
		t.Fatalf("prefix = %q", prefix)
	}
	// Without the trailing separator this prefix would also match
	// /services/users-admin/...
	if len(prefix) == 0 || prefix[len(prefix)-1] != '/' {
		t.Fatal("prefix must end with a separator")
	}
	if got := servicePrefix("/kit/", "/users/"); got != "/kit/users/" {
		t.Fatalf("prefix = %q, want /kit/users/", got)
	}
}

func TestDecodeRegistration(t *testing.T) {
	if _, err := decodeRegistration("  "); err == nil {
		t.Fatal("empty value must fail")
	}
	if _, err := decodeRegistration(`{"metadata":{"zone":"eu"}}`); err == nil {
		t.Fatal("a registration without an address must fail")
	}
	instance, err := decodeRegistration("  10.0.0.1:8080  ")
	if err != nil {
		t.Fatalf("bare address: %v", err)
	}
	if instance.Address != "10.0.0.1:8080" {
		t.Fatalf("address = %q", instance.Address)
	}
}

func waitUntil(t *testing.T, timeout time.Duration, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met in time")
}
