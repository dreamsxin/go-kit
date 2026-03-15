package circuitbreaker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sony/gobreaker"

	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
)

// ─────────────────────────── Gobreaker: AllowRequests / HalfOpen ───────────────

// TestGobreaker_PassThroughWhenClosed 验证熔断器在关闭状态下正常放行请求
func TestGobreaker_PassThroughWhenClosed(t *testing.T) {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "test-pass"})
	ep := circuitbreaker.Gobreaker(cb)(func(ctx context.Context, req interface{}) (interface{}, error) {
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

// TestGobreaker_ReturnsEndpointError 验证业务错误（非熔断器错误）能正确透传
func TestGobreaker_ReturnsEndpointError(t *testing.T) {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "test-err"})
	want := errors.New("business error")
	ep := circuitbreaker.Gobreaker(cb)(func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, want
	})
	_, err := ep(context.Background(), nil)
	if !errors.Is(err, want) {
		t.Errorf("want %v, got %v", want, err)
	}
}

// TestGobreaker_OpenAfterThreshold 验证达到阈值后熔断器打开
func TestGobreaker_OpenAfterThreshold(t *testing.T) {
	// MinimumRequests=5, ConsecutiveFailures=5 → 连续 5 次失败后打开
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "thresh-test",
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 3 },
	})
	boom := errors.New("fail")
	ep := circuitbreaker.Gobreaker(cb)(func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, boom
	})

	// 触发 3 次失败
	for i := 0; i < 3; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}

	// 现在熔断器应该打开，后续请求直接被拒绝（error != boom）
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Fatal("expected open-circuit error, got nil")
	}
	if errors.Is(err, boom) {
		t.Errorf("expected circuit-open error, but got original endpoint error: %v", err)
	}
}
