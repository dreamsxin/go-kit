package consul

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	consul "github.com/hashicorp/consul/api"
)

const defaultIndex = 0

var errStopped = errors.New("quit and closed consul instancer")

// 服务实例发现类
type Instancer struct {
	cache       *eventCache
	client      Client
	logger      *slog.Logger
	service     string
	tags        []string
	passingOnly bool // 只返回正常的实例
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	stopOnce    sync.Once
}

type InstancerOption func(*Instancer)

func TagsInstancerOptions(tags []string) InstancerOption {
	return func(r *Instancer) {
		r.tags = tags
	}
}

func NewInstancer(client Client, logger *slog.Logger, service string, passingOnly bool, options ...InstancerOption) *Instancer {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Instancer{
		cache:       newEventCache(),
		client:      client,
		logger:      logger,
		service:     service,
		passingOnly: passingOnly,
		ctx:         ctx,
		cancel:      cancel,
	}
	for _, option := range options {
		option(s)
	}

	instances, index, err := s.getInstances(ctx, defaultIndex)
	if err == nil {
		s.logger.Debug("consul instances loaded", "count", len(instances))
	} else {
		s.logger.Debug("consul initial query failed", "err", err)
	}
	s.cache.Update(Event{Instances: instances, Err: err})
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.loop(index)
	}()
	return s
}

// Stop terminates the instancer.
func (s *Instancer) Stop() {
	s.stopOnce.Do(s.cancel)
	s.wg.Wait()
}

// Close implements the service-discovery lifecycle contract.
func (s *Instancer) Close() error {
	s.Stop()
	return nil
}

func (s *Instancer) loop(lastIndex uint64) {
	var (
		instances []Instance
		err       error
		d         time.Duration = 10 * time.Millisecond
		index     uint64
	)
	for {
		instances, index, err = s.getInstances(s.ctx, lastIndex)
		switch {
		case errors.Is(err, errStopped):
			s.logger.Debug("consul watch stopped")
			return
		case err != nil:
			s.logger.Debug("consul watch failed", "err", err, "retry_after", d)
			if !waitForRetry(d, s.ctx.Done()) {
				return
			}
			d = nextDelay(d)
			s.cache.Update(Event{Err: err})
		case index == defaultIndex:
			s.logger.Debug("consul watch returned zero index", "retry_after", d)
			if !waitForRetry(d, s.ctx.Done()) {
				return
			}
			d = nextDelay(d)
		case index < lastIndex:
			s.logger.Debug("consul watch index regressed", "index", index, "previous", lastIndex, "retry_after", d)
			lastIndex = defaultIndex
			if !waitForRetry(d, s.ctx.Done()) {
				return
			}
			d = nextDelay(d)
		default:
			s.logger.Debug("consul instances updated", "index", index, "count", len(instances))
			lastIndex = index
			s.cache.Update(Event{Instances: instances})
			d = 10 * time.Millisecond
		}
	}
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

// 获取实例列表
func (s *Instancer) getInstances(ctx context.Context, lastIndex uint64) ([]Instance, uint64, error) {
	tag := ""
	if len(s.tags) > 0 {
		tag = s.tags[0]
	}

	s.logger.Debug("query consul instances", "last_index", lastIndex)
	query := (&consul.QueryOptions{WaitIndex: lastIndex}).WithContext(ctx)
	entries, meta, err := s.client.Service(s.service, tag, s.passingOnly, query)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, 0, errStopped
		}
		return nil, 0, err
	}
	if meta == nil {
		return nil, 0, fmt.Errorf("consul: service query returned nil metadata")
	}
	if len(s.tags) > 1 {
		entries = filterEntries(entries, s.tags[1:]...)
	}
	return makeInstances(entries), meta.LastIndex, nil
}

// Register implements Instancer.
func (s *Instancer) Register(ch chan Event) Event {
	return s.cache.Register(ch)
}

// Deregister implements Instancer.
func (s *Instancer) Deregister(ch chan Event) {
	s.cache.Deregister(ch)
}

func filterEntries(entries []*consul.ServiceEntry, tags ...string) []*consul.ServiceEntry {
	var es []*consul.ServiceEntry

ENTRIES:
	for _, entry := range entries {
		ts := make(map[string]struct{}, len(entry.Service.Tags))
		for _, tag := range entry.Service.Tags {
			ts[tag] = struct{}{}
		}

		for _, tag := range tags {
			if _, ok := ts[tag]; !ok {
				continue ENTRIES
			}
		}
		es = append(es, entry)
	}

	return es
}

func makeInstances(entries []*consul.ServiceEntry) []Instance {
	instances := make([]Instance, len(entries))
	for i, entry := range entries {
		addr := entry.Node.Address
		if entry.Service.Address != "" {
			addr = entry.Service.Address
		}
		instances[i] = Instance{
			Address:  fmt.Sprintf("%s:%d", addr, entry.Service.Port),
			Metadata: metadataFor(entry),
		}
	}
	return instances
}

// metadataFor lifts the service Meta a registration reported into instance
// labels. Tags stay out: they are a set, not key/value pairs, and NewInstancer
// already filters on them server-side.
func metadataFor(entry *consul.ServiceEntry) map[string]any {
	if len(entry.Service.Meta) == 0 {
		return nil
	}
	metadata := make(map[string]any, len(entry.Service.Meta))
	for key, value := range entry.Service.Meta {
		metadata[key] = value
	}
	return metadata
}
