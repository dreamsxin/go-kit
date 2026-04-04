package circuitbreaker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
)

func TestHystrix_ClosedPassesThrough(t *testing.T) {
	circuitbreaker.HystrixConfigureCommand("test-pass", circuitbreaker.HystrixConfig{
		Timeout:                time.Second,
		MaxConcurrentRequests:  100,
		RequestVolumeThreshold: 20,
		ErrorPercentThreshold:  50,
	})
	ep := circuitbreaker.Hystrix("test-pass")(func(_ context.Context, _ interface{}) (interface{}, error) {
		return "ok", nil
	})
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("want 'ok', got %v", resp)
	}
}

func TestHystrix_OpensAfterErrorThreshold(t *testing.T) {
	const cmd = "test-open"
	circuitbreaker.HystrixConfigureCommand(cmd, circuitbreaker.HystrixConfig{
		Timeout:                500 * time.Millisecond,
		MaxConcurrentRequests:  100,
		RequestVolumeThreshold: 5,  // open after 5 requests
		ErrorPercentThreshold:  50, // if ≥50% fail
		SleepWindow:            10 * time.Second,
	})

	boom := errors.New("fail")
	ep := circuitbreaker.Hystrix(cmd)(func(_ context.Context, _ interface{}) (interface{}, error) {
		return nil, boom
	})

	// Send enough failures to trip the circuit
	for i := 0; i < 10; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}

	// Circuit should now be open
	_, err := ep(context.Background(), nil)
	if !errors.Is(err, circuitbreaker.ErrHystrixCircuitOpen) {
		t.Errorf("want ErrHystrixCircuitOpen, got %v", err)
	}
}

func TestHystrix_Timeout(t *testing.T) {
	const cmd = "test-timeout"
	circuitbreaker.HystrixConfigureCommand(cmd, circuitbreaker.HystrixConfig{
		Timeout:                50 * time.Millisecond,
		MaxConcurrentRequests:  100,
		RequestVolumeThreshold: 100, // high threshold so circuit stays closed
		ErrorPercentThreshold:  100,
	})

	ep := circuitbreaker.Hystrix(cmd)(func(_ context.Context, _ interface{}) (interface{}, error) {
		time.Sleep(200 * time.Millisecond)
		return "late", nil
	})

	_, err := ep(context.Background(), nil)
	if !errors.Is(err, circuitbreaker.ErrHystrixTimeout) {
		t.Errorf("want ErrHystrixTimeout, got %v", err)
	}
}

func TestHystrix_MaxConcurrency(t *testing.T) {
	const cmd = "test-concurrency"
	circuitbreaker.HystrixConfigureCommand(cmd, circuitbreaker.HystrixConfig{
		Timeout:                time.Second,
		MaxConcurrentRequests:  1, // only 1 at a time
		RequestVolumeThreshold: 100,
		ErrorPercentThreshold:  100,
	})

	block := make(chan struct{})
	ep := circuitbreaker.Hystrix(cmd)(func(_ context.Context, _ interface{}) (interface{}, error) {
		<-block
		return nil, nil
	})

	// First call blocks
	done := make(chan struct{})
	go func() {
		ep(context.Background(), nil) //nolint:errcheck
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)

	// Second call should be rejected
	_, err := ep(context.Background(), nil)
	if !errors.Is(err, circuitbreaker.ErrHystrixMaxConcurrency) {
		t.Errorf("want ErrHystrixMaxConcurrency, got %v", err)
	}

	close(block)
	<-done
}

func TestHystrix_HalfOpenRecovery(t *testing.T) {
	const cmd = "test-halfopen"
	circuitbreaker.HystrixConfigureCommand(cmd, circuitbreaker.HystrixConfig{
		Timeout:                500 * time.Millisecond,
		MaxConcurrentRequests:  100,
		RequestVolumeThreshold: 3,
		ErrorPercentThreshold:  50,
		SleepWindow:            50 * time.Millisecond, // short for test
	})

	fail := true
	ep := circuitbreaker.Hystrix(cmd)(func(_ context.Context, _ interface{}) (interface{}, error) {
		if fail {
			return nil, errors.New("fail")
		}
		return "recovered", nil
	})

	// Trip the circuit
	for i := 0; i < 6; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}
	if _, err := ep(context.Background(), nil); !errors.Is(err, circuitbreaker.ErrHystrixCircuitOpen) {
		t.Fatalf("circuit should be open, got %v", err)
	}

	// Wait for sleep window → half-open
	time.Sleep(80 * time.Millisecond)
	fail = false

	// Probe request should succeed and close the circuit
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("probe request failed: %v", err)
	}
	if resp != "recovered" {
		t.Errorf("want 'recovered', got %v", resp)
	}

	// Circuit should now be closed again
	resp2, err2 := ep(context.Background(), nil)
	if err2 != nil {
		t.Fatalf("post-recovery request failed: %v", err2)
	}
	if resp2 != "recovered" {
		t.Errorf("want 'recovered', got %v", resp2)
	}
}

func TestHystrix_ContextCancellation(t *testing.T) {
	const cmd = "test-ctx-cancel"
	circuitbreaker.HystrixConfigureCommand(cmd, circuitbreaker.HystrixConfig{
		Timeout:                5 * time.Second,
		MaxConcurrentRequests:  100,
		RequestVolumeThreshold: 100,
		ErrorPercentThreshold:  100,
	})

	ep := circuitbreaker.Hystrix(cmd)(func(ctx context.Context, _ interface{}) (interface{}, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := ep(ctx, nil)
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

// Note: the testFailingEndpoint helper is used by Gobreaker and HandyBreaker tests.
// Hystrix uses a different (time-window based) algorithm so the helper's
// shouldPass logic does not apply directly.
