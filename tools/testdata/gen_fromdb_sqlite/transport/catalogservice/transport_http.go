package catalogservice

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/dreamsxin/go-kit/transport/http/server"
	idl "example.com/gen_fromdb_sqlite"
	genendpoint "example.com/gen_fromdb_sqlite/endpoint/catalogservice"
)

// NewHTTPHandler returns the generated HTTP handler set.
func NewHTTPHandler(endpoints genendpoint.CatalogServiceEndpoints) http.Handler {
	m := http.NewServeMux()
	registerHTTPServeMuxRoutes(m, endpoints)
	return m
}

// RegisterHTTPRoutes binds the generated HTTP routes onto a gorilla/mux router.
func RegisterHTTPRoutes(router *mux.Router, endpoints genendpoint.CatalogServiceEndpoints, prefix string) {

	// POST /user
	router.Handle(routePath(prefix, "/user"), server.NewJSONEndpoint[idl.CreateUserRequest](
		endpoints.CreateUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("POST")

	// GET /user/{id}
	router.Handle(routePath(prefix, "/user/{id}"), server.NewJSONEndpoint[idl.GetUserRequest](
		endpoints.GetUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// PUT /user/{id}
	router.Handle(routePath(prefix, "/user/{id}"), server.NewJSONEndpoint[idl.UpdateUserRequest](
		endpoints.UpdateUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("PUT")

	// DELETE /user/{id}
	router.Handle(routePath(prefix, "/user/{id}"), server.NewJSONEndpoint[idl.DeleteUserRequest](
		endpoints.DeleteUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("DELETE")

	// GET /users
	router.Handle(routePath(prefix, "/users"), server.NewJSONEndpoint[idl.ListUsersRequest](
		endpoints.ListUsersEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

}

func registerHTTPServeMuxRoutes(m *http.ServeMux, endpoints genendpoint.CatalogServiceEndpoints) {

	m.Handle("POST /user", server.NewJSONEndpoint[idl.CreateUserRequest](
		endpoints.CreateUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /user/{id}", server.NewJSONEndpoint[idl.GetUserRequest](
		endpoints.GetUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("PUT /user/{id}", server.NewJSONEndpoint[idl.UpdateUserRequest](
		endpoints.UpdateUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("DELETE /user/{id}", server.NewJSONEndpoint[idl.DeleteUserRequest](
		endpoints.DeleteUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /users", server.NewJSONEndpoint[idl.ListUsersRequest](
		endpoints.ListUsersEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func routePath(prefix, route string) string {
	if prefix == "" {
		return route
	}
	return prefix + route
}


// decodeCreateUserRequest uses the default JSON decode path.
//
// @Summary      Create User
// @Description  Create User
// @Tags         CatalogService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.CreateUserRequest  true  "CreateUser request"
// @Success      200      {object}  idl.CreateUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /user [post]
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

// decodeGetUserRequest uses the default JSON decode path.
//
// @Summary      Get User
// @Description  Get User details
// @Tags         CatalogService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.GetUserRequest  true  "GetUser request"
// @Success      200      {object}  idl.GetUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /user/{id} [get]
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

// decodeUpdateUserRequest uses the default JSON decode path.
//
// @Summary      Update User
// @Description  Update User
// @Tags         CatalogService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.UpdateUserRequest  true  "UpdateUser request"
// @Success      200      {object}  idl.UpdateUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /user/{id} [put]
func decodeUpdateUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeUpdateUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeDeleteUserRequest uses the default JSON decode path.
//
// @Summary      Delete User
// @Description  Delete User
// @Tags         CatalogService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.DeleteUserRequest  true  "DeleteUser request"
// @Success      200      {object}  idl.DeleteUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /user/{id} [delete]
func decodeDeleteUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.DeleteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeDeleteUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeListUsersRequest uses the default JSON decode path.
//
// @Summary      List Users
// @Description  List Users
// @Tags         CatalogService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.ListUsersRequest  true  "ListUsers request"
// @Success      200      {object}  idl.ListUsersResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /users [get]
func decodeListUsersRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.ListUsersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeListUsersResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

