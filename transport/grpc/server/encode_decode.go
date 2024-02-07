package server

import (
	"context"
)

// 将 gRPC request 对象转为用户定义的数据
type DecodeRequestFunc func(context.Context, interface{}) (request interface{}, err error)

// 将返回的数据 转为 gRPC response 对象
type EncodeResponseFunc func(context.Context, interface{}) (response interface{}, err error)
