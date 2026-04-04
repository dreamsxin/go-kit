package endpoint_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
)

func TestBackpressureMiddleware_AllowsUnderLimit(t *testing.T) {
	ep := endpoint.BackpressureMiddleware(5)(endpoint.Nop)
	for i := 0; i < 5; i++ {
		if _, err := ep(context.Background(), nil); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
}

func TestBackpressureMiddleware_RejectsOverLimit(t *testing.T) {
	// slow endpoint so we can saturate the limit
	ready := make(chan struct{})
	slow := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
		<-ready
		return nil, nil
	})

	ep := endpoint.BackpressureMiddleware(2)(slow)

	var wg sync.WaitGroup
	errs := make(chan error, 10)

	// launch 4 concurrent calls — 2 should be rejected
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ep(context.Background(), nil)
			if err != nil {
				errs <- err
			}
		}()
	}

	time.Sleep(20 * time.Millisecond) // let goroutines start
	close(ready)                       // unblock slow endpoint
	wg.Wait()
	close(errs)

	rejected := 0
	for err := range errs {
		if errors.Is(err, endpoint.ErrBackpressure) {
			rejected++
		}
	}
	if rejected == 0 {
		t.Error("expected at least one ErrBackpressure, got none")
	}
}

func TestInFlightMiddleware_TracksCount(t *testing.T) {
	var inflight int64
	ready := make(chan struct{})
	slow := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		<-ready
		return nil, nil
	})

	ep := endpoint.InFlightMiddleware(10, &inflight)(slow)

	done := make(chan struct{})
	go func() {
		ep(context.Background(), nil) //nolint:errcheck
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	if inflight != 1 {
		t.Errorf("inflight: want 1, got %d", inflight)
	}
	close(ready)
	<-done
	if inflight != 0 {
		t.Errorf("inflight after done: want 0, got %d", inflight)
	}
}
