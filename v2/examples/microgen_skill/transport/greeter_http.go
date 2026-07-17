package transport

import (
	"context"
	"encoding/json"
	"net/http"

	genendpoint "github.com/dreamsxin/go-kit/v2/examples/microgen_skill/endpoint"
	idl "github.com/dreamsxin/go-kit/v2/examples/microgen_skill/pb"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// NewHTTPHandler returns the Greeter HTTP handler set.
func NewHTTPHandler(endpoints genendpoint.GreeterEndpoints) http.Handler {
	m := http.NewServeMux()
	RegisterHTTPRoutes(m, endpoints, "")
	return m
}

// RegisterHTTPRoutes binds Greeter routes to a standard ServeMux.
func RegisterHTTPRoutes(router *http.ServeMux, endpoints genendpoint.GreeterEndpoints, prefix string) {
	router.Handle("POST "+routePath(prefix, "/sayhello"), server.NewServer(
		endpoints.SayHelloEndpoint,
		DecodeSayHelloRequest,
		encodeResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))
	router.Handle("GET "+routePath(prefix, "/getstatus"), server.NewServer(
		endpoints.GetStatusEndpoint,
		DecodeGetStatusRequest,
		encodeResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))
}

func routePath(prefix, route string) string {
	if prefix == "" {
		return route
	}
	return prefix + route
}

func DecodeSayHelloRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.HelloRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func DecodeGetStatusRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.Empty
	if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeResponse(_ context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}
