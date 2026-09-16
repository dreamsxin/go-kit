package server

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// RequestFunc mutates a gRPC call's context and metadata before it is handled.
type RequestFunc func(context.Context, metadata.MD) context.Context
