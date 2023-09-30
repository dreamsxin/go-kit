package endpoint

import "io"

// 端点构建工厂接口
type Factory func(instance string) (Endpoint, io.Closer, error)
