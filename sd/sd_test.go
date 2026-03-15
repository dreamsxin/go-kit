// Package sd_test provides unit tests for the sd/endpointer, balancer and executor
// components using an in-process mock Instancer — no Consul or other external
// service is required.
package sd_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd/endpointer"
	"github.com/dreamsxin/go-kit/sd/endpointer/balancer"
	"github.com/dreamsxin/go-kit/sd/endpointer/executor"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/interfaces"
)

// ─────────────────────────── mock Instancer ───────────────────────────

// mockInstancer is a simple in-process Instancer driven by a channel.
type mockInstancer struct {
	mu          sync.Mutex
	subscribers []chan<- events.Event
}

func (m *mockInstancer) Register(ch chan<- events.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribers = append(m.subscribers, ch)
}

func (m *mockInstancer) Deregister(ch chan<- events.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	subs := m.subscribers[:0]
	for _, s := range m.subscribers {
		if s != ch {
			subs = append(subs, s)
		}
	}
	m.subscribers = subs
}

func (m *mockInstancer) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.subscribers {
		close(ch)
	}
	m.subscribers = nil
}

func (m *mockInstancer) Broadcast(ev events.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.subscribers {
		ch <- ev
	}
}

// ─────────────────────────── factory helpers ───────────────────────────

func instanceEndpoint(instance string) endpoint.Endpoint {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		return instance, nil
	}
}

func newFactory() endpoint.Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		return instanceEndpoint(instance), io.NopCloser(nil), nil
	}
}

func newLogger(t *testing.T) *kitlog.Logger {
	t.Helper()
	l, _ := kitlog.NewDevelopment()
	return l
}

// ─────────────────────────── Endpointer tests ───────────────────────────

func TestEndpointer_EmptyInitially(t *testing.T) {
	inst := &mockInstancer{}
	ep := endpointer.NewEndpointer(inst, newFactory(), newLogger(t))
	endpoints, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(endpoints))
	}
}

func TestEndpointer_ReceivesInstances(t *testing.T) {
	inst := &mockInstancer{}
	ep := endpointer.NewEndpointer(inst, newFactory(), newLogger(t))

	inst.Broadcast(events.Event{Instances: []string{"host1:80", "host2:80"}})
	time.Sleep(20 * time.Millisecond) // let the goroutine process

	endpoints, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(endpoints))
	}
}

func TestEndpointer_UpdateInstances(t *testing.T) {
	inst := &mockInstancer{}
	ep := endpointer.NewEndpointer(inst, newFactory(), newLogger(t))

	inst.Broadcast(events.Event{Instances: []string{"a:80", "b:80", "c:80"}})
	time.Sleep(20 * time.Millisecond)

	inst.Broadcast(events.Event{Instances: []string{"a:80"}})
	time.Sleep(20 * time.Millisecond)

	endpoints, _ := ep.Endpoints()
	if len(endpoints) != 1 {
		t.Errorf("expected 1 endpoint after update, got %d", len(endpoints))
	}
}

func TestEndpointer_FactoryError(t *testing.T) {
	factoryErr := errors.New("factory fail")
	badFactory := func(instance string) (endpoint.Endpoint, io.Closer, error) {
		return nil, nil, factoryErr
	}
	inst := &mockInstancer{}
	ep := endpointer.NewEndpointer(inst, badFactory, newLogger(t))

	inst.Broadcast(events.Event{Instances: []string{"bad:80"}})
	time.Sleep(20 * time.Millisecond)

	endpoints, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints when factory fails, got %d", len(endpoints))
	}
}

// ─────────────────────────── RoundRobin balancer tests ───────────────────────────

func TestRoundRobin_NoEndpoints(t *testing.T) {
	inst := &mockInstancer{}
	ep := endpointer.NewEndpointer(inst, newFactory(), newLogger(t))
	rr := balancer.NewRoundRobin(ep)

	_, err := rr.Endpoint()
	if !errors.Is(err, interfaces.ErrNoEndpoints) {
		t.Errorf("want ErrNoEndpoints, got %v", err)
	}
}

