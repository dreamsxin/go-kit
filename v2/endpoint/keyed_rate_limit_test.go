package endpoint_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// keyedBuckets is a per-key allowance for the tests: each key gets its own
// budget, so exhausting one must not affect another.
type keyedBuckets struct {
	remaining map[string]int
	waited    []string
	after     map[string]time.Duration
	fallback  time.Duration
}

func (b *keyedBuckets) AllowKey(_ context.Context, key string) bool {
	if b.remaining[key] <= 0 {
		return false
	}
	b.remaining[key]--
	return true
}

func (b *keyedBuckets) WaitKey(_ context.Context, key string) error {
	b.waited = append(b.waited, key)
	return nil
}

func (b *keyedBuckets) RetryAfterForKey(key string) time.Duration {
	return b.after[key]
}

func (b *keyedBuckets) RetryAfter() time.Duration { return b.fallback }

func keyFromRequest(_ context.Context, request any) string {
	key, _ := request.(string)
	return key
}

func TestKeyedRateLimitMiddlewareLimitsEachKeySeparately(t *testing.T) {
	buckets := &keyedBuckets{remaining: map[string]int{"tenant-a": 1, "tenant-b": 1}}
	ep := endpoint.KeyedRateLimitMiddleware(buckets, keyFromRequest)(endpoint.Nop)

	if _, err := ep(context.Background(), "tenant-a"); err != nil {
		t.Fatalf("first call for tenant-a: %v", err)
	}
	if _, err := ep(context.Background(), "tenant-a"); !errors.Is(err, endpoint.ErrRateLimited) {
		t.Fatalf("second call for tenant-a: err = %v, want ErrRateLimited", err)
	}
	// The whole point: tenant-a spending its allowance must not reject tenant-b.
	if _, err := ep(context.Background(), "tenant-b"); err != nil {
		t.Fatalf("first call for tenant-b: %v", err)
	}
}

func TestKeyedRateLimitMiddlewareLimitsAnUnidentifiedCaller(t *testing.T) {
	buckets := &keyedBuckets{remaining: map[string]int{"": 1}}
	ep := endpoint.KeyedRateLimitMiddleware(buckets, func(context.Context, any) string { return "" })(endpoint.Nop)

	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("first unidentified call: %v", err)
	}
	if _, err := ep(context.Background(), nil); !errors.Is(err, endpoint.ErrRateLimited) {
		t.Fatalf("second unidentified call: err = %v, want ErrRateLimited — an empty key is not an exemption", err)
	}

}

func TestKeyedRateLimitMiddlewarePrefersThePerKeyRetryHint(t *testing.T) {
	buckets := &keyedBuckets{
		remaining: map[string]int{},
		after:     map[string]time.Duration{"tenant-a": 3 * time.Second},
		fallback:  30 * time.Second,
	}
	ep := endpoint.KeyedRateLimitMiddleware(buckets, keyFromRequest)(endpoint.Nop)

	_, err := ep(context.Background(), "tenant-a")
	var reporter endpoint.RetryAfterReporter
	if !errors.As(err, &reporter) {
		t.Fatalf("err = %v, want a retry-after hint", err)
	}
	if got := reporter.RetryAfter(); got != 3*time.Second {
		t.Fatalf("RetryAfter = %v, want the per-key 3s rather than the 30s fallback", got)
	}
}

func TestKeyedRateLimitMiddlewareFallsBackToTheUnkeyedHint(t *testing.T) {
	buckets := &keyedBuckets{remaining: map[string]int{}, fallback: 30 * time.Second}
	ep := endpoint.KeyedRateLimitMiddleware(buckets, keyFromRequest)(endpoint.Nop)

	_, err := ep(context.Background(), "tenant-a")
	var reporter endpoint.RetryAfterReporter
	if !errors.As(err, &reporter) {
		t.Fatalf("err = %v, want a retry-after hint", err)
	}
	if got := reporter.RetryAfter(); got != 30*time.Second {
		t.Fatalf("RetryAfter = %v, want the unkeyed 30s", got)
	}
}

func TestDelayKeyedRateLimitMiddlewareWaitsUnderTheRequestKey(t *testing.T) {
	buckets := &keyedBuckets{remaining: map[string]int{}}
	ep := endpoint.DelayKeyedRateLimitMiddleware(buckets, keyFromRequest)(endpoint.Nop)

	if _, err := ep(context.Background(), "tenant-a"); err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(buckets.waited) != 1 || buckets.waited[0] != "tenant-a" {
		t.Fatalf("waited on %v, want [tenant-a]", buckets.waited)
	}
}

func TestDelayKeyedRateLimitMiddlewareReturnsTheWaitError(t *testing.T) {
	limiter := endpoint.KeyedRateLimiterFuncs{
		WaitKeyFn: func(ctx context.Context, _ string) error { return context.Canceled },
	}
	ep := endpoint.DelayKeyedRateLimitMiddleware(limiter, keyFromRequest)(endpoint.Nop)

	if _, err := ep(context.Background(), "tenant-a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestKeyedRateLimiterFuncsWithNilFieldsAllows(t *testing.T) {
	var limiter endpoint.KeyedRateLimiterFuncs
	if !limiter.AllowKey(context.Background(), "k") {
		t.Fatal("AllowKey with a nil field should allow")
	}
	if err := limiter.WaitKey(context.Background(), "k"); err != nil {
		t.Fatalf("WaitKey with a nil field: %v", err)
	}
}

func TestKeyedRateLimitMiddlewarePanicsOnMisassembly(t *testing.T) {
	cases := map[string]func(){
		"nil limiter": func() { endpoint.KeyedRateLimitMiddleware(nil, keyFromRequest) },
		"nil key":     func() { endpoint.KeyedRateLimitMiddleware(endpoint.KeyedRateLimiterFuncs{}, nil) },
		"nil limiter delayed": func() {
			endpoint.DelayKeyedRateLimitMiddleware(nil, keyFromRequest)
		},
		"nil key delayed": func() {
			endpoint.DelayKeyedRateLimitMiddleware(endpoint.KeyedRateLimiterFuncs{}, nil)
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected a panic at assembly")
				}
			}()
			build()
		})
	}
}

func TestBuilderInstallsTheKeyedRateLimit(t *testing.T) {
	buckets := &keyedBuckets{remaining: map[string]int{"tenant-a": 1}}
	ep := endpoint.NewBuilder(endpoint.Nop).
		WithKeyedRateLimit(buckets, keyFromRequest).
		Build()

	if _, err := ep(context.Background(), "tenant-a"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := ep(context.Background(), "tenant-a"); !errors.Is(err, endpoint.ErrRateLimited) {
		t.Fatalf("second call: err = %v, want ErrRateLimited", err)
	}
}
