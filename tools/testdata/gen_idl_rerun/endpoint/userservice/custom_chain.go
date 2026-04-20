package userservice

import (
	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
)

func applyCustomMiddleware(ep endpoint.Endpoint, logger *kitlog.Logger, cfg MiddlewareConfig, name string) endpoint.Endpoint {
	_ = logger
	_ = cfg
	_ = name
	return ep
}