func TestRoundRobin_SingleEndpoint(t *testing.T) {
	inst := &mockInstancer{}
	ep := endpointer.NewEndpointer(inst, newFactory(), newLogger(t))

	inst.Broadcast(events.Event{Instances: []string{"only:80"}})
	time.Sleep(20 * time.Millisecond)

	rr := balancer.NewRoundRobin(ep)
	for i := 0; i < 5; i++ {
		e, err := rr.Endpoint()
		if err != nil {
			t.Fatalf("Endpoint[%d]: unexpected error: %v", i, err)
		}
		resp, _ := e(context.Background(), nil)
		if resp != "only:80" {
			t.Errorf("Endpoint[%d]: want 'only:80', got %v", i, resp)
		}
	}
}

func TestRoundRobin_Distributes(t *testing.T) {
	instances := []string{"svc1:80", "svc2:80", "svc3:80"}
	inst := &mockInstancer{}
	ep := endpointer.NewEndpointer(inst, newFactory(), newLogger(t))

	inst.Broadcast(events.Event{Instances: instances})
	time.Sleep(20 * time.Millisecond)

	rr := balancer.NewRoundRobin(ep)
	counts := map[string]int{}
	n := 9
	for i := 0; i < n; i++ {
		e, err := rr.Endpoint()
		if err != nil {
			t.Fatalf("Endpoint[%d]: %v", i, err)
		}
		resp, _ := e(context.Background(), nil)
		counts[resp.(string)]++
	}
	for _, inst := range instances {
		if counts[inst] != n/len(instances) {
			t.Errorf("instance %q: want %d calls, got %d", inst, n/len(instances), counts[inst])
		}
	}
}

// ─────────────────────────── Retry executor tests ───────────────────────────

// fixedBalancer always returns the same endpoint.
type fixedBalancer struct{ ep endpoint.Endpoint }

func (f fixedBalancer) Endpoint() (endpoint.Endpoint, error) { return f.ep, nil }

// errorBalancer always returns an error.
type errorBalancer struct{ err error }

func (e errorBalancer) Endpoint() (endpoint.Endpoint, error) { return nil, e.err }

// countingBalancer counts calls and fails until a threshold is met.
type countingBalancer struct {
	mu        sync.Mutex
	calls     int
	threshold int
}

func (c *countingBalancer) Endpoint() (endpoint.Endpoint, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls >= c.threshold {
		return endpoint.Nop, nil
	}
	return nil, errors.New("not yet")
}

func TestRetry_SuccessFirstTry(t *testing.T) {
	want := "result"
	b := fixedBalancer{ep: func(ctx context.Context, req interface{}) (interface{}, error) {
		return want, nil
	}}
	ep := executor.Retry(3, time.Second, b)
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != want {
		t.Errorf("want %q, got %v", want, resp)
	}
}

func TestRetry_SuccessAfterRetries(t *testing.T) {
	cb := &countingBalancer{threshold: 3}
	ep := executor.Retry(5, time.Second, cb)
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestRetry_ExhaustsMaxRetries(t *testing.T) {
	fail := errors.New("always fail")
	b := errorBalancer{err: fail}
	ep := executor.Retry(3, time.Second, b)
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	var retErr executor.RetryError
	if !errors.As(err, &retErr) {
		t.Fatalf("expected RetryError, got %T: %v", err, err)
	}
	if len(retErr.RawErrors) == 0 {
		t.Error("RetryError.RawErrors should not be empty")
	}
}

func TestRetry_Timeout(t *testing.T) {
	// Endpoint blocks longer than the retry timeout.
	b := fixedBalancer{ep: func(ctx context.Context, req interface{}) (interface{}, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
			return "ok", nil
		}
	}}
	ep := executor.Retry(10, 50*time.Millisecond, b)
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestRetry_RetryWithCallback(t *testing.T) {
	callCount := 0
	b := fixedBalancer{ep: func(ctx context.Context, req interface{}) (interface{}, error) {
		callCount++
		if callCount < 3 {
			return nil, errors.New("not ready")
		}
		return "done", nil
	}}

	cbCalls := 0
	ep := executor.RetryWithCallback(time.Second, b, func(n int, err error) (bool, error) {
		cbCalls++
		return n < 5, nil
	})

	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "done" {
		t.Errorf("want 'done', got %v", resp)
	}
	if cbCalls == 0 {
		t.Error("callback should have been called at least once")
	}
}
