package client

import (
	"context"
)

// EncodeRequestFunc encodes a domain request value into a gRPC request proto.
type EncodeRequestFunc func(context.Context, interface{}) (request interface{}, err error)

// DecodeResponseFunc decodes a gRPC response proto into a domain response value.
type DecodeResponseFunc func(context.Context, interface{}) (response interface{}, err error)
