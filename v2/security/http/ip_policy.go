package httpsecurity

import (
	"fmt"
	"net/http"
)

// IPPolicyConfig denies matching Deny networks first. When Allow is non-empty,
// all other client IPs are denied.
type IPPolicyConfig struct {
	Allow []string
	Deny  []string
}

// NewIPPolicy creates client IP allow/deny middleware. Place NewTrustedProxy
// outside this middleware when forwarded client IPs should be considered.
func NewIPPolicy(config IPPolicyConfig) (Middleware, error) {
	allow, err := parseNetworks(config.Allow)
	if err != nil {
		return nil, fmt.Errorf("httpsecurity: allow policy: %w", err)
	}
	deny, err := parseNetworks(config.Deny)
	if err != nil {
		return nil, fmt.Errorf("httpsecurity: deny policy: %w", err)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			ip, err := effectiveClientIP(request)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
			if containsIP(deny, ip) || len(allow) > 0 && !containsIP(allow, ip) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, request)
		})
	}, nil
}
