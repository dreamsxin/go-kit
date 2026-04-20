package userservice

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/dreamsxin/go-kit/transport/http/server"
	idl "example.com/gen_proto_component_flow/pb"
	genendpoint "example.com/gen_proto_component_flow/endpoint/userservice"
)

// NewHTTPHandler returns the generated HTTP handler set.
func NewHTTPHandler(endpoints genendpoint.UserServiceEndpoints) http.Handler {
	m := http.NewServeMux()
	registerHTTPServeMuxRoutes(m, endpoints)
	return m
}

// RegisterHTTPRoutes binds the generated HTTP routes onto a gorilla/mux router.
func RegisterHTTPRoutes(router *mux.Router, endpoints genendpoint.UserServiceEndpoints, prefix string) {

	// GET /getuser
	router.Handle(routePath(prefix, "/getuser"), server.NewJSONEndpoint[idl.GetUserRequest](
		endpoints.GetUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// POST /createuser
	router.Handle(routePath(prefix, "/createuser"), server.NewJSONEndpoint[idl.CreateUserRequest](
		endpoints.CreateUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("POST")

}

func registerHTTPServeMuxRoutes(m *http.ServeMux, endpoints genendpoint.UserServiceEndpoints) {

	m.Handle("GET /getuser", server.NewJSONEndpoint[idl.GetUserRequest](
		endpoints.GetUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("POST /createuser", server.NewJSONEndpoint[idl.CreateUserRequest](
		endpoints.CreateUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func routePath(prefix, route string) string {
	if prefix == "" {
		return route
	}
	return prefix + route
}


// decodeGetUserRequest uses the default JSON decode path.
//
// @Summary      GetUser retrieves a user by ID.
// @Description  GetUser retrieves a user by ID.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.GetUserRequest  true  "GetUser request"
// @Success      200      {object}  idl.GetUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /getuser [get]
func decodeGetUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.GetUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeGetUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeCreateUserRequest uses the default JSON decode path.
//
// @Summary      CreateUser creates a new user.
// @Description  CreateUser creates a new user.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.CreateUserRequest  true  "CreateUser request"
// @Success      200      {object}  idl.CreateUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /createuser [post]
func decodeCreateUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeCreateUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

