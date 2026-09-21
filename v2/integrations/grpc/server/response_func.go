package server

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// ResponseFunc populates the response headers and trailers a gRPC call will
// send, and returns a possibly enriched context. The server passes empty
// metadata for the hook to write into, then sends whatever it wrote.
type ResponseFunc func(ctx context.Context, header *metadata.MD, trailer *metadata.MD) context.Context
