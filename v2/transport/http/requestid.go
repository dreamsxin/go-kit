package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// RequestIDHeader is the header a request ID travels in.
const RequestIDHeader = "X-Request-ID"

// MaxRequestIDLength is the largest request ID the default policy accepts.
const MaxRequestIDLength = 128

// RequestIDValidator decides whether a caller-supplied request ID is trusted.
// An ID that fails validation is replaced by a fresh one rather than rejected:
// correlation is a convenience, not an authorization decision, and a malformed
// header should not fail a request.
type RequestIDValidator func(string) bool

// DefaultRequestIDValidator accepts common ASCII token characters and rejects
// empty, oversized, whitespace-containing, or control-character values. The last
// two matter because the ID is echoed into a response header, and a value
// carrying CR or LF would be a header injection.
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

// NewRequestID mints a request ID. It falls back to a fixed value rather than
// failing when the entropy source does, because a request without correlation is
// still a request worth serving.
func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return "request-id-unavailable"
}

// RequestID returns the request's correlation ID: the caller's if it passes
// validator, a fresh one otherwise. A nil validator selects
// DefaultRequestIDValidator.
func RequestID(r *http.Request, validator RequestIDValidator) string {
	if r != nil {
		if id := r.Header.Get(RequestIDHeader); ValidRequestID(validator, id) {
			return id
		}
	}
	return NewRequestID()
}

// ValidRequestID reports whether id passes validator, defaulting to
// DefaultRequestIDValidator.
func ValidRequestID(validator RequestIDValidator, id string) bool {
	if validator == nil {
		validator = DefaultRequestIDValidator
	}
	return validator(id)
}

// ExtractRequestID is a RequestFunc that puts the request's correlation ID into
// the context under the default policy, so an endpoint and its logs can read it
// through endpoint.RequestIDFromContext:
//
//	server.NewServer(ep, server.ServerBefore(transporthttp.ExtractRequestID))
//
// Use RequestIDExtractor for a validator of your own.
func ExtractRequestID(ctx context.Context, r *http.Request) context.Context {
	return RequestIDExtractor(nil)(ctx, r)
}

// RequestIDExtractor builds an ExtractRequestID with a specific validator.
func RequestIDExtractor(validator RequestIDValidator) func(context.Context, *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		if id := endpoint.RequestIDFromContext(ctx); ValidRequestID(validator, id) {
			return ctx
		}
		return endpoint.WithRequestID(ctx, RequestID(r, validator))
	}
}

// EchoRequestID is a ResponseFunc that writes the context's request ID into the
// response, so a caller can correlate what it received with what the service
// logged:
//
//	server.NewServer(ep, server.ServerAfter(transporthttp.EchoRequestID))
//
// It writes nothing when no ID is in the context; pair it with
// ExtractRequestID, which guarantees one.
func EchoRequestID(ctx context.Context, w http.ResponseWriter) context.Context {
	id := endpoint.RequestIDFromContext(ctx)
	if id == "" || w == nil {
		return ctx
	}
	w.Header().Set(RequestIDHeader, id)
	return ctx
}
