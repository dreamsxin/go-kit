package etcd

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// Instancer watches one service prefix in etcd and publishes snapshots.
type Instancer struct {
	cache     *eventCache
	client    Client
	logger    *slog.Logger
	namespace string
	prefix    string
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	stopOnce  sync.Once
	retryBase time.Duration

	// revision is owned by the watch goroutine after NewInstancer returns.
	revision int64
}

// InstancerOption configures an Instancer.
type InstancerOption func(*Instancer)

// NamespaceInstancerOptions sets the key namespace to watch. It must match the
// namespace the registrations use; DefaultNamespace is the default on both
// sides.
func NamespaceInstancerOptions(namespace string) InstancerOption {
	return func(s *Instancer) {
		s.namespace = namespace
	}
}

// NewInstancer subscribes to a service prefix and keeps the latest snapshot.
//
// The initial read happens before NewInstancer returns, so a caller that gets
// an Instancer already has instances — or the error explaining why it does not.
func NewInstancer(client Client, logger *slog.Logger, service string, options ...InstancerOption) *Instancer {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Instancer{
		cache:     newEventCache(),
		client:    client,
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
		retryBase: 10 * time.Millisecond,
	}
	for _, option := range options {
		option(s)
	}
	s.prefix = servicePrefix(s.namespace, service)

	instances, revision, err := s.load(ctx)
	if err == nil {
		s.logger.Debug("etcd instances loaded", "prefix", s.prefix, "count", len(instances))
	} else {
		s.logger.Debug("etcd initial read failed", "prefix", s.prefix, "err", err)
	}
	s.cache.Update(Event{Instances: instances, Err: err})
	s.revision = revision

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.loop()
	}()
	return s
}

// Stop cancels the watch and joins its goroutine.
func (s *Instancer) Stop() {
	s.stopOnce.Do(s.cancel)
	s.wg.Wait()
}

// Close implements the service-discovery lifecycle contract.
func (s *Instancer) Close() error {
	s.Stop()
	return nil
}

// Register implements the discovery subscription contract.
func (s *Instancer) Register(ch chan Event) Event { return s.cache.Register(ch) }

// Deregister implements the discovery subscription contract.
func (s *Instancer) Deregister(ch chan Event) { s.cache.Deregister(ch) }

func (s *Instancer) loop() {
	delay := s.retryBase
	for {
		// Watching from the revision after the last read is what closes the gap
		// between "read the prefix" and "watch the prefix": no change in
		// between can slip through unseen.
		changes, err := s.client.Watch(s.ctx, s.prefix, s.revision+1)
		if err != nil {
			if s.stopped() {
				return
			}
			s.logger.Debug("etcd watch failed", "prefix", s.prefix, "err", err, "retry_after", delay)
			s.cache.Update(Event{Err: err})
			if !waitForRetry(delay, s.ctx.Done()) {
				return
			}
			delay = nextDelay(delay)
			continue
		}

		before := s.revision
		if err := s.consume(changes); err != nil {
			return
		}
		if s.revision != before {
			// The watch did useful work before it ended, so this is a fresh
			// failure rather than a continuing one.
			delay = s.retryBase
		}

		// The watch channel closed: either we are shutting down, or etcd
		// dropped it and we re-establish from the last revision we saw.
		if s.stopped() {
			return
		}
		s.logger.Debug("etcd watch closed", "prefix", s.prefix, "retry_after", delay)
		if !waitForRetry(delay, s.ctx.Done()) {
			return
		}
		delay = nextDelay(delay)
	}
}

// errStopped ends the watch loop. It is not returned to callers.
var errStopped = errors.New("etcd: instancer stopped")

// consume re-reads the prefix on every change signal and returns when the watch
// ends. It selects on ctx as well as on the channel: Stop must not depend on the
// Client implementation closing the channel promptly, or a well-behaved shutdown
// hangs on a misbehaving watch.
func (s *Instancer) consume(changes <-chan struct{}) error {
	for {
		select {
		case <-s.ctx.Done():
			return errStopped
		case _, open := <-changes:
			if !open {
				return nil
			}
		}

		instances, current, err := s.load(s.ctx)
		if err != nil {
			if s.stopped() {
				return errStopped
			}
			s.logger.Debug("etcd read failed", "prefix", s.prefix, "err", err)
			s.cache.Update(Event{Err: err})
			continue
		}
		s.revision = current
		s.logger.Debug("etcd instances updated", "prefix", s.prefix, "count", len(instances), "revision", current)
		s.cache.Update(Event{Instances: instances})
	}
}

func (s *Instancer) stopped() bool {
	return errors.Is(s.ctx.Err(), context.Canceled)
}

// load reads the prefix and decodes every entry. A single malformed value is
// skipped rather than failing the snapshot: one bad key written by hand should
// not take a whole service out of discovery.
func (s *Instancer) load(ctx context.Context) ([]Instance, int64, error) {
	entries, revision, err := s.client.Entries(ctx, s.prefix)
	if err != nil {
		return nil, 0, err
	}

	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	instances := make([]Instance, 0, len(entries))
	for _, key := range keys {
		instance, err := decodeRegistration(entries[key])
		if err != nil {
			s.logger.Warn("etcd registration ignored", "key", key, "err", err)
			continue
		}
		instances = append(instances, instance)
	}
	return instances, revision, nil
}

func waitForRetry(delay time.Duration, stop <-chan struct{}) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-stop:
		return false
	}
}

func nextDelay(delay time.Duration) time.Duration {
	delay *= 2
	delay = time.Duration(float64(delay) * (rand.Float64() + 0.5))
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}
