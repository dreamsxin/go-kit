package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
)

// NewJSONServer creates an HTTP server that automatically handles JSON
// encoding/decoding for the given endpoint.
//
// JSONErrorEncoder is used by default — errors are written as
// {"error": "..."} with an appropriate HTTP status code.
// Pass ServerErrorEncoder to override.
//
// Example:
//
//	handler := server.NewJSONServer[HelloReq](
//	    func(ctx context.Context, req HelloReq) (any, error) {
//	        return HelloResp{Message: "Hello, " + req.Name}, nil
//	    },
//	)
//	http.ListenAndServe(":8080", handler)
func NewJSONServer[Req any](
	handler func(ctx context.Context, req Req) (any, error),
	options ...ServerOption,
) *Server {
	ep := endpoint.Endpoint(func(ctx context.Context, request any) (any, error) {
		return handler(ctx, request.(Req))
	})
	dec := func(_ context.Context, r *http.Request) (any, error) {
		var req Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
		return req, nil
	}
	// JSONErrorEncoder is the default; callers can override via ServerErrorEncoder.
	opts := append([]ServerOption{ServerErrorEncoder(JSONErrorEncoder)}, options...)
	return NewServer(ep, dec, EncodeJSONResponse, opts...)
}

// DecodeJSONRequest returns a DecodeRequestFunc that decodes the HTTP request
// body as JSON into a value of type T.
//
// Useful when you want standard JSON decoding but still need to pass a custom
// DecodeRequestFunc to NewServer.
//
// Example:
//
//	server.NewServer(ep, server.DecodeJSONRequest[MyRequest](), server.EncodeJSONResponse)
func DecodeJSONRequest[T any]() DecodeRequestFunc {
	return func(_ context.Context, r *http.Request) (any, error) {
		var v T
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}
}

// NewJSONServerWithMiddleware is a convenience wrapper that combines
// NewJSONServer with a middleware chain built via endpoint.Builder.
//
// It is the recommended entry point for most JSON API handlers:
//
//	handler := server.NewJSONServerWithMiddleware[HelloReq](
//	    func(ctx context.Context, req HelloReq) (any, error) {
//	        return HelloResp{Message: "Hello, " + req.Name}, nil
//	    },
//	    func(b *endpoint.Builder) *endpoint.Builder {
//	        return b.
//	            WithTimeout(5 * time.Second).
//	            Use(ratelimit.NewErroringLimiter(limiter)).
//	            Use(circuitbreaker.Gobreaker(cb))
//	    },
//	    server.ServerErrorEncoder(server.JSONErrorEncoder),
//	)
func NewJSONServerWithMiddleware[Req any](
	handler func(ctx context.Context, req Req) (any, error),
	middleware func(*endpoint.Builder) *endpoint.Builder,
	options ...ServerOption,
) *Server {
	base := endpoint.Endpoint(func(ctx context.Context, request any) (any, error) {
		return handler(ctx, request.(Req))
	})
	ep := middleware(endpoint.NewBuilder(base)).Build()
	dec := func(_ context.Context, r *http.Request) (any, error) {
		var req Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
		return req, nil
	}
	return NewServer(ep, dec, EncodeJSONResponse, options...)
}
