package endpoint

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/sd/events"
	kitlog "github.com/dreamsxin/go-kit/log"
)

// ─────────────────────────── Nop ───────────────────────────

func TestNop(t *testing.T) {
	resp, err := Nop(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Nop should not return error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("Nop should return non-nil response")
	}
}

// ─────────────────────────── Chain / Middleware ───────────────────────────

func TestEndpointBasic(t *testing.T) {
	ep := func(ctx context.Context, request interface{}) (interface{}, error) {
		return "response", nil
	}
	resp, err := ep(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "response" {
		t.Errorf("expected 'response', got %v", resp)
	}
}

func TestMiddlewareChain(t *testing.T) {
	var calls []string

	mw1 := func(next Endpoint) Endpoint {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			calls = append(calls, "mw1")
			return next(ctx, request)
		}
	}
	mw2 := func(next Endpoint) Endpoint {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			calls = append(calls, "mw2")
			return next(ctx, request)
		}
	}
	ep := func(ctx context.Context, request interface{}) (interface{}, error) {
		calls = append(calls, "endpoint")
		return "ok", nil
	}

	chained := Chain(mw1, mw2)(ep)
	resp, err := chained(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("expected 'ok', got %v", resp)
	}
	expected := []string{"mw1", "mw2", "endpoint"}
	if len(calls) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(calls), calls)
	}
	for i, c := range calls {
		if c != expected[i] {
			t.Errorf("call[%d]: want %q, got %q", i, expected[i], c)
		}
	}
}

// Chain 只包一层时应透传原 endpoint
func TestChainSingle(t *testing.T) {
	var called bool
	mw := func(next Endpoint) Endpoint {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			called = true
			return next(ctx, req)
		}
	}
	ep := func(ctx context.Context, request interface{}) (interface{}, error) {
		return "direct", nil
	}
	chained := Chain(mw)(ep)
	resp, err := chained(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "direct" {
		t.Errorf("expected 'direct', got %v", resp)
	}
	if !called {
		t.Error("middleware should have been called")
	}
}

// ─────────────────────────── Failer ───────────────────────────

type failResponse struct{ err error }

func (f failResponse) Failed() error { return f.err }

func TestFailer_WithError(t *testing.T) {
	sentinel := errors.New("business logic failed")
	var resp interface{} = failResponse{err: sentinel}
	if f, ok := resp.(Failer); ok {
		if f.Failed() != sentinel {
			t.Errorf("expected sentinel error, got %v", f.Failed())
		}
	} else {
		t.Fatal("response should implement Failer")
	}
}

func TestFailer_WithoutError(t *testing.T) {
	var resp interface{} = failResponse{err: nil}
	if f, ok := resp.(Failer); ok {
		if f.Failed() != nil {
			t.Errorf("expected nil, got %v", f.Failed())
		}
	} else {
		t.Fatal("response should implement Failer")
	}
}

// ─────────────────────────── MetricsMiddleware ───────────────────────────

func TestMetricsMiddleware_Success(t *testing.T) {
	m := &Metrics{}
	ep := MetricsMiddleware(m)(func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})

	for i := 0; i < 3; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}
	if m.RequestCount != 3 {
		t.Errorf("RequestCount: want 3, got %d", m.RequestCount)
	}
	if m.SuccessCount != 3 {
		t.Errorf("SuccessCount: want 3, got %d", m.SuccessCount)
	}
	if m.ErrorCount != 0 {
		t.Errorf("ErrorCount: want 0, got %d", m.ErrorCount)
	}
	if m.TotalDuration < 0 {
		t.Error("TotalDuration should be non-negative")
	}
	if m.LastRequestTime.IsZero() {
		t.Error("LastRequestTime should not be zero")
	}
}

