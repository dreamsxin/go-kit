package consul

import (
	"errors"
	"fmt"
	"time"

	consul "github.com/hashicorp/consul/api"

	"github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/instance"
	"github.com/dreamsxin/go-kit/utils"
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
	quitc       chan struct{}
}

type InstancerOption func(*Instancer)

func TagsInstancerOptions(tags []string) InstancerOption {
	return func(r *Instancer) {
		r.tags = tags
	}
}

func NewInstancer(client Client, logger *log.Logger, service string, passingOnly bool, options ...InstancerOption) *Instancer {
	s := &Instancer{
		cache:       instance.NewCache(),
		client:      client,
		logger:      logger,
		service:     service,
		passingOnly: passingOnly,
		quitc:       make(chan struct{}),
	}

	instances, index, err := s.getInstances(defaultIndex, nil)
	if err == nil {
		s.logger.Sugar().Errorln("instances", len(instances))
	} else {
		s.logger.Sugar().Debugln("err", err)
	}
	for _, option := range options {
		option(s)
	}

	s.cache.Update(events.Event{Instances: instances, Err: err})
	go s.loop(index)
	return s
}

// Stop terminates the instancer.
func (s *Instancer) Stop() {
	close(s.quitc)
}

func (s *Instancer) loop(lastIndex uint64) {
	var (
		instances []string
		err       error
		d         time.Duration = 10 * time.Millisecond
		index     uint64
	)
	for {
		instances, index, err = s.getInstances(lastIndex, s.quitc)
		switch {
		case errors.Is(err, errStopped):
			s.logger.Sugar().Debugln("loop", errStopped)
			return // stopped via quitc
		case err != nil:
			s.logger.Sugar().Debugln("loop", err, d.Seconds())
			time.Sleep(d)
			d = utils.Exponential(d)
			s.cache.Update(events.Event{Err: err})
		case index == defaultIndex:
			s.logger.Sugar().Debugln("loop", "index is not sane", d.Seconds())
			time.Sleep(d)
			d = utils.Exponential(d)
		case index < lastIndex:
			s.logger.Sugar().Debugln("loop", "index is less than previous; resetting to default", d.Seconds())
			lastIndex = defaultIndex
			time.Sleep(d)
			d = utils.Exponential(d)
		default:
			s.logger.Sugar().Debugln("loop", "default", "index", index)
			lastIndex = index
			s.cache.Update(events.Event{Instances: instances})
			d = 10 * time.Millisecond
		}
	}
}

// 获取实例列表
func (s *Instancer) getInstances(lastIndex uint64, interruptc chan struct{}) ([]string, uint64, error) {
	tag := ""
	if len(s.tags) > 0 {
		tag = s.tags[0]
	}

	type response struct {
		instances []string
		index     uint64
	}

	var (
		errc = make(chan error, 1)
		resc = make(chan response, 1)
	)

	go func() {
		s.logger.Sugar().Debugln("getInstances", "lastIndex", lastIndex)
		entries, meta, err := s.client.Service(s.service, tag, s.passingOnly, &consul.QueryOptions{
			WaitIndex: lastIndex,
		})
		s.logger.Sugar().Debugln("getInstances", entries, meta, err)
		if err != nil {
			errc <- err
			return
		}
		if len(s.tags) > 1 {
			entries = filterEntries(entries, s.tags[1:]...)
		}
		resc <- response{
			instances: makeInstances(entries),
			index:     meta.LastIndex,
		}
	}()

	select {
	case err := <-errc:
		s.logger.Sugar().Debugln("getInstances", err)
		return nil, 0, err
	case res := <-resc:
		s.logger.Sugar().Debugln("getInstances", res)
		return res.instances, res.index, nil
	case <-interruptc:
		s.logger.Sugar().Debugln("getInstances", errStopped)
		return nil, 0, errStopped
	}
}

// Register implements Instancer.
func (s *Instancer) Register(ch chan<- events.Event) {
	s.cache.Register(ch)
}

// Deregister implements Instancer.
func (s *Instancer) Deregister(ch chan<- events.Event) {
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
