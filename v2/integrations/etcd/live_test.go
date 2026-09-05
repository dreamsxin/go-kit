package etcd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// These tests run against a real etcd. They are skipped unless
// GOKIT_ETCD_ENDPOINTS names one, so `go test ./...` stays self-contained:
//
//	etcd --data-dir <tmp>
//	GOKIT_ETCD_ENDPOINTS=127.0.0.1:2379 go test ./integrations/etcd/ -run Live
//
// They exist because a fake cannot answer the questions this package actually
// bets on. The fake decides when a key disappears; etcd decides it from lease
// TTLs, keepalive streams, and revocation, and those are the mechanisms
// Registrar's whole design rests on. Everything else — lock ordering, timeout
// budgets, error paths — stays in etcd_test.go, where it is deterministic.
const liveEndpointsEnv = "GOKIT_ETCD_ENDPOINTS"

// liveEtcd dials the configured cluster and returns a client plus a namespace
// unique to this test, so a shared cluster and parallel runs cannot collide. The
// namespace is deleted on cleanup whether the test passed or not.
func liveEtcd(t *testing.T) (*clientv3.Client, string) {
	t.Helper()

	endpoints := strings.TrimSpace(os.Getenv(liveEndpointsEnv))
	if endpoints == "" {
		t.Skipf("set %s to run the live etcd tests (e.g. 127.0.0.1:2379)", liveEndpointsEnv)
	}

	etcd, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(endpoints, ","),
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial etcd at %s: %v", endpoints, err)
	}

	// Fail loudly here rather than inside the first assertion: a cluster that is
	// not reachable is a setup problem, not a test failure.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = etcd.Get(ctx, "/")
	cancel()
	if err != nil {
		_ = etcd.Close()
		t.Fatalf("etcd at %s is not answering: %v", endpoints, err)
	}

	namespace := fmt.Sprintf("/gokit-live/%s-%d", sanitize(t.Name()), time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = etcd.Delete(ctx, namespace, clientv3.WithPrefix())
		cancel()
		_ = etcd.Close()
	})
	return etcd, namespace
}

func sanitize(name string) string {
	return strings.NewReplacer("/", "-", " ", "-").Replace(name)
}

// recordedLease reads the lease the Client wrapper remembers for key. It lives at
// file scope because the tests shadow the *client type with a local named client.
func recordedLease(c Client, key string) (clientv3.LeaseID, bool) {
	concrete, ok := c.(*client)
	if !ok {
		return 0, false
	}
	concrete.mu.Lock()
	defer concrete.mu.Unlock()
	lease, leased := concrete.leases[key]
	return lease, leased
}

// keysUnder reports the instance keys currently stored under the service prefix.
func keysUnder(t *testing.T, etcd *clientv3.Client, prefix string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := etcd.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		t.Fatalf("get %s: %v", prefix, err)
	}
	keys := make([]string, 0, len(response.Kvs))
	for _, kv := range response.Kvs {
		keys = append(keys, string(kv.Key))
	}
	return keys
}