func TestMetricsMiddleware_Error(t *testing.T) {
	m := &Metrics{}
	sentinel := errors.New("oops")
	ep := MetricsMiddleware(m)(func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, sentinel
	})

	ep(context.Background(), nil) //nolint:errcheck
	ep(context.Background(), nil) //nolint:errcheck
	if m.RequestCount != 2 {
		t.Errorf("RequestCount: want 2, got %d", m.RequestCount)
	}
	if m.ErrorCount != 2 {
		t.Errorf("ErrorCount: want 2, got %d", m.ErrorCount)
	}
	if m.SuccessCount != 0 {
		t.Errorf("SuccessCount: want 0, got %d", m.SuccessCount)
	}
}

func TestMetricsMiddleware_Mixed(t *testing.T) {
	m := &Metrics{}
	calls := 0
	ep := MetricsMiddleware(m)(func(ctx context.Context, req interface{}) (interface{}, error) {
		calls++
		if calls%2 == 0 {
			return nil, errors.New("even fail")
		}
		return "odd ok", nil
	})

	for i := 0; i < 4; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}
	if m.RequestCount != 4 {
		t.Errorf("RequestCount: want 4, got %d", m.RequestCount)
	}
	if m.SuccessCount != 2 {
		t.Errorf("SuccessCount: want 2, got %d", m.SuccessCount)
	}
	if m.ErrorCount != 2 {
		t.Errorf("ErrorCount: want 2, got %d", m.ErrorCount)
	}
}

// ─────────────────────────── ErrorHandlingMiddleware ───────────────────────────

func TestErrorHandlingMiddleware_NoError(t *testing.T) {
	ep := ErrorHandlingMiddleware("op")(func(ctx context.Context, req interface{}) (interface{}, error) {
		return "val", nil
	})
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "val" {
		t.Errorf("expected 'val', got %v", resp)
	}
}

func TestErrorHandlingMiddleware_WrapsError(t *testing.T) {
	raw := errors.New("raw err")
	ep := ErrorHandlingMiddleware("myop")(func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, raw
	})
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ew *ErrorWrapper
	if !errors.As(err, &ew) {
		t.Fatalf("expected *ErrorWrapper, got %T: %v", err, err)
	}
	if ew.Operation != "myop" {
		t.Errorf("Operation: want %q, got %q", "myop", ew.Operation)
	}
	if !errors.Is(err, raw) {
		t.Errorf("Unwrap chain should reach raw error")
	}
}

func TestErrorWrapper_ErrorString(t *testing.T) {
	ew := &ErrorWrapper{Operation: "doThings", Err: errors.New("boom")}
	want := "doThings: boom"
	if ew.Error() != want {
		t.Errorf("want %q, got %q", want, ew.Error())
	}
}

// ─────────────────────────── EndpointCache ───────────────────────────

// nopCloser satisfies io.Closer for test factories.
type nopCloser struct{ closed bool }

func (n *nopCloser) Close() error { n.closed = true; return nil }

func makeFactory(instances map[string]Endpoint) Factory {
	return func(instance string) (Endpoint, io.Closer, error) {
		if ep, ok := instances[instance]; ok {
			return ep, &nopCloser{}, nil
		}
		return nil, nil, errors.New("unknown instance: " + instance)
	}
}

func TestEndpointCache_UpdateAndEndpoints(t *testing.T) {
	logger, _ := kitlog.NewDevelopment()
	ep1 := func(ctx context.Context, req interface{}) (interface{}, error) { return "svc1", nil }
	ep2 := func(ctx context.Context, req interface{}) (interface{}, error) { return "svc2", nil }

	factory := makeFactory(map[string]Endpoint{
		"host1:8080": ep1,
		"host2:8080": ep2,
	})

	cache := NewEndpointCache(factory, logger, EndpointerOptions{})
	cache.Update(events.Event{Instances: []string{"host1:8080", "host2:8080"}})

	endpoints, err := cache.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}
}

