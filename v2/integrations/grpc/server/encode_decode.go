package server

import (
	"context"
)

// DecodeRequestFunc decodes a raw gRPC request proto into a domain request value.
type DecodeRequestFunc func(context.Context, any) (request any, err error)

// EncodeResponseFunc encodes a domain response value into a gRPC response proto.
type EncodeResponseFunc func(context.Context, any) (response any, err error)
