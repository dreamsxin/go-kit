// Package main demonstrates every endpoint middleware in the framework:
//
//   - endpoint.Chain          — compose middlewares in declaration order
//   - endpoint.Builder        — fluent alternative to Chain
//   - endpoint.Failer         — carry business errors in the response value
//   - endpoint.TimeoutMiddleware
//   - endpoint.MetricsMiddleware
//   - endpoint.ErrorHandlingMiddleware
//   - circuitbreaker.Gobreaker   (sony/gobreaker)
//   - circuitbreaker.HandyBreaker (streadway/handy)
//   - ratelimit.NewErroringLimiter  — reject immediately when over limit
//   - ratelimit.NewDelayingLimiter  — wait for a token (respects ctx deadline)
//
// Run:
//
//	go run ./examples/middleware
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker"
	handybreaker "github.com/streadway/handy/breaker"
	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
)

// ── Failer response ───────────────────────────────────────────────────────────

// calcResponse carries either a result or a business-logic error.
// Implementing endpoint.Failer lets the transport layer detect failures
// without relying on the Go error return value.
type calcResponse struct {
	Result float64
	Err    error
}

// Failed implements endpoint.Failer.
func (r calcResponse) Failed() error { return r.Err }

// ── Business logic ────────────────────────────────────────────────────────────

func divide(_ context.Context, a, b float64) calcResponse {
	if b == 0 {
		return calcResponse{Err: errors.New("division by zero")}
	}
	return calcResponse{Result: a / b}
}

// ── Endpoint ──────────────────────────────────────────────────────────────────

type divReq struct{ A, B float64 }

// divideEndpoint is a plain (untyped) Endpoint — request/response are any.
var divideEndpoint = endpoint.Endpoint(func(ctx context.Context, req any) (any, error) {
	r := req.(divReq)
	return divide(ctx, r.A, r.B), nil
})

// divideTyped is a TypedEndpoint — no type assertions needed at the call site.
var divideTyped = endpoint.TypedEndpoint[divReq, calcResponse](
	func(ctx context.Context, req divReq) (calcResponse, error) {
		return divide(ctx, req.A, req.B), nil
	},
)

// ── Demo helpers ──────────────────────────────────────────────────────────────

