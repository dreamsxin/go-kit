package consul

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	consul "github.com/hashicorp/consul/api"

	"github.com/dreamsxin/go-kit/v2/log"
	"github.com/dreamsxin/go-kit/v2/sd/events"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"github.com/dreamsxin/go-kit/v2/utils"
)

const defaultIndex = 0

var errStopped = errors.New("quit and closed consul instancer")

// 服务实例发现类
type Instancer struct {
	cache       *instance.Cache
	client      Client
	logger      *log.Logger
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

func NewInstancer(client Client, logger *log.Logger, service string, passingOnly bool, options ...InstancerOption) *Instancer {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Instancer{
		cache:       instance.NewCache(),
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
		s.logger.Sugar().Debugln("instances", len(instances))
	} else {
		s.logger.Sugar().Debugln("err", err)
	}
	s.cache.Update(events.Event{Instances: instances, Err: err})
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

func (s *Instancer) loop(lastIndex uint64) {
	var (
		instances []string
		err       error
		d         time.Duration = 10 * time.Millisecond
		index     uint64
	)
	for {
		instances, index, err = s.getInstances(s.ctx, lastIndex)
		switch {
		case errors.Is(err, errStopped):
			s.logger.Sugar().Debugln("loop", errStopped)
			return
		case err != nil:
			s.logger.Sugar().Debugln("loop", err, d.Seconds())
			if !waitForRetry(d, s.ctx.Done()) {
				return
			}
			d = utils.Exponential(d)
			s.cache.Update(events.Event{Err: err})
		case index == defaultIndex:
			s.logger.Sugar().Debugln("loop", "index is not sane", d.Seconds())
			if !waitForRetry(d, s.ctx.Done()) {
				return
			}
			d = utils.Exponential(d)
		case index < lastIndex:
			s.logger.Sugar().Debugln("loop", "index is less than previous; resetting to default", d.Seconds())
			lastIndex = defaultIndex
			if !waitForRetry(d, s.ctx.Done()) {
				return
			}
			d = utils.Exponential(d)
		default:
			s.logger.Sugar().Debugln("loop", "default", "index", index)
			lastIndex = index
			s.cache.Update(events.Event{Instances: instances})
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

// 获取实例列表
func (s *Instancer) getInstances(ctx context.Context, lastIndex uint64) ([]string, uint64, error) {
	tag := ""
	if len(s.tags) > 0 {
		tag = s.tags[0]
	}

	s.logger.Sugar().Debugln("getInstances", "lastIndex", lastIndex)
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
func (s *Instancer) Register(ch chan events.Event) events.Event {
	return s.cache.Register(ch)
}

// Deregister implements Instancer.
func (s *Instancer) Deregister(ch chan events.Event) {
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

func makeInstances(entries []*consul.ServiceEntry) []string {
	instances := make([]string, len(entries))
	for i, entry := range entries {
		addr := entry.Node.Address
		if entry.Service.Address != "" {
			addr = entry.Service.Address
		}
		instances[i] = fmt.Sprintf("%s:%d", addr, entry.Service.Port)
	}
	return instances
}
