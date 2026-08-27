package kit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

const requestIDHeader = "X-Request-ID"

// MaxRequestIDLength is the largest request ID accepted by the default policy.
const MaxRequestIDLength = 128

// RequestIDValidator decides whether a caller-supplied request ID is trusted.
type RequestIDValidator func(string) bool

func requestIDMiddleware(h *HTTP) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			requestID := requestIDFromContextOrHeader(ctx, h.requestIDValidator)
			ctx = endpoint.WithRequestID(ctx, requestID)
			if rw := responseWriterFromContext(ctx); rw != nil {
				rw.Header().Set(requestIDHeader, requestID)
			} else if rw, ok := req.(http.ResponseWriter); ok {
				rw.Header().Set(requestIDHeader, requestID)
			}
			return next(ctx, req)
		}
	}
}

func requestIDFromContextOrHeader(ctx context.Context, validator RequestIDValidator) string {
	if id := endpoint.RequestIDFromContext(ctx); validateRequestID(validator, id) {
		return id
	}
	if r := requestFromContext(ctx); r != nil {
		if id := r.Header.Get(requestIDHeader); validateRequestID(validator, id) {
			return id
		}
	}
	return newRequestID()
}

func validateRequestID(validator RequestIDValidator, id string) bool {
	if validator == nil {
		validator = DefaultRequestIDValidator
	}
	return validator(id)
}

// DefaultRequestIDValidator accepts common ASCII token characters and rejects
// empty, oversized, whitespace-containing, or control-character values.
func DefaultRequestIDValidator(id string) bool {
	if len(id) == 0 || len(id) > MaxRequestIDLength {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == ':' {
			continue
		}
		return false
	}
	return true
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return "request-id-unavailable"
}
