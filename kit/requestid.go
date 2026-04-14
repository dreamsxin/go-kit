package kit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
)

const requestIDHeader = "X-Request-ID"

func requestIDMiddleware() endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			requestID := requestIDFromContextOrHeader(ctx)
			ctx = endpoint.WithRequestID(ctx, requestID)
			if rw, ok := req.(http.ResponseWriter); ok {
				rw.Header().Set(requestIDHeader, requestID)
			}
			return next(ctx, req)
		}
	}
}

func requestIDFromContextOrHeader(ctx context.Context) string {
	if id := endpoint.RequestIDFromContext(ctx); id != "" {
		return id
	}
	if r := requestFromContext(ctx); r != nil {
		if id := r.Header.Get(requestIDHeader); id != "" {
			return id
		}
	}
	return newRequestID()
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return "request-id-unavailable"
}
