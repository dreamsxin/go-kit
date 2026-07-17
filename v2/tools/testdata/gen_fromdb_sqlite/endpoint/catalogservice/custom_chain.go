package catalogservice

import (
	"github.com/dreamsxin/go-kit/v2/endpoint"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
)

// applyCustomMiddleware is the user-owned hook for endpoint middleware.
// Add custom endpoint middleware here; microgen will not overwrite this file on rerun.
func applyCustomMiddleware(ep endpoint.Endpoint, logger *kitlog.Logger, cfg MiddlewareConfig, name string) endpoint.Endpoint {
	_ = logger
	_ = cfg
	_ = name
	return ep
}