func TestEndpointCache_UpdateRemovesOld(t *testing.T) {
	logger, _ := kitlog.NewDevelopment()
	closerA := &nopCloser{}
	factoryCallCount := 0

	factory := func(instance string) (Endpoint, io.Closer, error) {
		factoryCallCount++
		if instance == "A" {
			return func(ctx context.Context, req interface{}) (interface{}, error) { return "A", nil }, closerA, nil
		}
		return func(ctx context.Context, req interface{}) (interface{}, error) { return "B", nil }, &nopCloser{}, nil
	}

	cache := NewEndpointCache(factory, logger, EndpointerOptions{})

	// 注册 A
	cache.Update(events.Event{Instances: []string{"A"}})
	if factoryCallCount != 1 {
		t.Errorf("factory should be called once, got %d", factoryCallCount)
	}

	// 更新为 B，A 应该被关闭
	cache.Update(events.Event{Instances: []string{"B"}})
	if !closerA.closed {
		t.Error("closer for 'A' should have been called after removal")
	}

	endpoints, _ := cache.Endpoints()
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
	}
}

func TestEndpointCache_SameInstanceReused(t *testing.T) {
	logger, _ := kitlog.NewDevelopment()
	factoryCallCount := 0
	factory := func(instance string) (Endpoint, io.Closer, error) {
		factoryCallCount++
		return Nop, &nopCloser{}, nil
	}

	cache := NewEndpointCache(factory, logger, EndpointerOptions{})
	cache.Update(events.Event{Instances: []string{"host:80"}})
	cache.Update(events.Event{Instances: []string{"host:80"}}) // same instance again

	if factoryCallCount != 1 {
		t.Errorf("factory should only be called once for same instance, got %d", factoryCallCount)
	}
}

func TestEndpointCache_ErrorEvent_NoInvalidate(t *testing.T) {
	logger, _ := kitlog.NewDevelopment()
	factory := makeFactory(map[string]Endpoint{"h:1": Nop})

	cache := NewEndpointCache(factory, logger, EndpointerOptions{InvalidateOnError: false})
	cache.Update(events.Event{Instances: []string{"h:1"}})

	// 发送错误事件，不开 InvalidateOnError 时不清空
	cache.Update(events.Event{Err: errors.New("consul down")})

	endpoints, err := cache.Endpoints()
	if err != nil {
		t.Fatalf("should not return error when InvalidateOnError=false, got %v", err)
	}
	if len(endpoints) != 1 {
		t.Errorf("expected 1 endpoint after error (no-invalidate), got %d", len(endpoints))
	}
}

func TestEndpointCache_ErrorEvent_WithInvalidate(t *testing.T) {
	logger, _ := kitlog.NewDevelopment()
	factory := makeFactory(map[string]Endpoint{"h:1": Nop})
	timeout := 50 * time.Millisecond

	cache := NewEndpointCache(factory, logger, EndpointerOptions{
		InvalidateOnError: true,
		InvalidateTimeout: timeout,
	})
	cache.Update(events.Event{Instances: []string{"h:1"}})

	// 发送错误事件
	cache.Update(events.Event{Err: errors.New("sd error")})

	// deadline 到期前仍可获取端点
	endpoints, err := cache.Endpoints()
	if err != nil {
		t.Fatalf("before deadline: want no error, got %v", err)
	}
	if len(endpoints) == 0 {
		t.Error("before deadline: expected at least 1 endpoint")
	}

	// 等 deadline 过期
	time.Sleep(timeout + 20*time.Millisecond)

	endpoints, err = cache.Endpoints()
	if err == nil {
		t.Error("after deadline: expected error due to InvalidateOnError, got nil")
	}
	if len(endpoints) != 0 {
		t.Errorf("after deadline: expected 0 endpoints, got %d", len(endpoints))
	}
}

func TestEndpointCache_EmptyUpdate(t *testing.T) {
	logger, _ := kitlog.NewDevelopment()
	factory := makeFactory(map[string]Endpoint{"h:1": Nop})

	cache := NewEndpointCache(factory, logger, EndpointerOptions{})
	cache.Update(events.Event{Instances: []string{"h:1"}})
	cache.Update(events.Event{Instances: []string{}}) // clear

	endpoints, err := cache.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints after empty update, got %d", len(endpoints))
	}
}
