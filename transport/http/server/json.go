package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
)

// NewJSONServer creates an HTTP server that automatically handles JSON
// encoding/decoding for the given handler function.
//
// JSONErrorEncoder is used by default — errors are written as
// {"error": "..."} with an appropriate HTTP status code.
// Pass ServerErrorEncoder to override.
//
// Example:
//
//	handler := server.NewJSONServer[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
//	    return HelloResp{...}, nil
//	})
//	http.ListenAndServe(":8080", handler)
func NewJSONServer[Req any](
	handler func(ctx context.Context, req Req) (any, error),
	options ...ServerOption,
) *Server {
	e := endpoint.Endpoint(func(ctx context.Context, request any) (any, error) {
		return handler(ctx, request.(Req))
	})
	// JSONErrorEncoder is the default; callers can override via ServerErrorEncoder.
	opts := append([]ServerOption{ServerErrorEncoder(JSONErrorEncoder)}, options...)
	return NewServer(e, DecodeJSONRequest[Req](), EncodeJSONResponse, opts...)
}

// NewJSONEndpoint creates an HTTP server for an existing endpoint.Endpoint.
// Use this when you have already built your endpoint with middleware.
func NewJSONEndpoint[Req any](
	e endpoint.Endpoint,
	options ...ServerOption,
) *Server {
	opts := append([]ServerOption{ServerErrorEncoder(JSONErrorEncoder)}, options...)
	return NewServer(e, DecodeJSONRequest[Req](), EncodeJSONResponse, opts...)
}

// NewJSONServerWithMiddleware is a convenience wrapper that combines
// a handler function with a middleware chain built via endpoint.Builder.
func NewJSONServerWithMiddleware[Req any](
	handler func(ctx context.Context, req Req) (any, error),
	middleware func(*endpoint.Builder) *endpoint.Builder,
	options ...ServerOption,
) *Server {
	e := endpoint.Endpoint(func(ctx context.Context, request any) (any, error) {
		return handler(ctx, request.(Req))
	})
	ep := middleware(endpoint.NewBuilder(e)).Build()
	return NewJSONEndpoint[Req](ep, options...)
}

// DecodeJSONRequest returns a DecodeRequestFunc that decodes the HTTP request
// body as JSON into a value of type T.
func DecodeJSONRequest[T any]() DecodeRequestFunc {
	return func(_ context.Context, r *http.Request) (any, error) {
		var v T
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}
}
