package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
)

// ─────────────────────────── AllowerFunc adapter ───────────────────────────

func TestAllowerFunc_Allow(t *testing.T) {
	called := false
	f := ratelimit.AllowerFunc(func() bool {
		called = true
		return true
	})
	if !f.Allow() {
		t.Error("AllowerFunc should return true")
	}
	if !called {
		t.Error("AllowerFunc underlying function should have been called")
	}
}

func TestAllowerFunc_Deny(t *testing.T) {
	f := ratelimit.AllowerFunc(func() bool { return false })
	if f.Allow() {
		t.Error("AllowerFunc should return false")
	}
}

// ─────────────────────────── WaiterFunc adapter ───────────────────────────

func TestWaiterFunc_Wait(t *testing.T) {
	called := false
	f := ratelimit.WaiterFunc(func(ctx context.Context) error {
		called = true
		return nil
	})
	if err := f.Wait(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("WaiterFunc underlying function should have been called")
	}
}

// ─────────────────────────── NewErroringLimiter ───────────────────────────

func TestErroringLimiter_AllowsWithBurst(t *testing.T) {
	// burst=3, rate=0 → 允许前 3 次，之后拒绝
	lim := rate.NewLimiter(0, 3)
	ep := ratelimit.NewErroringLimiter(lim)(nopEndpoint)

	for i := 0; i < 3; i++ {
		if _, err := ep(context.Background(), nil); err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
	}
	// 第 4 次应该被限流
	_, err := ep(context.Background(), nil)
	if err != ratelimit.ErrLimited {
		t.Errorf("want ErrLimited, got %v", err)
	}
}

func TestErroringLimiter_ErrLimitedValue(t *testing.T) {
	// ErrLimited 应是一个固定值，方便调用方 errors.Is 比对
	if ratelimit.ErrLimited == nil {
		t.Fatal("ErrLimited should not be nil")
	}
	if ratelimit.ErrLimited.Error() != "rate limit exceeded" {
		t.Errorf("unexpected ErrLimited message: %q", ratelimit.ErrLimited.Error())
	}
}

// ─────────────────────────── NewDelayingLimiter ───────────────────────────

func TestDelayingLimiter_AllowsWithBurst(t *testing.T) {
	// burst=2 的令牌桶，前 2 次不需等待
	lim := rate.NewLimiter(rate.Every(time.Hour), 2)
	ep := ratelimit.NewDelayingLimiter(lim)(nopEndpoint)

	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, err := ep(ctx, nil)
		cancel()
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
	}
}

func TestDelayingLimiter_BlocksAndTimesOut(t *testing.T) {
	// burst=0，每次请求都需要等待很长时间 → 超时
	lim := rate.NewLimiter(rate.Every(time.Hour), 0)
	ep := ratelimit.NewDelayingLimiter(lim)(nopEndpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := ep(ctx, nil)
	if err == nil {
		t.Fatal("expected timeout/deadline error, got nil")
	}
}
