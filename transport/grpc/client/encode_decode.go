package client

import (
	"context"
)

// 转为 gRPC request 对象
type EncodeRequestFunc func(context.Context, interface{}) (request interface{}, err error)

// 解码 gRPC response 对象
type DecodeResponseFunc func(context.Context, interface{}) (response interface{}, err error)
