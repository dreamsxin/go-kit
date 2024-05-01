package endpoint

import (
	"context"
	"time"
)

// 端点：映射到一个具体目标地址
type Endpoint func(ctx context.Context, request interface{}) (response interface{}, err error)

func Nop(context.Context, interface{}) (interface{}, error) { return struct{}{}, nil }

type EndpointerOptions struct {
	InvalidateOnError bool
	InvalidateTimeout time.Duration
}

type EndpointerOption func(*EndpointerOptions)

func InvalidateOnError(timeout time.Duration) EndpointerOption {
	return func(opts *EndpointerOptions) {
		opts.InvalidateOnError = true
		opts.InvalidateTimeout = timeout
	}
}

type Failer interface {
	Failed() error
}
