package userservice

import (
	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
)

// applyCustomMiddleware is the user-owned hook for endpoint middleware.
// Add custom endpoint middleware here; microgen will not overwrite this file on rerun.
func applyCustomMiddleware(ep endpoint.Endpoint, logger *kitlog.Logger, cfg MiddlewareConfig, name string) endpoint.Endpoint {
	_ = logger
	_ = cfg
	_ = name
	return ep
}

// preserved custom middleware
