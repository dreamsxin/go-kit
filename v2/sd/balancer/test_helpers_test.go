package balancer_test

import (
	"context"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
)

func pick(t *testing.T, lb sd.Balancer, request any) sd.Picked {
	t.Helper()
	selected, err := lb.Pick(context.Background(), request)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	return selected
}

func callPicked(t *testing.T, picked sd.Picked, request any) any {
	t.Helper()
	started := time.Now()
	response, err := picked.Endpoint(context.Background(), request)
	if picked.Done != nil {
		picked.Done(sd.Outcome{Err: err, Latency: time.Since(started)})
	}
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	return response
}

func selectAddress(t *testing.T, lb sd.Balancer) string {
	t.Helper()
	return callPicked(t, pick(t, lb, nil), nil).(string)
}
