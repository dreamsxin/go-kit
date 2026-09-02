package etcd

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultTTL is the lease lifetime of a registration. The client renews it at
// roughly a third of this interval, so the value is the worst-case delay before
// a dead instance leaves discovery.
const DefaultTTL = 15 * time.Second

// Registrar keeps one instance registered in etcd for as long as it runs.
//
// The registration is a leased key, so an instance that crashes disappears on
// its own. The other half of that deal is renewal: if the lease is lost — an
// expired keepalive, a member restart, a network partition long enough to
// matter — the key is gone while the process is still healthy. Registrar
// watches for exactly that and registers again, because an instance that is
// serving traffic but missing from discovery is worse than one that is down.
type Registrar struct {
	client    Client
	logger    *slog.Logger
	service   string
	address   string
	namespace string
	id        string
	ttl       time.Duration
	metadata  map[string]string
	retryBase time.Duration

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	key    string
}

// RegistrarOption configures a Registrar.
type RegistrarOption func(*Registrar)

// IDRegistrarOptions sets the instance key suffix. The default is the address,
// which is unique per instance; override it when a stable identity has to
// survive an address change.
func IDRegistrarOptions(id string) RegistrarOption {
	return func(r *Registrar) {
		r.id = id
	}
}

// NamespaceRegistrarOptions sets the key namespace. It must match the namespace
// the Instancer watches; DefaultNamespace is the default on both sides.
func NamespaceRegistrarOptions(namespace string) RegistrarOption {
	return func(r *Registrar) {
		r.namespace = namespace
	}
}

// TTLRegistrarOptions sets the lease lifetime. Values at or below zero fall
// back to DefaultTTL. Shorter means faster removal of dead instances and more
// renewal traffic.
func TTLRegistrarOptions(ttl time.Duration) RegistrarOption {
	return func(r *Registrar) {
		if ttl > 0 {
			r.ttl = ttl
		}
	}
}

// MetaRegistrarOptions reports static labels with the registration, which is
// what discovery consumers filter and weight on: zone, version, protocol,
// capability, weight, tenant. Repeated calls merge.
//
// Keep live load out of here. Every update rewrites the key and wakes every
// watcher, and consumers would still read a stale number; balancers that need
// live signals measure them in process instead.
func MetaRegistrarOptions(meta map[string]string) RegistrarOption {
	return func(r *Registrar) {
		if len(meta) == 0 {
			return
		}
		if r.metadata == nil {
			r.metadata = make(map[string]string, len(meta))
		}
		for key, value := range meta {
			r.metadata[key] = value
		}
	}
}

// NewRegistrar describes one instance of service reachable at address:port.
func NewRegistrar(client Client, logger *slog.Logger, service, address string, port int, options ...RegistrarOption) *Registrar {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	r := &Registrar{
		client:    client,
		logger:    logger,
		service:   service,
		address:   net.JoinHostPort(address, strconv.Itoa(port)),
		ttl:       DefaultTTL,
		retryBase: 100 * time.Millisecond,
	}
	for _, option := range options {
		option(r)
	}
	if r.id == "" {
		r.id = r.address
	}
	r.key = servicePrefix(r.namespace, r.service) + strings.Trim(r.id, "/")
	return r
}

// Register writes the instance key and keeps it alive until Deregister.
//
// Calling it twice without an intervening Deregister is a no-op on the second
// call: one Registrar owns one key.
func (r *Registrar) Register() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx != nil {
		return nil
	}

	value, err := encodeRegistration(r.address, r.metadata)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	lost, err := r.client.Register(ctx, r.key, value, r.ttl)
	if err != nil {
		cancel()
		return err
	}
	r.ctx, r.cancel = ctx, cancel
	r.logger.Debug("etcd service registered", "key", r.key, "ttl", r.ttl)

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.supervise(ctx, value, lost)
	}()
	return nil
}

// Deregister removes the instance key and stops renewing it.
func (r *Registrar) Deregister() error {
	r.mu.Lock()
	cancel := r.cancel
	r.ctx, r.cancel = nil, nil
	r.mu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()
	r.wg.Wait()

	if err := r.client.Deregister(context.Background(), r.key); err != nil {
		return err
	}
	r.logger.Debug("etcd service deregistered", "key", r.key)
	return nil
}

// supervise re-registers whenever the lease stops being renewed. Backoff is
// bounded so a cluster that is down for a while does not turn one instance into
// a hot loop, and resets on success so a single blip is recovered promptly.
func (r *Registrar) supervise(ctx context.Context, value string, lost <-chan struct{}) {
	delay := r.retryBase
	for {
		select {
		case <-ctx.Done():
			return
		case <-lost:
		}

		if ctx.Err() != nil {
			return
		}
		r.logger.Warn("etcd registration lost, registering again", "key", r.key)

		for {
			if !waitForRetry(delay, ctx.Done()) {
				return
			}
			again, err := r.client.Register(ctx, r.key, value, r.ttl)
			if err == nil {
				lost = again
				delay = r.retryBase
				r.logger.Debug("etcd service registered", "key", r.key, "ttl", r.ttl)
				break
			}
			if ctx.Err() != nil {
				return
			}
			r.logger.Debug("etcd re-registration failed", "key", r.key, "err", err, "retry_after", delay)
			delay = nextDelay(delay)
		}
	}
}
