package client

import "context"

// FinalizerFunc is called at the end of every gRPC client call, regardless
// of success or failure.
type FinalizerFunc func(ctx context.Context, err error)
