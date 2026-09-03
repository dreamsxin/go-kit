// Package etcd registers services in etcd and discovers them again.
//
// etcd has no built-in service model, so this package defines a small one: one
// key per instance under a service prefix, holding the address and the static
// labels the instance reported. The key is attached to a lease and kept alive,
// which is what makes a crashed instance disappear on its own.
package etcd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// DefaultRequestTimeout bounds a single etcd request. Discovery reads happen on
// the critical path of a client start-up, so they must not hang forever on an
// unreachable member.
const DefaultRequestTimeout = 5 * time.Second

// Client is the etcd surface this package needs. It is an interface so callers
// can supply their own transport, and so tests need no etcd server.
type Client interface {
	// Entries returns every key/value under prefix and the store revision they
	// were read at.
	Entries(ctx context.Context, prefix string) (map[string]string, int64, error)

	// Watch reports changes under prefix starting at revision. Each receive
	// means "the set may have changed"; the caller re-reads. The channel is
	// closed when the watch ends, whether because ctx ended or because etcd
	// dropped it.
	Watch(ctx context.Context, prefix string, revision int64) (<-chan struct{}, error)

	// Register writes key with value under a lease of ttl and keeps that lease
	// alive until ctx ends. The returned channel is closed once the
	// registration is no longer being kept alive — a lost lease, a restarted
	// cluster — so the caller can register again instead of silently vanishing
	// from discovery.
	Register(ctx context.Context, key, value string, ttl time.Duration) (lost <-chan struct{}, err error)

	// Deregister removes key.
	Deregister(ctx context.Context, key string) error
}

// ClientOption configures the etcd client wrapper.
type ClientOption func(*client)

// WithRequestTimeout bounds each etcd request. Values at or below zero fall
// back to DefaultRequestTimeout.
func WithRequestTimeout(timeout time.Duration) ClientOption {
	return func(c *client) {
		if timeout > 0 {
			c.timeout = timeout
		}
	}
}

// NewClient wraps a concrete etcd client.
func NewClient(etcd *clientv3.Client, options ...ClientOption) Client {
	c := &client{etcd: etcd, timeout: DefaultRequestTimeout, leases: map[string]clientv3.LeaseID{}}
	for _, option := range options {
		option(c)
	}
	return c
}

type client struct {
	etcd    *clientv3.Client
	timeout time.Duration

	mu     sync.Mutex
	leases map[string]clientv3.LeaseID
}

func (c *client) Entries(ctx context.Context, prefix string) (map[string]string, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	response, err := c.etcd.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, 0, err
	}
	entries := make(map[string]string, len(response.Kvs))
	for _, kv := range response.Kvs {
		entries[string(kv.Key)] = string(kv.Value)
	}
	revision := int64(0)
	if response.Header != nil {
		revision = response.Header.Revision
	}
	return entries, revision, nil
}

func (c *client) Watch(ctx context.Context, prefix string, revision int64) (<-chan struct{}, error) {
	options := []clientv3.OpOption{clientv3.WithPrefix()}
	if revision > 0 {
		options = append(options, clientv3.WithRev(revision))
	}
	watch := c.etcd.Watch(ctx, prefix, options...)

	changes := make(chan struct{}, 1)
	go func() {
		defer close(changes)
		for response := range watch {
			if err := response.Err(); err != nil {
				return
			}
			// Coalesce: the consumer re-reads the whole prefix anyway, so one
			// pending signal is as good as ten.
			select {
			case changes <- struct{}{}:
			default:
			}
		}
	}()
	return changes, nil
}

func (c *client) Register(ctx context.Context, key, value string, ttl time.Duration) (<-chan struct{}, error) {
	seconds := int64(ttl.Seconds())
	if seconds < 1 {
		// A sub-second lease would be revoked between keepalives.
		seconds = 1
	}

	grantCtx, cancel := context.WithTimeout(ctx, c.timeout)
	lease, err := c.etcd.Grant(grantCtx, seconds)
	cancel()
	if err != nil {
		return nil, err
	}

	putCtx, cancel := context.WithTimeout(ctx, c.timeout)
	_, err = c.etcd.Put(putCtx, key, value, clientv3.WithLease(lease.ID))
	cancel()
	if err != nil {
		// The lease is granted but nothing is attached to it, so nothing will
		// ever renew it. Revoking now beats holding a dead lease for a TTL.
		c.revoke(lease.ID)
		return nil, err
	}

	keepAlive, err := c.etcd.KeepAlive(ctx, lease.ID)
	if err != nil {
		c.revoke(lease.ID)
		return nil, err
	}

	c.mu.Lock()
	c.leases[key] = lease.ID
	c.mu.Unlock()

	lost := make(chan struct{})
	go func() {
		defer close(lost)
		// The keepalive channel must be drained or etcd stops renewing. It
		// closes when the lease is gone or ctx ended, which is exactly the
		// signal the caller needs.
		for range keepAlive {
		}
	}()
	return lost, nil
}

func (c *client) Deregister(ctx context.Context, key string) error {
	c.mu.Lock()
	lease, leased := c.leases[key]
	c.mu.Unlock()

	// Delete and Revoke get a deadline each. Sharing one meant a Delete that
	// consumed the whole budget left the Revoke certain to fail, so a slow
	// delete turned into a leaked lease.
	var errs []error
	deleteCtx, cancelDelete := context.WithTimeout(ctx, c.timeout)
	_, err := c.etcd.Delete(deleteCtx, key)
	cancelDelete()
	if err != nil {
		errs = append(errs, fmt.Errorf("delete %s: %w", key, err))
	}

	if leased {
		// Revoking releases the lease immediately instead of leaving it to
		// expire, which matters when the same key is registered again at once.
		revokeCtx, cancelRevoke := context.WithTimeout(ctx, c.timeout)
		_, err := c.etcd.Revoke(revokeCtx, lease)
		cancelRevoke()
		if err != nil {
			errs = append(errs, fmt.Errorf("revoke lease for %s: %w", key, err))
		}
	}

	if err := errors.Join(errs...); err != nil {
		// Keep the lease recorded. A retry needs the same lease to revoke, and
		// forgetting it here would leave it renewing until its TTL ran out.
		return err
	}

	c.mu.Lock()
	// Drop the mapping only if it is still the lease this call revoked: a
	// concurrent Register may have replaced it, and that newer lease must not
	// be forgotten by an older teardown.
	if current, ok := c.leases[key]; ok && leased && current == lease {
		delete(c.leases, key)
	}
	c.mu.Unlock()
	return nil
}

// revoke releases a lease the caller could not finish attaching a key to. It is
// best effort: the lease expires on its own if etcd is unreachable, so there is
// nothing useful to report to a caller that is already returning an error.
func (c *client) revoke(lease clientv3.LeaseID) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	_, _ = c.etcd.Revoke(ctx, lease)
}
