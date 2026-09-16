package server

import (
	"context"
)

// FinalizerFunc runs after a gRPC call finishes and observes the final error,
// if any. It always runs, whether the call succeeded or failed.
type FinalizerFunc func(ctx context.Context, err error)
