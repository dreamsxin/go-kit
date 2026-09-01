package client

import (
	"context"
)

// EncodeRequestFunc encodes a domain request value into a gRPC request proto.
type EncodeRequestFunc func(context.Context, any) (request any, err error)

// DecodeResponseFunc decodes a gRPC response proto into a domain response value.
type DecodeResponseFunc func(context.Context, any) (response any, err error)