// waitForKeys polls until the prefix holds want keys, or fails. Polling is the
// honest tool here: etcd applies a lease expiry on its own schedule, so the test
// has no event to wait on.
func waitForKeys(t *testing.T, etcd *clientv3.Client, prefix string, want int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	var last []string
	for time.Now().Before(deadline) {
		last = keysUnder(t, etcd, prefix)
		if len(last) == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("prefix %s holds %d keys after %v, want %d: %v", prefix, len(last), within, want, last)
}

// waitForAddresses polls an Instancer's published snapshot. It reads through
// Register rather than a subscription channel so a snapshot that arrived before
// the test looked still counts.
func waitForAddresses(t *testing.T, instancer *Instancer, want int, within time.Duration) []Instance {
	t.Helper()
	deadline := time.Now().Add(within)
	var last Event
	for time.Now().Before(deadline) {
		last = instancer.Register(nil)
		if last.Err == nil && len(last.Instances) == want {
			return last.Instances
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("instancer published %d instances (err %v) after %v, want %d",
		len(last.Instances), last.Err, within, want)
	return nil
}

// The end-to-end contract: a registration becomes discoverable through a watch,
// with its labels, and deregistering removes it again.
func TestLiveRegisterIsDiscoveredAndRemoved(t *testing.T) {
	etcd, namespace := liveEtcd(t)
	client := NewClient(etcd)
	prefix := servicePrefix(namespace, "users")

	instancer := NewInstancer(client, nil, "users", NamespaceInstancerOptions(namespace))
	defer instancer.Stop()

	registrar := NewRegistrar(client, nil, "users", "127.0.0.1", 8080,
		NamespaceRegistrarOptions(namespace),
		TTLRegistrarOptions(5*time.Second),
		MetaRegistrarOptions(map[string]string{"zone": "a"}),
	)
	if err := registrar.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}

	instances := waitForAddresses(t, instancer, 1, 10*time.Second)
	if instances[0].Address != "127.0.0.1:8080" {
		t.Errorf("address = %q, want 127.0.0.1:8080", instances[0].Address)
	}
	if zone := instances[0].Metadata["zone"]; zone != "a" {
		t.Errorf("zone label = %v, want a", zone)
	}

	if err := registrar.Deregister(); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	waitForAddresses(t, instancer, 0, 10*time.Second)
	if keys := keysUnder(t, etcd, prefix); len(keys) != 0 {
		t.Errorf("keys after Deregister = %v, want none", keys)
	}
}

// Keepalive is the reason Registrar exists. With a TTL of 2s, a key that is
// still there after 6s proves the renewal stream is running — and that the TTL
// is short enough for the next test to mean anything.
func TestLiveKeepAliveOutlivesTheTTL(t *testing.T) {
	etcd, namespace := liveEtcd(t)
	client := NewClient(etcd)
	prefix := servicePrefix(namespace, "users")

	registrar := NewRegistrar(client, nil, "users", "127.0.0.1", 8081,
		NamespaceRegistrarOptions(namespace),
		TTLRegistrarOptions(2*time.Second),
	)
	if err := registrar.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer func() { _ = registrar.Deregister() }()

	waitForKeys(t, etcd, prefix, 1, 5*time.Second)
	time.Sleep(6 * time.Second)
	if keys := keysUnder(t, etcd, prefix); len(keys) != 1 {
		t.Fatalf("keys after 3 TTLs = %v, want the registration to still be there", keys)
	}
}

// The other half of the deal: when renewal stops without a clean deregister —
// the process was killed — etcd must drop the key on its own. Cancelling the
// context passed to Client.Register is what a dead process looks like from
// etcd's side.
func TestLiveLeaseExpiresWhenRenewalStops(t *testing.T) {
	etcd, namespace := liveEtcd(t)
	client := NewClient(etcd)
	prefix := servicePrefix(namespace, "users")
	key := prefix + "127.0.0.1:8082"

	ctx, cancel := context.WithCancel(context.Background())
	value, err := encodeRegistration("127.0.0.1:8082", nil)
	if err != nil {
		t.Fatalf("encodeRegistration: %v", err)
	}
	lost, err := client.Register(ctx, key, value, 2*time.Second, sd.ConflictOverwrite)
	if err != nil {
		cancel()
		t.Fatalf("Register: %v", err)
	}
	waitForKeys(t, etcd, prefix, 1, 5*time.Second)

	cancel()
	select {
	case <-lost:
	case <-time.After(5 * time.Second):
		t.Fatal("lost channel never closed after the keepalive context ended")
	}

	// Nothing renews the lease now, so etcd expires it. The window is the TTL
	// plus however long the cluster takes to notice.
	waitForKeys(t, etcd, prefix, 0, 15*time.Second)
}

// Deregister revokes the lease rather than leaving it to expire. A revoked lease
// is unknown to etcd, so asking for its TTL fails — that is the observable
// difference between revoked and merely detached.
func TestLiveDeregisterRevokesTheLease(t *testing.T) {
	etcd, namespace := liveEtcd(t)
	client := NewClient(etcd)
	prefix := servicePrefix(namespace, "users")
	key := prefix + "127.0.0.1:8083"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	value, err := encodeRegistration("127.0.0.1:8083", nil)
	if err != nil {
		t.Fatalf("encodeRegistration: %v", err)
	}
	if _, err := client.Register(ctx, key, value, 30*time.Second, sd.ConflictOverwrite); err != nil {
		t.Fatalf("Register: %v", err)
	}
	waitForKeys(t, etcd, prefix, 1, 5*time.Second)

	// Read the lease the client recorded, so the assertion is about the lease
	// Deregister actually acted on.
	lease, leased := recordedLease(client, key)
	if !leased {
		t.Fatal("client did not record a lease for the registration")
	}

	if err := client.Deregister(context.Background(), key); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if keys := keysUnder(t, etcd, prefix); len(keys) != 0 {
		t.Errorf("keys after Deregister = %v, want none", keys)
	}

	ttlCtx, ttlCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ttlCancel()
	response, err := etcd.TimeToLive(ttlCtx, lease)
	if err != nil {
		t.Fatalf("TimeToLive: %v", err)
	}
	if response.TTL != -1 {
		t.Errorf("lease TTL = %d, want -1 (revoked); Deregister left it alive", response.TTL)
	}

	if _, stillRecorded := recordedLease(client, key); stillRecorded {
		t.Error("client still records a lease for a key it successfully deregistered")
	}
}

// Register and Deregister racing must not leave the Registrar reporting a
// registration that is not in etcd. Against a real cluster the round trips are
// long enough for the interleaving to be reachable; the last operation is a
// Register, so the key must exist when the dust settles.
func TestLiveConcurrentRegisterDeregisterEndsRegistered(t *testing.T) {
	etcd, namespace := liveEtcd(t)
	client := NewClient(etcd)
	prefix := servicePrefix(namespace, "users")

	registrar := NewRegistrar(client, nil, "users", "127.0.0.1", 8084,
		NamespaceRegistrarOptions(namespace),
		TTLRegistrarOptions(10*time.Second),
	)
	defer func() { _ = registrar.Deregister() }()

	for round := 0; round < 5; round++ {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := registrar.Register(); err != nil {
				t.Errorf("round %d Register: %v", round, err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := registrar.Deregister(); err != nil {
				t.Errorf("round %d Deregister: %v", round, err)
			}
		}()
		wg.Wait()
	}

	// Whatever order the rounds took, one final Register must leave exactly one
	// live key: the Registrar's view and etcd's must agree.
	if err := registrar.Register(); err != nil {
		t.Fatalf("final Register: %v", err)
	}
	waitForKeys(t, etcd, prefix, 1, 10*time.Second)
}

// Conflict semantics are decided by etcd's transaction, so only etcd can say
// whether they hold. A second Client stands in for a second process: it has no
// memory of the key, which is exactly the position a restarted or duplicated
// instance is in.
func TestLiveCreateOnlyRefusesAKeyThatExists(t *testing.T) {
	etcd, namespace := liveEtcd(t)
	prefix := servicePrefix(namespace, "users")
	key := prefix + "127.0.0.1:8085"

	value, err := encodeRegistration("127.0.0.1:8085", nil)
	if err != nil {
		t.Fatalf("encodeRegistration: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	owner := NewClient(etcd)
	if _, err := owner.Register(ctx, key, value, 30*time.Second, sd.ConflictCreateOnly); err != nil {
		t.Fatalf("first create-only Register: %v", err)
	}
	waitForKeys(t, etcd, prefix, 1, 5*time.Second)

	other := NewClient(etcd)
	if _, err := other.Register(ctx, key, value, 30*time.Second, sd.ConflictCreateOnly); !errors.Is(err, sd.ErrConflict) {
		t.Fatalf("second create-only Register error = %v, want sd.ErrConflict", err)
	}
	// Overwrite is the semantics that ignores the holder, and it still does.
	if _, err := other.Register(ctx, key, value, 30*time.Second, sd.ConflictOverwrite); err != nil {
		t.Fatalf("overwrite Register: %v", err)
	}
}

// Compare-and-swap has to allow the one case a registrar depends on — rewriting
// its own key after a lost lease — while refusing the one it exists for.
func TestLiveCompareAndSwapKeepsItsOwnKeyAndLosesATakenOne(t *testing.T) {
	etcd, namespace := liveEtcd(t)
	prefix := servicePrefix(namespace, "users")
	key := prefix + "127.0.0.1:8086"

	value, err := encodeRegistration("127.0.0.1:8086", nil)
	if err != nil {
		t.Fatalf("encodeRegistration: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	owner := NewClient(etcd)
	if _, err := owner.Register(ctx, key, value, 30*time.Second, sd.ConflictCompareAndSwap); err != nil {
		t.Fatalf("first compare-and-swap Register: %v", err)
	}
	waitForKeys(t, etcd, prefix, 1, 5*time.Second)

	// The key still holds what this Client wrote, so writing again is allowed.
	if _, err := owner.Register(ctx, key, value, 30*time.Second, sd.ConflictCompareAndSwap); err != nil {
		t.Fatalf("compare-and-swap over its own registration: %v", err)
	}

	other := NewClient(etcd)
	if _, err := other.Register(ctx, key, value, 30*time.Second, sd.ConflictOverwrite); err != nil {
		t.Fatalf("competing overwrite Register: %v", err)
	}

	// The key moved on, so the original owner must not take it back.
	if _, err := owner.Register(ctx, key, value, 30*time.Second, sd.ConflictCompareAndSwap); !errors.Is(err, sd.ErrConflict) {
		t.Fatalf("compare-and-swap after another writer took the key: err = %v, want sd.ErrConflict", err)
	}
}
