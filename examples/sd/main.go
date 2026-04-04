// Package main demonstrates the service-discovery (sd) components
// without any external dependency (no Consul, no network):
//
//   - sd/instance.Cache        — in-memory Instancer for testing
//   - sd/endpointer            — wires Instancer → EndpointCache
//   - sd/endpointer/balancer   — lock-free RoundRobin
//   - sd/endpointer/executor   — Retry, RetryAlways, RetryWithCallback
//   - sd.NewEndpoint           — one-liner that wires everything together
//   - endpoint.InvalidateOnError — cache invalidation on SD errors
//
// Run:
//
//	go run ./examples/sd
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd"
	"github.com/dreamsxin/go-kit/sd/endpointer"
	"github.com/dreamsxin/go-kit/sd/endpointer/balancer"
	"github.com/dreamsxin/go-kit/sd/endpointer/executor"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/instance"
	"github.com/dreamsxin/go-kit/sd/interfaces"
)

// ── Factory helper ────────────────────────────────────────────────────────────

// instanceFactory returns an Endpoint that echoes the instance address.
func instanceFactory(addr string) (endpoint.Endpoint, io.Closer, error) {
	ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return addr, nil
	})
	return ep, io.NopCloser(nil), nil
}

var factory = endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
	return instanceFactory(addr)
})

// ── Demo 1: instance.Cache + Endpointer + RoundRobin ─────────────────────────

func demo1_RoundRobin(logger *kitlog.Logger) {
	fmt.Println("=== 1. instance.Cache + Endpointer + RoundRobin ===")

	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, factory, logger)
	lb := balancer.NewRoundRobin(ep)

	// No instances yet
	_, err := lb.Endpoint()
	fmt.Printf("  no instances: %v\n", err) // ErrNoEndpoints

	// Register two instances
	cache.Update(events.Event{Instances: []string{"host-A:8080", "host-B:8080"}})
	time.Sleep(10 * time.Millisecond) // let the goroutine process

	fmt.Println("  round-robin over 4 calls:")
	for i := 0; i < 4; i++ {
		e, _ := lb.Endpoint()
		resp, _ := e(context.Background(), nil)
		fmt.Printf("    call %d → %s\n", i+1, resp)
	}

	// Remove one instance
	cache.Update(events.Event{Instances: []string{"host-A:8080"}})
	time.Sleep(10 * time.Millisecond)

	e, _ := lb.Endpoint()
	resp, _ := e(context.Background(), nil)
	fmt.Printf("  after removing host-B: %s\n", resp)
}

// ── Demo 2: executor.Retry ────────────────────────────────────────────────────

func demo2_Retry(logger *kitlog.Logger) {
	fmt.Println("\n=== 2. executor.Retry (max 3 attempts) ===")

	attempts := 0
	flakyFactory := endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			attempts++
			if attempts < 3 {
				return nil, fmt.Errorf("attempt %d failed", attempts)
			}
			return fmt.Sprintf("success on attempt %d", attempts), nil
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, flakyFactory, logger)
	lb := balancer.NewRoundRobin(ep)
	retryEp := executor.Retry(5, time.Second, lb)

	resp, err := retryEp(context.Background(), nil)
	fmt.Printf("  result: %v, err: %v\n", resp, err)
}

// ── Demo 3: executor.RetryWithCallback ───────────────────────────────────────

func demo3_RetryWithCallback(logger *kitlog.Logger) {
	fmt.Println("\n=== 3. executor.RetryWithCallback ===")

	var sentinelErr = errors.New("non-retryable")
	callCount := 0

	flakyFactory := endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			callCount++
			switch callCount {
			case 1:
				return nil, errors.New("transient error")
			case 2:
				return nil, sentinelErr // non-retryable
			default:
				return "ok", nil
			}
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, flakyFactory, logger)
	lb := balancer.NewRoundRobin(ep)

	retryEp := executor.RetryWithCallback(time.Second, lb,
		func(n int, err error) (keepTrying bool, replacement error) {
			if errors.Is(err, sentinelErr) {
				fmt.Printf("  attempt %d: non-retryable error, stopping\n", n)
				return false, err
			}
			fmt.Printf("  attempt %d: transient error, retrying\n", n)
			return true, nil
		},
	)

	_, err := retryEp(context.Background(), nil)
	fmt.Printf("  final error: %v\n", err)
}

// ── Demo 4: sd.NewEndpoint (one-liner) ───────────────────────────────────────

func demo4_NewEndpoint(logger *kitlog.Logger) {
	fmt.Println("\n=== 4. sd.NewEndpoint (one-liner) ===")

	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc1:80", "svc2:80", "svc3:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := sd.NewEndpoint(cache, factory, logger,
		sd.WithMaxRetries(3),
		sd.WithTimeout(500*time.Millisecond),
	)

	fmt.Println("  5 calls via sd.NewEndpoint:")
	for i := 0; i < 5; i++ {
		resp, err := ep(context.Background(), nil)
		fmt.Printf("    call %d → %v (err=%v)\n", i+1, resp, err)
	}
}

// ── Demo 5: InvalidateOnError ─────────────────────────────────────────────────

func demo5_InvalidateOnError(logger *kitlog.Logger) {
	fmt.Println("\n=== 5. endpoint.InvalidateOnError ===")

	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, factory, logger,
		endpoint.InvalidateOnError(50*time.Millisecond),
	)
	lb := balancer.NewRoundRobin(ep)

	// Healthy call
	e, err := lb.Endpoint()
	fmt.Printf("  before error: endpoint=%v err=%v\n", e != nil, err)

	// Simulate SD error
	cache.Update(events.Event{Err: errors.New("consul down")})
	time.Sleep(10 * time.Millisecond)

	// Within grace period — still returns cached endpoints
	e, err = lb.Endpoint()
	fmt.Printf("  during grace period: endpoint=%v err=%v\n", e != nil, err)

	// After grace period — cache is cleared
	time.Sleep(80 * time.Millisecond)
	_, err = lb.Endpoint()
	fmt.Printf("  after grace period: err=%v\n", err)
	if errors.Is(err, interfaces.ErrNoEndpoints) || err != nil {
		fmt.Println("  cache invalidated as expected")
	}
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	logger := kitlog.NewNopLogger()

	demo1_RoundRobin(logger)
	demo2_Retry(logger)
	demo3_RetryWithCallback(logger)
	demo4_NewEndpoint(logger)
	demo5_InvalidateOnError(logger)

	fmt.Println("\nDone.")
}
