package retry_test

import (
	"context"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/retry"
)

// benchBalancer picks a trivial endpoint, so the measurement reflects the retry
// machinery rather than a transport.
type benchBalancer struct{}

func (benchBalancer) Pick(_ context.Context, _ any) (sd.Picked, error) {
	return sd.Picked{Endpoint: endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	})}, nil
}
func (benchBalancer) Close() error { return nil }

// BenchmarkRetrySingleAttempt measures the default one-attempt configuration, the
// path that used to pay for a goroutine and a buffered channel on every call even
// though it could never retry. The fast path runs the attempt inline.
func BenchmarkRetrySingleAttempt(b *testing.B) {
	ep := retry.New(benchBalancer{})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ep(ctx, nil); err != nil {
			b.Fatal(err)
		}
	}
}
