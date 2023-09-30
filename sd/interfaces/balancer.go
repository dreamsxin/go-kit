package interfaces

import (
	"errors"

	"github.com/dreamsxin/go-kit/endpoint"
)

// 端点负载均衡接口
type Balancer interface {
	Endpoint() (endpoint.Endpoint, error)
}

// 没有端点可选择时返回的错误信息
var ErrNoEndpoints = errors.New("no endpoints available")
