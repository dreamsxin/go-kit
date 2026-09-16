package server

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// ResponseFunc observes a gRPC call's response headers and trailers, returning
// a possibly enriched context.
type ResponseFunc func(ctx context.Context, header *metadata.MD, trailer *metadata.MD) context.Context