func call(ep endpoint.Endpoint, req divReq) {
	resp, err := ep(context.Background(), req)
	if err != nil {
		fmt.Printf("  endpoint error: %v\n", err)
		return
	}
	cr := resp.(calcResponse)
	if cr.Failed() != nil {
		fmt.Printf("  business error (Failer): %v\n", cr.Failed())
		return
	}
	fmt.Printf("  result: %.2f\n", cr.Result)
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	// ── 1. Chain ──────────────────────────────────────────────────────────────
	fmt.Println("=== 1. endpoint.Chain ===")
	var order []string
	tag := func(name string) endpoint.Middleware {
		return func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, req any) (any, error) {
				order = append(order, name+":pre")
				resp, err := next(ctx, req)
				order = append(order, name+":post")
				return resp, err
			}
		}
	}
	chained := endpoint.Chain(tag("A"), tag("B"), tag("C"))(endpoint.Nop)
	chained(context.Background(), nil) //nolint:errcheck
	fmt.Printf("  execution order: %v\n", order)
	// Output: [A:pre B:pre C:pre C:post B:post A:post]

	// ── 2. Builder ────────────────────────────────────────────────────────────
	fmt.Println("\n=== 2. endpoint.Builder ===")
	var m endpoint.Metrics
	ep := endpoint.NewBuilder(divideEndpoint).
		WithMetrics(&m).
		WithErrorHandling("divide").
		Use(endpoint.TimeoutMiddleware(2 * time.Second)).
		Build()

	call(ep, divReq{10, 2})  // success
	call(ep, divReq{5, 0})   // Failer (not an endpoint error)
	fmt.Printf("  metrics: requests=%d success=%d errors=%d\n",
		m.RequestCount, m.SuccessCount, m.ErrorCount)

	// ── 2b. TypedEndpoint — no type assertions ────────────────────────────────
	fmt.Println("\n=== 2b. TypedEndpoint (compile-time type safety) ===")
	typedEp := endpoint.Unwrap[divReq, calcResponse](
		endpoint.NewTypedBuilder(divideTyped).
			WithTimeout(2 * time.Second).
			Build(),
	)
	r, err2 := typedEp(context.Background(), divReq{9, 3})
	if err2 != nil {
		fmt.Printf("  error: %v\n", err2)
	} else {
		fmt.Printf("  result: %.2f (no type assertion needed)\n", r.Result)
	}

	// ── 3. Failer ─────────────────────────────────────────────────────────────
	fmt.Println("\n=== 3. endpoint.Failer ===")
	resp, _ := divideEndpoint(context.Background(), divReq{0, 0})
	if f, ok := resp.(endpoint.Failer); ok && f.Failed() != nil {
		fmt.Printf("  Failer detected: %v\n", f.Failed())
	}

	// ── 4. ErrorHandlingMiddleware ────────────────────────────────────────────
	fmt.Println("\n=== 4. ErrorHandlingMiddleware ===")
	failEp := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("something went wrong")
	})
	wrapped := endpoint.ErrorHandlingMiddleware("myOp")(failEp)
	_, err := wrapped(context.Background(), nil)
	var ew *endpoint.ErrorWrapper
	if errors.As(err, &ew) {
		fmt.Printf("  operation=%q cause=%v\n", ew.Operation, ew.Err)
	}

	// ── 5. TimeoutMiddleware ──────────────────────────────────────────────────
	fmt.Println("\n=== 5. TimeoutMiddleware ===")
	slowEp := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
		select {
		case <-time.After(5 * time.Second):
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	timedEp := endpoint.TimeoutMiddleware(50 * time.Millisecond)(slowEp)
	_, err = timedEp(context.Background(), nil)
	fmt.Printf("  timeout triggered: %v\n", err)

	// ── 6. Gobreaker ──────────────────────────────────────────────────────────
	fmt.Println("\n=== 6. circuitbreaker.Gobreaker ===")
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "demo",
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= 3 },
	})
	alwaysFail := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("backend down")
	})
	gbEp := circuitbreaker.Gobreaker(cb)(alwaysFail)
	for i := 0; i < 5; i++ {
		_, err := gbEp(context.Background(), nil)
		fmt.Printf("  call %d: %v\n", i+1, err)
	}

	// ── 7. HandyBreaker ───────────────────────────────────────────────────────
	fmt.Println("\n=== 7. circuitbreaker.HandyBreaker ===")
	hb := circuitbreaker.HandyBreaker(handybreaker.NewBreaker(0.5))
	for i := 0; i < 4; i++ {
		_, err := hb(alwaysFail)(context.Background(), nil)
		fmt.Printf("  call %d: %v\n", i+1, err)
	}

	// ── 8. ErroringLimiter ────────────────────────────────────────────────────
	fmt.Println("\n=== 8. ratelimit.NewErroringLimiter ===")
	// burst=2: first 2 succeed, then rejected immediately
	lim := rate.NewLimiter(0, 2)
	errLimEp := ratelimit.NewErroringLimiter(lim)(endpoint.Nop)
	for i := 0; i < 4; i++ {
		_, err := errLimEp(context.Background(), nil)
		if err != nil {
			fmt.Printf("  call %d: rejected — %v\n", i+1, err)
		} else {
			fmt.Printf("  call %d: allowed\n", i+1)
		}
	}

	// ── 9. DelayingLimiter ────────────────────────────────────────────────────
	fmt.Println("\n=== 9. ratelimit.NewDelayingLimiter ===")
	// 1 token/second: first call instant, second must wait
	delayLim := rate.NewLimiter(rate.Every(time.Second), 1)
	delayEp := ratelimit.NewDelayingLimiter(delayLim)(endpoint.Nop)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	delayEp(ctx, nil) //nolint:errcheck — consumes the token
	_, err = delayEp(ctx, nil)
	fmt.Printf("  second call (ctx deadline): %v\n", err)

	fmt.Println("\nDone.")
}
