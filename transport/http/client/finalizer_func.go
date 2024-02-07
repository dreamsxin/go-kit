package client

import (
	"context"
)

// 返回结果前进行额外的工作
type FinalizerFunc func(context.Context, error)
