package sd_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/events"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
)

func nopFactory(addr string) (endpoint.Endpoint, io.Closer, error) {
	ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return addr, nil
	})
	return ep, io.NopCloser(nil), nil
}

func nopLogger() *kitlog.Logger { return kitlog.NewNopLogger() }

func newTestEndpoint(t *testing.T, cache *instance.Cache, opts ...sd.Option) endpoint.Endpoint {
	t.Helper()
	ep, closer, err := sd.NewEndpoint(cache, endpoint.Factory(nopFactory), nopLogger(), opts...)
	if err != nil {
		t.Fatalf("NewEndpoint: %v", err)
	}
	t.Cleanup(func() {
		if err := closer.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return ep
}

// ── NewEndpoint ───────────────────────────────────────────────────────────────

func TestNewEndpoint_CallsInstance(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"host:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := newTestEndpoint(t, cache)
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "host:80" {
		t.Errorf("want 'host:80', got %v", resp)
	}
}

func TestNewEndpoint_RoundRobin(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"a:80", "b:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := newTestEndpoint(t, cache,
		sd.WithMaxAttempts(1),
		sd.WithTimeout(time.Second),
	)

	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		resp, err := ep(context.Background(), nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		seen[resp.(string)]++
	}
	if seen["a:80"] == 0 || seen["b:80"] == 0 {
		t.Errorf("expected both instances to be called, got %v", seen)
	}
}

func TestNewEndpoint_WithOptions(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := newTestEndpoint(t, cache,
		sd.WithMaxAttempts(2),
		sd.WithTimeout(500*time.Millisecond),
		sd.WithInvalidateOnError(5*time.Second),
	)
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "svc:80" {
		t.Errorf("want 'svc:80', got %v", resp)
	}
}

// ── NewEndpointWithDefaults ───────────────────────────────────────────────────

func TestNewEndpointWithDefaults(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"default:80"}})
	time.Sleep(10 * time.Millisecond)

	ep, closer, err := sd.NewEndpointWithDefaults(cache, endpoint.Factory(nopFactory), nopLogger())
	if err != nil {
		t.Fatalf("NewEndpointWithDefaults: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "default:80" {
		t.Errorf("want 'default:80', got %v", resp)
	}
}

func TestNewEndpoint_RejectsInvalidConfiguration(t *testing.T) {
	cache := instance.NewCache()
	tests := []struct {
		name    string
		src     *instance.Cache
		factory endpoint.Factory
		logger  *kitlog.Logger
		opts    []sd.Option
		want    string
	}{
		{name: "nil instancer", factory: nopFactory, logger: nopLogger(), want: "instancer is nil"},
		{name: "nil factory", src: cache, logger: nopLogger(), want: "factory is nil"},
		{name: "nil logger", src: cache, factory: nopFactory, want: "logger is nil"},
		{name: "attempts", src: cache, factory: nopFactory, logger: nopLogger(), opts: []sd.Option{sd.WithMaxAttempts(0)}, want: "max attempts"},
		{name: "timeout", src: cache, factory: nopFactory, logger: nopLogger(), opts: []sd.Option{sd.WithTimeout(0)}, want: "timeout"},
		{name: "invalidation", src: cache, factory: nopFactory, logger: nopLogger(), opts: []sd.Option{sd.WithInvalidateOnError(-time.Second)}, want: "invalidate-on-error"},
		{name: "nil option", src: cache, factory: nopFactory, logger: nopLogger(), opts: []sd.Option{nil}, want: "option 0 is nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, closer, err := sd.NewEndpoint(tt.src, tt.factory, tt.logger, tt.opts...)
			if closer != nil {
				t.Fatal("invalid construction returned a closer")
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestNewEndpoint_CloserReleasesFactoryResources(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	closed := false
	factory := endpoint.Factory(func(string) (endpoint.Endpoint, io.Closer, error) {
		return endpoint.Nop, closerFunc(func() error {
			closed = true
			return nil
		}), nil
	})

	ep, closer, err := sd.NewEndpoint(cache, factory, nopLogger())
	if err != nil {
		t.Fatalf("NewEndpoint: %v", err)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !closed {
		t.Fatal("factory resource was not closed")
	}
	_, err = ep(context.Background(), nil)
	if !errors.Is(err, endpoint.ErrEndpointCacheClosed) {
		t.Fatalf("call after Close error = %v, want ErrEndpointCacheClosed", err)
	}
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }
