package server

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// RequestFunc reads a gRPC call's incoming metadata and returns the context the
// rest of the call uses. Only that context carries forward: the metadata is the
// request's own, and writing to it changes nothing the handler or the wire sees.
type RequestFunc func(context.Context, metadata.MD) context.Context
