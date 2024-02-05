package server

import (
	"context"
	"net/http"
)

// 返回结果前进行额外的工作
type ResponseFunc func(context.Context, *http.Request, *InterceptingWriter) context.Context
