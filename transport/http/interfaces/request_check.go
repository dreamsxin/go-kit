package interfaces

import "net/http"

// 用于接口类型判断
type StatusCoder interface {
	StatusCode() int
}

type Headerer interface {
	Headers() http.Header
}
