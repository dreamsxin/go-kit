package userservice

import (
	"context"
	"encoding/json"
	genendpoint "example.com/gen_proto_component_flow/endpoint/userservice"
	idl "example.com/gen_proto_component_flow/pb"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
	"net/http"
)

// NewHTTPHandler returns the generated HTTP handler set.
func NewHTTPHandler(endpoints genendpoint.UserServiceEndpoints) http.Handler {
	m := http.NewServeMux()
	registerHTTPServeMuxRoutes(m, endpoints)
	return m
}

// RegisterHTTPRoutes binds the generated HTTP routes onto a standard ServeMux.
func RegisterHTTPRoutes(router *http.ServeMux, endpoints genendpoint.UserServiceEndpoints, prefix string) {

	// GET /getuser
	router.Handle("GET "+routePath(prefix, "/getuser"), server.NewServer(
		endpoints.GetUserEndpoint,
		decodeGetUserRequest,
		encodeGetUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// POST /createuser
	router.Handle("POST "+routePath(prefix, "/createuser"), server.NewServer(
		endpoints.CreateUserEndpoint,
		decodeCreateUserRequest,
		encodeCreateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func registerHTTPServeMuxRoutes(m *http.ServeMux, endpoints genendpoint.UserServiceEndpoints) {

	m.Handle("GET /getuser", server.NewServer(
		endpoints.GetUserEndpoint,
		decodeGetUserRequest,
		encodeGetUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("POST /createuser", server.NewServer(
		endpoints.CreateUserEndpoint,
		decodeCreateUserRequest,
		encodeCreateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func routePath(prefix, route string) string {
	if prefix == "" {
		return route
	}
	return prefix + route
}

// decodeGetUserRequest uses the generated method-aware decode path.
func decodeGetUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.GetUserRequest
	if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeGetUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeCreateUserRequest uses the generated method-aware decode path.
func decodeCreateUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.CreateUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	if err := transporthttp.DecodePathRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeCreateUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}
