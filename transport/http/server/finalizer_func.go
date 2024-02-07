package server

import (
	"context"
	"net/http"
)

// 返回结果前进行额外的工作
type FinalizerFunc func(context.Context, *http.Request, *InterceptingWriter)
