package server

import (
	"context"
	"net/http"
)

// 发出请求前可以进行额外的工作，将信息放入 context
type RequestFunc func(context.Context, *http.Request) context.Context
