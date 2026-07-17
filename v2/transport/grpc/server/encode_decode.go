package server

import (
	"context"
)

// DecodeRequestFunc decodes a raw gRPC request proto into a domain request value.
type DecodeRequestFunc func(context.Context, interface{}) (request interface{}, err error)

// EncodeResponseFunc encodes a domain response value into a gRPC response proto.
type EncodeResponseFunc func(context.Context, interface{}) (response interface{}, err error)
