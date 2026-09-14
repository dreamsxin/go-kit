package client_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	sdclient "github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"github.com/dreamsxin/go-kit/v2/sd/retry"
)

type retryClock struct {
	*endpoint.ManualClock
	waits chan time.Duration
}

func newRetryClock() *retryClock {
	return &retryClock{endpoint.NewManualClock(time.Unix(0, 0)), make(chan time.Duration, 8)}
}

func (c *retryClock) NewTimer(d time.Duration) endpoint.Timer {
	timer := c.ManualClock.NewTimer(d)
	c.waits <- d // Observe the wait only after it has been registered.
	return timer
}

func clientReceive[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case v := <-values:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for client progress")
		var zero T
		return zero
	}
}

type callResult struct {
	value any
	err   error
}

func TestNewEndpoint_ConfiguredRetryTiming(t *testing.T) {
	clock := newRetryClock()
	source := &trackedInstancer{Cache: instance.NewCache()}
	defer source.Close() //nolint:errcheck
	source.Update(sd.Event{Instances: sd.Addresses("old:80")})
	created := make(chan string, 2)
	var calls, closed atomic.Int32
	failure := errors.New("retry this backend")
	type contextKey struct{}
	factory := func(inst sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		created <- inst.Address
		return func(ctx context.Context, request any) (any, error) {
			calls.Add(1)
			if request != "payload" || ctx.Value(contextKey{}) != "trace" {
				return nil, errors.New("lost call input")
			}
			if inst.Address == "old:80" {
				return nil, failure
			}
			return inst.Address, nil
		}, closerFunc(func() error { closed.Add(1); return nil }), nil
	}
	call, resources, err := sdclient.NewEndpoint(source, factory, nil,
		sdclient.WithMaxAttempts(2), sdclient.WithTimeout(time.Minute),
		sdclient.WithRetryable(func(err error) bool { return errors.Is(err, failure) }),
		sdclient.WithRetryClock(clock),
		sdclient.WithRetryBackoff(func(attempt int) time.Duration { return time.Duration(attempt) * time.Hour }))
	if err != nil {
		t.Fatal(err)
	}
	defer resources.Close() //nolint:errcheck
	clientReceive(t, created)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "trace"))
	defer cancel()
	done := make(chan callResult, 1)
	go func() { value, err := call(ctx, "payload"); done <- callResult{value, err} }()
	if delay := clientReceive(t, clock.waits); delay != time.Hour {
		t.Fatalf("backoff=%v, want 1h", delay)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d before clock advance, want 1", calls.Load())
	}

	// The next retry must use the live discovery set, not the first picked endpoint.
	source.Update(sd.Event{Instances: sd.Addresses("new:80")})
	if address := clientReceive(t, created); address != "new:80" {
		t.Fatalf("new instance=%q", address)
	}
	clock.Advance(time.Hour)
	result := clientReceive(t, done)
	if result.err != nil || result.value != "new:80" || calls.Load() != 2 {
		t.Fatalf("result=%v error=%v calls=%d", result.value, result.err, calls.Load())
	}
	if clock.Pending() != 0 {
		t.Fatal("completed client retained its timer")
	}
	if err := resources.Close(); err != nil {
		t.Fatal(err)
	}
	if err := resources.Close(); err != nil {
		t.Fatal(err)
	}
	if closed.Load() != 2 || source.deregistered.Load() != 1 || source.closed.Load() {
		t.Fatalf("closed endpoints=%d unsubscribed=%d source closed=%v", closed.Load(), source.deregistered.Load(), source.closed.Load())
	}
	if _, err := call(context.Background(), "payload"); !errors.Is(err, sd.ErrClosed) {
		t.Fatalf("after close: %v", err)
	}
}

