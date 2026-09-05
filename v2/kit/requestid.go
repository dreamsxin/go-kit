package kit

import (
	"context"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

// RequestIDValidator decides whether a caller-supplied request ID is trusted.
//
// It is transporthttp.RequestIDValidator: the header name, the trust policy, and
// the generator belong to the HTTP transport, so a service assembled from the
// transport packages correlates requests the same way this assembly does.
type RequestIDValidator = transporthttp.RequestIDValidator

// MaxRequestIDLength is the largest request ID accepted by the default policy.
const MaxRequestIDLength = transporthttp.MaxRequestIDLength

// DefaultRequestIDValidator accepts common ASCII token characters and rejects
// empty, oversized, whitespace-containing, or control-character values.
func DefaultRequestIDValidator(id string) bool {
	return transporthttp.DefaultRequestIDValidator(id)
}

// requestIDMiddleware puts the request's correlation ID in the context and
// echoes it to the caller. The decision of which ID to use is the transport's;
// what belongs here is reaching the request and the writer through this
// assembly's context plumbing.
//
// The validator is read per call, not captured when the middleware is built, so
// WithRequestID and WithRequestIDValidator work in either order.
func requestIDMiddleware(h *HTTP) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			ctx = transporthttp.RequestIDExtractor(h.requestIDValidator)(ctx, requestFromContext(ctx))
			requestID := endpoint.RequestIDFromContext(ctx)
			if rw := responseWriterFromContext(ctx); rw != nil {
				rw.Header().Set(transporthttp.RequestIDHeader, requestID)
			} else if rw, ok := req.(http.ResponseWriter); ok {
				rw.Header().Set(transporthttp.RequestIDHeader, requestID)
			}
			return next(ctx, req)
		}
	}
}
