package server

import (
	"context"

	"google.golang.org/grpc/metadata"
)

type RequestFunc func(context.Context, metadata.MD) context.Context
