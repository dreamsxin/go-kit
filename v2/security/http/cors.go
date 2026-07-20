package httpsecurity

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

var defaultCORSMethods = []string{http.MethodGet, http.MethodHead, http.MethodPost}
var defaultCORSHeaders = []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"}

// CORSConfig defines browser cross-origin policy. AllowedOrigins must contain
// exact HTTP(S) origins or a single "*" entry.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           time.Duration
}

type corsPolicy struct {
	origins          map[string]struct{}
	wildcard         bool
	methods          map[string]struct{}
	methodHeader     string
	headers          map[string]struct{}
	headerHeader     string
	exposedHeader    string
	allowCredentials bool
	maxAge           string
}

// NewCORS validates and compiles CORS policy.
func NewCORS(config CORSConfig) (Middleware, error) {
	policy, err := compileCORS(config)
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			origin := request.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, request)
				return
			}
			normalized, err := normalizedOrigin(origin)
			if err != nil || !policy.allowsOrigin(normalized) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			preflightMethod := strings.ToUpper(strings.TrimSpace(request.Header.Get("Access-Control-Request-Method")))
			preflight := request.Method == http.MethodOptions && preflightMethod != ""
			method := request.Method
			if preflight {
				method = preflightMethod
			}
			if _, ok := policy.methods[method]; !ok {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			if preflight && !policy.allowsHeaders(request.Header.Get("Access-Control-Request-Headers")) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			policy.writeOriginHeaders(w.Header(), origin)
			if preflight {
				addVary(w.Header(), "Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers")
				w.Header().Set("Access-Control-Allow-Methods", policy.methodHeader)
				if policy.headerHeader != "" {
					w.Header().Set("Access-Control-Allow-Headers", policy.headerHeader)
				}
				if policy.maxAge != "" {
					w.Header().Set("Access-Control-Max-Age", policy.maxAge)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			addVary(w.Header(), "Origin")
			if policy.exposedHeader != "" {
				w.Header().Set("Access-Control-Expose-Headers", policy.exposedHeader)
			}
			next.ServeHTTP(w, request)
		})
	}, nil
}

func compileCORS(config CORSConfig) (*corsPolicy, error) {
	if len(config.AllowedOrigins) == 0 {
		return nil, fmt.Errorf("httpsecurity: CORS allowed origins are required")
	}
	policy := &corsPolicy{
		origins:          make(map[string]struct{}),
		methods:          make(map[string]struct{}),
		headers:          make(map[string]struct{}),
		allowCredentials: config.AllowCredentials,
	}
	for _, origin := range config.AllowedOrigins {
		if strings.TrimSpace(origin) == "*" {
			policy.wildcard = true
			continue
		}
		normalized, err := normalizedOrigin(origin)
		if err != nil {
			return nil, fmt.Errorf("httpsecurity: CORS: %w", err)
		}
		policy.origins[normalized] = struct{}{}
	}
	if policy.wildcard && (len(policy.origins) > 0 || config.AllowCredentials) {
		return nil, fmt.Errorf("httpsecurity: CORS wildcard origin cannot be combined with exact origins or credentials")
	}

	methods := config.AllowedMethods
	if len(methods) == 0 {
		methods = defaultCORSMethods
	}
	methodValues := make([]string, 0, len(methods))
	for _, method := range methods {
		method = strings.ToUpper(strings.TrimSpace(method))
		if !validMethod(method) {
			return nil, fmt.Errorf("httpsecurity: CORS invalid method %q", method)
		}
		if _, ok := policy.methods[method]; !ok {
			policy.methods[method] = struct{}{}
			methodValues = append(methodValues, method)
		}
	}
	sort.Strings(methodValues)
	policy.methodHeader = strings.Join(methodValues, ", ")

	headers := config.AllowedHeaders
	if len(headers) == 0 {
		headers = defaultCORSHeaders
	}
	headerValues, err := compileHeaderNames(headers, policy.headers)
	if err != nil {
		return nil, fmt.Errorf("httpsecurity: CORS allowed headers: %w", err)
	}
	policy.headerHeader = strings.Join(headerValues, ", ")
	exposed, err := compileHeaderNames(config.ExposedHeaders, nil)
	if err != nil {
		return nil, fmt.Errorf("httpsecurity: CORS exposed headers: %w", err)
	}
	policy.exposedHeader = strings.Join(exposed, ", ")
	if config.MaxAge < 0 {
		return nil, fmt.Errorf("httpsecurity: CORS max age cannot be negative")
	}
	if config.MaxAge > 0 {
		policy.maxAge = strconv.FormatInt(int64(config.MaxAge/time.Second), 10)
	}
	return policy, nil
}

func compileHeaderNames(values []string, target map[string]struct{}) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = http.CanonicalHeaderKey(strings.TrimSpace(value))
		if !validHeaderName(value) {
			return nil, fmt.Errorf("invalid header %q", value)
		}
		lower := strings.ToLower(value)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		if target != nil {
			target[lower] = struct{}{}
		}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func (policy *corsPolicy) allowsOrigin(origin string) bool {
	if policy.wildcard {
		return true
	}
	_, ok := policy.origins[origin]
	return ok
}

func (policy *corsPolicy) allowsHeaders(value string) bool {
	if strings.TrimSpace(value) == "" {
		return true
	}
	for _, header := range strings.Split(value, ",") {
		header = strings.ToLower(strings.TrimSpace(header))
		if !validHeaderName(header) {
			return false
		}
		if _, ok := policy.headers[header]; !ok {
			return false
		}
	}
	return true
}

func (policy *corsPolicy) writeOriginHeaders(header http.Header, origin string) {
	if policy.wildcard {
		header.Set("Access-Control-Allow-Origin", "*")
	} else {
		header.Set("Access-Control-Allow-Origin", origin)
	}
	if policy.allowCredentials {
		header.Set("Access-Control-Allow-Credentials", "true")
	}
}
