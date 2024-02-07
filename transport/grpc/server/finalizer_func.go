package server

import (
	"context"
)

// ServerFinalizerFunc can be used to perform work at the end of an gRPC
// request, after the response has been written to the client.
type FinalizerFunc func(ctx context.Context, err error)
