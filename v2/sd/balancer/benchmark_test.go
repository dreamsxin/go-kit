package balancer_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/feedback"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
)

// BenchmarkPick measures one client-side request's share of discovery: the
// snapshot read, the strategy, and the Done bookkeeping.

func benchEndpointer(b *testing.B, addresses ...string) endpointer.InstanceEndpointer {
	b.Helper()
	cache := instance.NewCache()
	factory := endpointer.Factory(func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		return endpoint.Nop, io.NopCloser(nil), nil
	})
	ep := endpointer.NewEndpointer(cache, factory, slog.New(slog.DiscardHandler))
	b.Cleanup(func() { _ = ep.Close() })
	if len(addresses) > 0 {
		cache.Update(sd.Event{Instances: sd.Addresses(addresses...)})
		waitForEndpoints(b, ep, len(addresses))
	}
	return ep
}

func waitForEndpoints(b *testing.B, source endpointer.InstanceEndpointer, want int) {
	b.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		instances, err := source.InstanceEndpoints()
		if err == nil && len(instances) == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	b.Fatalf("endpointer did not converge on %d instances", want)
}

func benchAddresses(count int) []string {
	addresses := make([]string, count)
	for i := range addresses {
		addresses[i] = "10.0.0." + string(rune('1'+i%9)) + ":8080"
	}
	return addresses
}

func BenchmarkPickRoundRobin(b *testing.B) {
	for _, size := range []int{1, 9} {
		b.Run(instanceCountLabel(size), func(b *testing.B) {
			lb := balancer.NewRoundRobin(benchEndpointer(b, benchAddresses(size)...))
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				picked, err := lb.Pick(ctx, nil)
				if err != nil {
					b.Fatal(err)
				}
				picked.Done(sd.Outcome{})
			}
		})
	}
}

// BenchmarkPickRoundRobinConcurrent reports whether the snapshot read contends.
func BenchmarkPickRoundRobinConcurrent(b *testing.B) {
	lb := balancer.NewRoundRobin(benchEndpointer(b, benchAddresses(9)...))
	ctx := context.Background()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			picked, err := lb.Pick(ctx, nil)
			if err != nil {
				b.Error(err)
				return
			}
			picked.Done(sd.Outcome{})
		}
	})
}

// BenchmarkPickLeastRequest is the measurement-driven assembly: the strategy
// consults the feedback table once per candidate.
func BenchmarkPickLeastRequest(b *testing.B) {
	table := feedback.NewTable()
	lb := balancer.New(benchEndpointer(b, benchAddresses(9)...), table.LeastRequest())
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		picked, err := lb.Pick(ctx, nil)
		if err != nil {
			b.Fatal(err)
		}
		picked.Done(sd.Outcome{Latency: time.Millisecond})
	}
}

func BenchmarkPickLeastRequestConcurrent(b *testing.B) {
	table := feedback.NewTable()
	lb := balancer.New(benchEndpointer(b, benchAddresses(9)...), table.LeastRequest())
	ctx := context.Background()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			picked, err := lb.Pick(ctx, nil)
			if err != nil {
				b.Error(err)
				return
			}
			picked.Done(sd.Outcome{Latency: time.Millisecond})
		}
	})
}

func instanceCountLabel(size int) string {
	if size == 1 {
		return "one-instance"
	}
	return "many-instances"
}
