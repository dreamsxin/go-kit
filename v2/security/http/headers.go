package httpsecurity

import (
	"fmt"
	"net/http"
	"strings"
)

// SecurityHeadersConfig controls response hardening. The zero value applies
// conservative API-safe defaults. Set DisableDefaults to emit only explicitly
// configured values.
type SecurityHeadersConfig struct {
	DisableDefaults         bool
	FrameOptions            string
	ReferrerPolicy          string
	ContentSecurityPolicy   string
	PermissionsPolicy       string
	CrossOriginOpenerPolicy string
	StrictTransportSecurity string
}

type securityHeaders struct {
	values map[string]string
	hsts   string
}

// NewSecurityHeaders creates middleware that writes security headers before
// the wrapped handler starts. HSTS is emitted only for an effective HTTPS
// request.
func NewSecurityHeaders(config SecurityHeadersConfig) (Middleware, error) {
	values := make(map[string]string)
	if !config.DisableDefaults {
		values["X-Content-Type-Options"] = "nosniff"
		values["X-Frame-Options"] = "DENY"
		values["Referrer-Policy"] = "no-referrer"
		values["Cross-Origin-Opener-Policy"] = "same-origin"
	}
	configured := map[string]string{
		"X-Frame-Options":            config.FrameOptions,
		"Referrer-Policy":            config.ReferrerPolicy,
		"Content-Security-Policy":    config.ContentSecurityPolicy,
		"Permissions-Policy":         config.PermissionsPolicy,
		"Cross-Origin-Opener-Policy": config.CrossOriginOpenerPolicy,
	}
	for name, value := range configured {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !validHeaderValue(value) {
			return nil, fmt.Errorf("httpsecurity: invalid %s value", name)
		}
		values[name] = value
	}
	hsts := strings.TrimSpace(config.StrictTransportSecurity)
	if !validHeaderValue(hsts) {
		return nil, fmt.Errorf("httpsecurity: invalid Strict-Transport-Security value")
	}
	policy := securityHeaders{values: values, hsts: hsts}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			for name, value := range policy.values {
				w.Header().Set(name, value)
			}
			if policy.hsts != "" && effectiveScheme(request) == "https" {
				w.Header().Set("Strict-Transport-Security", policy.hsts)
			}
			next.ServeHTTP(w, request)
		})
	}, nil
}
