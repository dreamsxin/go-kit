package http

import (
	"context"
	"net/http"
)

// PopulateRequestContext is a RequestFunc that populates several values into
// the context from the HTTP request. Those values may be extracted using the
// corresponding ContextKey type in this package.
func PopulateRequestContext(ctx context.Context, r *http.Request) context.Context {
	for k, v := range map[contextKey]string{
		ContextKeyRequestMethod:          r.Method,
		ContextKeyRequestURI:             r.RequestURI,
		ContextKeyRequestPath:            r.URL.Path,
		ContextKeyRequestProto:           r.Proto,
		ContextKeyRequestHost:            r.Host,
		ContextKeyRequestRemoteAddr:      r.RemoteAddr,
		ContextKeyRequestXForwardedFor:   r.Header.Get("X-Forwarded-For"),
		ContextKeyRequestXForwardedProto: r.Header.Get("X-Forwarded-Proto"),
		ContextKeyRequestAuthorization:   r.Header.Get("Authorization"),
		ContextKeyRequestReferer:         r.Header.Get("Referer"),
		ContextKeyRequestUserAgent:       r.Header.Get("User-Agent"),
		ContextKeyRequestXRequestID:      r.Header.Get("X-Request-Id"),
		ContextKeyRequestAccept:          r.Header.Get("Accept"),
	} {
		ctx = context.WithValue(ctx, k, v)
	}
	return ctx
}

type contextKey int

const (
	// Its value is r.Method.
	ContextKeyRequestMethod contextKey = iota

	// Its value is r.RequestURI.
	ContextKeyRequestURI

	// Its value is r.URL.Path.
	ContextKeyRequestPath

	// Its value is r.Proto.
	ContextKeyRequestProto

	// Its value is r.Host.
	ContextKeyRequestHost

	// Its value is r.RemoteAddr.
	ContextKeyRequestRemoteAddr

	// Its value is r.Header.Get("X-Forwarded-For").
	ContextKeyRequestXForwardedFor

	// Its value is r.Header.Get("X-Forwarded-Proto").
	ContextKeyRequestXForwardedProto

	// Its value is r.Header.Get("Authorization").
	ContextKeyRequestAuthorization

	// Its value is r.Header.Get("Referer").
	ContextKeyRequestReferer

	// Its value is r.Header.Get("User-Agent").
	ContextKeyRequestUserAgent

	// Its value is r.Header.Get("X-Request-Id").
	ContextKeyRequestXRequestID

	// Its value is r.Header.Get("Accept").
	ContextKeyRequestAccept

	// Its value is of type http.Header
	ContextKeyResponseHeaders

	// Its value is of type int64.
	ContextKeyResponseSize
)
