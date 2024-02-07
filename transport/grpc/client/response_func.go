package client

import (
	"context"

	"google.golang.org/grpc/metadata"
)

type ResponseFunc func(ctx context.Context, header metadata.MD, trailer metadata.MD) context.Context
