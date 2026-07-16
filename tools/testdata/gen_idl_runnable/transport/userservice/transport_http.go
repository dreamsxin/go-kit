package userservice

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/dreamsxin/go-kit/transport/http/server"
	idl "example.com/gen_idl_runnable"
	genendpoint "example.com/gen_idl_runnable/endpoint/userservice"
)

// NewHTTPHandler returns the generated HTTP handler set.
func NewHTTPHandler(endpoints genendpoint.UserServiceEndpoints) http.Handler {
	m := http.NewServeMux()
	registerHTTPServeMuxRoutes(m, endpoints)
	return m
}

// RegisterHTTPRoutes binds the generated HTTP routes onto a gorilla/mux router.
func RegisterHTTPRoutes(router *mux.Router, endpoints genendpoint.UserServiceEndpoints, prefix string) {

	// POST /createuser
	router.Handle(routePath(prefix, "/createuser"), server.NewStrictJSONEndpoint[idl.CreateUserRequest](
		endpoints.CreateUserEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("POST")

	// GET /getuser
	router.Handle(routePath(prefix, "/getuser"), server.NewStrictJSONEndpoint[idl.GetUserRequest](
		endpoints.GetUserEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// GET /listusers
	router.Handle(routePath(prefix, "/listusers"), server.NewStrictJSONEndpoint[idl.ListUsersRequest](
		endpoints.ListUsersEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// DELETE /deleteuser
	router.Handle(routePath(prefix, "/deleteuser"), server.NewStrictJSONEndpoint[idl.DeleteUserRequest](
		endpoints.DeleteUserEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("DELETE")

	// PUT /updateuser
	router.Handle(routePath(prefix, "/updateuser"), server.NewStrictJSONEndpoint[idl.UpdateUserRequest](
		endpoints.UpdateUserEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("PUT")

	// GET /findbyemail
	router.Handle(routePath(prefix, "/findbyemail"), server.NewStrictJSONEndpoint[idl.GetUserRequest](
		endpoints.FindByEmailEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// GET /searchusers
	router.Handle(routePath(prefix, "/searchusers"), server.NewStrictJSONEndpoint[idl.ListUsersRequest](
		endpoints.SearchUsersEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// GET /querystats
	router.Handle(routePath(prefix, "/querystats"), server.NewStrictJSONEndpoint[idl.GetUserRequest](
		endpoints.QueryStatsEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// DELETE /removeexpired
	router.Handle(routePath(prefix, "/removeexpired"), server.NewStrictJSONEndpoint[idl.DeleteUserRequest](
		endpoints.RemoveExpiredEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("DELETE")

	// PUT /editprofile
	router.Handle(routePath(prefix, "/editprofile"), server.NewStrictJSONEndpoint[idl.UpdateUserRequest](
		endpoints.EditProfileEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("PUT")

	// PUT /modifyemail
	router.Handle(routePath(prefix, "/modifyemail"), server.NewStrictJSONEndpoint[idl.UpdateUserRequest](
		endpoints.ModifyEmailEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("PUT")

	// PUT /patchstatus
	router.Handle(routePath(prefix, "/patchstatus"), server.NewStrictJSONEndpoint[idl.UpdateUserRequest](
		endpoints.PatchStatusEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("PUT")

}

func registerHTTPServeMuxRoutes(m *http.ServeMux, endpoints genendpoint.UserServiceEndpoints) {

	m.Handle("POST /createuser", server.NewStrictJSONEndpoint[idl.CreateUserRequest](
		endpoints.CreateUserEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /getuser", server.NewStrictJSONEndpoint[idl.GetUserRequest](
		endpoints.GetUserEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /listusers", server.NewStrictJSONEndpoint[idl.ListUsersRequest](
		endpoints.ListUsersEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("DELETE /deleteuser", server.NewStrictJSONEndpoint[idl.DeleteUserRequest](
		endpoints.DeleteUserEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("PUT /updateuser", server.NewStrictJSONEndpoint[idl.UpdateUserRequest](
		endpoints.UpdateUserEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /findbyemail", server.NewStrictJSONEndpoint[idl.GetUserRequest](
		endpoints.FindByEmailEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /searchusers", server.NewStrictJSONEndpoint[idl.ListUsersRequest](
		endpoints.SearchUsersEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /querystats", server.NewStrictJSONEndpoint[idl.GetUserRequest](
		endpoints.QueryStatsEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("DELETE /removeexpired", server.NewStrictJSONEndpoint[idl.DeleteUserRequest](
		endpoints.RemoveExpiredEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("PUT /editprofile", server.NewStrictJSONEndpoint[idl.UpdateUserRequest](
		endpoints.EditProfileEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("PUT /modifyemail", server.NewStrictJSONEndpoint[idl.UpdateUserRequest](
		endpoints.ModifyEmailEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("PUT /patchstatus", server.NewStrictJSONEndpoint[idl.UpdateUserRequest](
		endpoints.PatchStatusEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func routePath(prefix, route string) string {
	if prefix == "" {
		return route
	}
	return prefix + route
}


// decodeCreateUserRequest uses the generated strict JSON decode path.
//
// @Summary      CreateUser creates a new user.
// @Description  CreateUser creates a new user.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.CreateUserRequest  true  "CreateUser request"
// @Success      200      {object}  idl.CreateUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /createuser [post]
func decodeCreateUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.CreateUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeCreateUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeGetUserRequest uses the generated strict JSON decode path.
//
// @Summary      GetUser retrieves a user by ID.
// @Description  GetUser retrieves a user by ID.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.GetUserRequest  true  "GetUser request"
// @Success      200      {object}  idl.GetUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /getuser [get]
func decodeGetUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.GetUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeGetUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeListUsersRequest uses the generated strict JSON decode path.
//
// @Summary      ListUsers lists all users.
// @Description  ListUsers lists all users.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.ListUsersRequest  true  "ListUsers request"
// @Success      200      {object}  idl.ListUsersResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /listusers [get]
func decodeListUsersRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.ListUsersRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeListUsersResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeDeleteUserRequest uses the generated strict JSON decode path.
//
// @Summary      DeleteUser removes a user.
// @Description  DeleteUser removes a user.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.DeleteUserRequest  true  "DeleteUser request"
// @Success      200      {object}  idl.DeleteUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /deleteuser [delete]
func decodeDeleteUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.DeleteUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeDeleteUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeUpdateUserRequest uses the generated strict JSON decode path.
//
// @Summary      UpdateUser modifies a user.
// @Description  UpdateUser modifies a user.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.UpdateUserRequest  true  "UpdateUser request"
// @Success      200      {object}  idl.UpdateUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /updateuser [put]
func decodeUpdateUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeUpdateUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeFindByEmailRequest uses the generated strict JSON decode path.
//
// @Summary      FindByEmail finds users by email prefix.
// @Description  FindByEmail finds users by email prefix.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.GetUserRequest  true  "FindByEmail request"
// @Success      200      {object}  idl.GetUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /findbyemail [get]
func decodeFindByEmailRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.GetUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeFindByEmailResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeSearchUsersRequest uses the generated strict JSON decode path.
//
// @Summary      SearchUsers searches users.
// @Description  SearchUsers searches users.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.ListUsersRequest  true  "SearchUsers request"
// @Success      200      {object}  idl.ListUsersResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /searchusers [get]
func decodeSearchUsersRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.ListUsersRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeSearchUsersResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeQueryStatsRequest uses the generated strict JSON decode path.
//
// @Summary      QueryStats returns statistics.
// @Description  QueryStats returns statistics.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.GetUserRequest  true  "QueryStats request"
// @Success      200      {object}  idl.GetUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /querystats [get]
func decodeQueryStatsRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.GetUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeQueryStatsResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeRemoveExpiredRequest uses the generated strict JSON decode path.
//
// @Summary      RemoveExpired removes expired users.
// @Description  RemoveExpired removes expired users.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.DeleteUserRequest  true  "RemoveExpired request"
// @Success      200      {object}  idl.DeleteUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /removeexpired [delete]
func decodeRemoveExpiredRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.DeleteUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeRemoveExpiredResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeEditProfileRequest uses the generated strict JSON decode path.
//
// @Summary      EditProfile edits profile.
// @Description  EditProfile edits profile.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.UpdateUserRequest  true  "EditProfile request"
// @Success      200      {object}  idl.UpdateUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /editprofile [put]
func decodeEditProfileRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeEditProfileResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeModifyEmailRequest uses the generated strict JSON decode path.
//
// @Summary      ModifyEmail modifies email.
// @Description  ModifyEmail modifies email.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.UpdateUserRequest  true  "ModifyEmail request"
// @Success      200      {object}  idl.UpdateUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /modifyemail [put]
func decodeModifyEmailRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodeModifyEmailResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodePatchStatusRequest uses the generated strict JSON decode path.
//
// @Summary      PatchStatus patches status.
// @Description  PatchStatus patches status.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.UpdateUserRequest  true  "PatchStatus request"
// @Success      200      {object}  idl.UpdateUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /patchstatus [put]
func decodePatchStatusRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodePatchStatusResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

