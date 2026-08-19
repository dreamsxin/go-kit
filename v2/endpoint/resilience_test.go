package endpoint_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

func TestFallback_UsedOnlyOnError(t *testing.T) {
	primaryCalls := 0
	primary := func(context.Context, any) (any, error) {
		primaryCalls++
		return nil, errors.New("dependency down")
	}
	fallbackCalls := 0
	fallback := func(context.Context, any) (any, error) {
		fallbackCalls++
		return "default", nil
	}

	ep := endpoint.NewBuilder(primary).WithFallback(fallback).Build()

	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("fallback should absorb the error: %v", err)
	}
	if resp != "default" {
		t.Errorf("response: got %v, want default", resp)
	}
	if primaryCalls != 1 || fallbackCalls != 1 {
		t.Errorf("calls: primary=%d fallback=%d", primaryCalls, fallbackCalls)
	}

	// A successful primary never consults the fallback.
	okPrimary := func(context.Context, any) (any, error) { return "primary", nil }
	ep = endpoint.NewBuilder(okPrimary).WithFallback(fallback).Build()
	resp, err = ep(context.Background(), nil)
	if err != nil || resp != "primary" {
		t.Fatalf("primary success should win: %v %v", resp, err)
	}
	if fallbackCalls != 1 {
		t.Errorf("fallback should not run on success, calls=%d", fallbackCalls)
	}
}

func TestFallback_ReceivesSameRequest(t *testing.T) {
	var gotRequest string
	ep := endpoint.NewBuilder(
		func(context.Context, any) (any, error) { return nil, errors.New("down") },
	).WithFallback(func(_ context.Context, req any) (any, error) {
		gotRequest = req.(string)
		return "fallback", nil
	}).Build()

	if _, err := ep(context.Background(), "tenant-a"); err != nil {
		t.Fatal(err)
	}
	if gotRequest != "tenant-a" {
		t.Errorf("fallback request: got %q", gotRequest)
	}
}

func keyedRequest(key string) any { return struct{ Key string }{Key: key} }

func keyFunc(req any) string { return req.(struct{ Key string }).Key }

func TestBulkhead_IsolatesKeys(t *testing.T) {
	releaseA := make(chan struct{})
	var inflightA, rejectedB int64

	ep := endpoint.BulkheadMiddleware(1, keyFunc)(
		func(ctx context.Context, req any) (any, error) {
			if keyFunc(req) == "a" {
				atomic.AddInt64(&inflightA, 1)
				<-releaseA
				atomic.AddInt64(&inflightA, -1)
			}
			return "ok", nil
		})

	// Occupy key "a" until released.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = ep(context.Background(), keyedRequest("a"))
	}()
	waitFor(t, func() bool { return atomic.LoadInt64(&inflightA) == 1 })

	// Key "b" is unaffected by "a" being full.
	if _, err := ep(context.Background(), keyedRequest("b")); err != nil {
		t.Fatalf("key b should pass while key a is busy: %v", err)
	}
	if atomic.LoadInt64(&rejectedB) != 0 {
		t.Fatal("no b request should have been rejected")
	}
	close(releaseA)
	<-done
}

func TestBulkhead_RejectsWhenKeyFull(t *testing.T) {
	release := make(chan struct{})
	var busy int64
	ep := endpoint.BulkheadMiddleware(1, keyFunc)(
		func(context.Context, any) (any, error) {
			atomic.AddInt64(&busy, 1)
			<-release
			atomic.AddInt64(&busy, -1)
			return "ok", nil
		})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = ep(context.Background(), keyedRequest("a"))
	}()
	waitFor(t, func() bool { return atomic.LoadInt64(&busy) == 1 })

	// Second request on the same key waits for a bounded context and fails
	// with ErrBulkheadFull.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := ep(ctx, keyedRequest("a"))
	if !errors.Is(err, endpoint.ErrBulkheadFull) {
		t.Fatalf("error: want ErrBulkheadFull, got %v", err)
	}

	close(release)
	<-done

	// The freed slot serves the next request immediately.
	if _, err := ep(context.Background(), keyedRequest("a")); err != nil {
		t.Fatalf("slot should be free after release: %v", err)
	}
}

func TestBulkhead_NilKeySharesOnePool(t *testing.T) {
	release := make(chan struct{})
	var calls int64
	ep := endpoint.BulkheadMiddleware(1, nil)(
		func(context.Context, any) (any, error) {
			atomic.AddInt64(&calls, 1)
			<-release
			return "ok", nil
		})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = ep(context.Background(), keyedRequest("a"))
	}()
	waitFor(t, func() bool { return atomic.LoadInt64(&calls) == 1 })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// A different key still shares the single pool.
	_, err := ep(ctx, keyedRequest("b"))
	if !errors.Is(err, endpoint.ErrBulkheadFull) {
		t.Fatalf("nil key should share one pool, got %v", err)
	}

	close(release)
	<-done
}

func TestBulkhead_ConcurrentAccess(t *testing.T) {
	ep := endpoint.BulkheadMiddleware(4, keyFunc)(
		func(context.Context, any) (any, error) { return "ok", nil })

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ep(context.Background(), keyedRequest("shared")); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within deadline")
}