func TestNewEndpoint_RetryCancellationAndDeadline(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			clock := newRetryClock()
			source := instance.NewCache()
			defer source.Close() //nolint:errcheck
			source.Update(sd.Event{Instances: sd.Addresses("a:80")})
			var calls atomic.Int32
			call, resources, err := sdclient.NewEndpoint(source, func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
				return func(context.Context, any) (any, error) { calls.Add(1); return nil, sd.ErrNoEndpoints }, nil, nil
			}, nil, sdclient.WithMaxAttempts(3), sdclient.WithTimeout(time.Second),
				sdclient.WithRetryClock(clock), sdclient.WithRetryBackoff(func(int) time.Duration { return time.Hour }))
			if err != nil {
				t.Fatal(err)
			}
			defer resources.Close() //nolint:errcheck
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := call(ctx, nil); done <- err }()
			clientReceive(t, clock.waits)
			want := context.DeadlineExceeded
			if mode == "cancel" {
				cancel()
				want = context.Canceled
			}
			err = clientReceive(t, done)
			var history retry.Error
			if !errors.Is(err, want) || !errors.As(err, &history) || len(history.Attempts) != 1 {
				t.Fatalf("error=%v, want %v with one attempt", err, want)
			}
			if clock.Pending() != 0 || calls.Load() != 1 {
				t.Fatalf("pending=%d calls=%d", clock.Pending(), calls.Load())
			}
		})
	}
}

func TestNewEndpoint_RetryDefaultsAndPolicyRemainIntact(t *testing.T) {
	for _, tc := range []struct {
		name     string
		options  []sdclient.Option
		attempts int32
	}{
		{"defaults", nil, 1},
		{"attempt cap", []sdclient.Option{sdclient.WithMaxAttempts(3), sdclient.WithRetryBackoff(func(int) time.Duration { return 0 })}, 3},
		{"permanent", []sdclient.Option{sdclient.WithMaxAttempts(3), sdclient.WithRetryable(func(error) bool { return false })}, 1},
		{"nil restores defaults", []sdclient.Option{sdclient.WithMaxAttempts(2),
			sdclient.WithRetryClock(newRetryClock()), sdclient.WithRetryClock(nil),
			sdclient.WithRetryBackoff(func(int) time.Duration { panic("overwritten backoff must not run") }), sdclient.WithRetryBackoff(nil)}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := instance.NewCache()
			defer source.Close() //nolint:errcheck
			source.Update(sd.Event{Instances: sd.Addresses("a:80")})
			var calls atomic.Int32
			call, resources, err := sdclient.NewEndpoint(source, func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
				return func(context.Context, any) (any, error) { calls.Add(1); return nil, sd.ErrNoEndpoints }, nil, nil
			}, nil, tc.options...)
			if err != nil {
				t.Fatal(err)
			}
			defer resources.Close() //nolint:errcheck
			_, err = call(context.Background(), nil)
			if !errors.Is(err, sd.ErrNoEndpoints) || calls.Load() != tc.attempts {
				t.Fatalf("error=%v calls=%d want=%d", err, calls.Load(), tc.attempts)
			}
		})
	}
}

func TestNewEndpoint_RejectsTypedNilRetryClockBeforeSubscribing(t *testing.T) {
	source := &trackedInstancer{Cache: instance.NewCache()}
	defer source.Close() //nolint:errcheck
	call, resources, err := sdclient.NewEndpoint(source, nopFactory, nil,
		sdclient.WithRetryClock((*endpoint.ManualClock)(nil)))
	if call != nil || resources != nil || err == nil || source.registered.Load() != 0 {
		t.Fatalf("call=%v resources=%v error=%v registrations=%d", call != nil, resources, err, source.registered.Load())
	}
}

func ExampleWithRetryBackoff() {
	source := instance.NewCache()
	defer source.Close() //nolint:errcheck
	source.Update(sd.Event{Instances: sd.Addresses("local:80")})
	var attempts int
	call, resources, err := sdclient.NewEndpoint(source, func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		return func(context.Context, any) (any, error) {
			attempts++
			if attempts == 1 {
				return nil, sd.ErrNoEndpoints
			}
			return "ready", nil
		}, nil, nil
	}, nil, sdclient.WithMaxAttempts(2), sdclient.WithRetryBackoff(func(int) time.Duration { return 0 }))
	if err != nil {
		panic(err)
	}
	defer resources.Close() //nolint:errcheck
	value, err := call(context.Background(), nil)
	fmt.Println(value, err, attempts)
	// Output: ready <nil> 2
}
