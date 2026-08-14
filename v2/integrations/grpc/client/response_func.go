package client

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// ResponseFunc is called after a successful gRPC response is received.
// header and trailer contain the response metadata.
type ResponseFunc func(ctx context.Context, header metadata.MD, trailer metadata.MD) context.Context
