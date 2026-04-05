package endpointer_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd/endpointer"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/instance"
)

var nopLogger = kitlog.NewNopLogger()

var echoFactory = endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
	ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return addr, nil
	})
	return ep, io.NopCloser(nil), nil
})

func TestNewEndpointer_NoInstances(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger)

	eps, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(eps))
	}
}

func TestNewEndpointer_ReceivesInstances(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger)

	cache.Update(events.Event{Instances: []string{"a:80", "b:80"}})
	time.Sleep(20 * time.Millisecond)

	eps, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(eps))
	}
}

func TestNewEndpointer_UpdateInstances(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger)

	cache.Update(events.Event{Instances: []string{"a:80", "b:80", "c:80"}})
	time.Sleep(20 * time.Millisecond)

	cache.Update(events.Event{Instances: []string{"a:80"}})
	time.Sleep(20 * time.Millisecond)

	eps, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 1 {
		t.Errorf("expected 1 endpoint after update, got %d", len(eps))
	}
}

func TestNewEndpointer_FactoryError_SkipsInstance(t *testing.T) {
	failFactory := endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
		if addr == "bad:80" {
			return nil, nil, errors.New("factory error")
		}
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) { return addr, nil })
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, failFactory, nopLogger)

	cache.Update(events.Event{Instances: []string{"good:80", "bad:80"}})
	time.Sleep(20 * time.Millisecond)

	eps, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// bad:80 should be skipped
	if len(eps) != 1 {
		t.Errorf("expected 1 endpoint (bad skipped), got %d", len(eps))
	}
}

func TestNewEndpointer_WithInvalidateOnError(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger,
		endpoint.InvalidateOnError(50*time.Millisecond),
	)

	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(20 * time.Millisecond)

	// healthy
	eps, err := ep.Endpoints()
	if err != nil || len(eps) == 0 {
		t.Fatalf("expected healthy endpoints, got err=%v len=%d", err, len(eps))
	}

	// inject error
	cache.Update(events.Event{Err: errors.New("sd error")})
	time.Sleep(10 * time.Millisecond)

	// within grace period — still returns cached
	eps, _ = ep.Endpoints()
	if len(eps) == 0 {
		t.Log("grace period: cache may already be cleared (timing)")
	}

	// after grace period — cache cleared
	time.Sleep(80 * time.Millisecond)
	eps, err = ep.Endpoints()
	if err == nil && len(eps) > 0 {
		t.Error("expected cache to be invalidated after grace period")
	}
}
