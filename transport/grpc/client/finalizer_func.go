package client

import "context"

// 请求完成后调用
type FinalizerFunc func(ctx context.Context, err error)
