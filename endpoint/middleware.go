package endpoint

// 定义端点中间件类型
type Middleware func(Endpoint) Endpoint

// 链式调用中间件
func Chain(outer Middleware, others ...Middleware) Middleware {
	return func(next Endpoint) Endpoint {
		for i := len(others) - 1; i >= 0; i-- { // reverse
			next = others[i](next)
		}
		return outer(next)
	}
}
