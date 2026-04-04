package client

import (
	"context"
)

// FinalizerFunc is called at the end of every client call, regardless of
// success or failure.  Use it to record latency or release resources.
type FinalizerFunc func(context.Context, error)
