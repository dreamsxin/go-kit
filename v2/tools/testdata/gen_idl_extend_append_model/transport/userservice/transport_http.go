package userservice

import (
	"context"
	"encoding/json"
	idl "example.com/gen_idl_extend_append_model"
	genendpoint "example.com/gen_idl_extend_append_model/endpoint/userservice"
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

	// POST /createuser
	router.Handle("POST "+routePath(prefix, "/createuser"), server.NewServer(
		endpoints.CreateUserEndpoint,
		decodeCreateUserRequest,
		encodeCreateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /getuser
	router.Handle("GET "+routePath(prefix, "/getuser"), server.NewServer(
		endpoints.GetUserEndpoint,
		decodeGetUserRequest,
		encodeGetUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /listusers
	router.Handle("GET "+routePath(prefix, "/listusers"), server.NewServer(
		endpoints.ListUsersEndpoint,
		decodeListUsersRequest,
		encodeListUsersResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// DELETE /deleteuser
	router.Handle("DELETE "+routePath(prefix, "/deleteuser"), server.NewServer(
		endpoints.DeleteUserEndpoint,
		decodeDeleteUserRequest,
		encodeDeleteUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// PUT /updateuser
	router.Handle("PUT "+routePath(prefix, "/updateuser"), server.NewServer(
		endpoints.UpdateUserEndpoint,
		decodeUpdateUserRequest,
		encodeUpdateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /findbyemail
	router.Handle("GET "+routePath(prefix, "/findbyemail"), server.NewServer(
		endpoints.FindByEmailEndpoint,
		decodeFindByEmailRequest,
		encodeFindByEmailResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /searchusers
	router.Handle("GET "+routePath(prefix, "/searchusers"), server.NewServer(
		endpoints.SearchUsersEndpoint,
		decodeSearchUsersRequest,
		encodeSearchUsersResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /querystats
	router.Handle("GET "+routePath(prefix, "/querystats"), server.NewServer(
		endpoints.QueryStatsEndpoint,
		decodeQueryStatsRequest,
		encodeQueryStatsResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// DELETE /removeexpired
	router.Handle("DELETE "+routePath(prefix, "/removeexpired"), server.NewServer(
		endpoints.RemoveExpiredEndpoint,
		decodeRemoveExpiredRequest,
		encodeRemoveExpiredResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// PUT /editprofile
	router.Handle("PUT "+routePath(prefix, "/editprofile"), server.NewServer(
		endpoints.EditProfileEndpoint,
		decodeEditProfileRequest,
		encodeEditProfileResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// PUT /modifyemail
	router.Handle("PUT "+routePath(prefix, "/modifyemail"), server.NewServer(
		endpoints.ModifyEmailEndpoint,
		decodeModifyEmailRequest,
		encodeModifyEmailResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// PUT /patchstatus
	router.Handle("PUT "+routePath(prefix, "/patchstatus"), server.NewServer(
		endpoints.PatchStatusEndpoint,
		decodePatchStatusRequest,
		encodePatchStatusResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func registerHTTPServeMuxRoutes(m *http.ServeMux, endpoints genendpoint.UserServiceEndpoints) {

	m.Handle("POST /createuser", server.NewServer(
		endpoints.CreateUserEndpoint,
		decodeCreateUserRequest,
		encodeCreateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /getuser", server.NewServer(
		endpoints.GetUserEndpoint,
		decodeGetUserRequest,
		encodeGetUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /listusers", server.NewServer(
		endpoints.ListUsersEndpoint,
		decodeListUsersRequest,
		encodeListUsersResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("DELETE /deleteuser", server.NewServer(
		endpoints.DeleteUserEndpoint,
		decodeDeleteUserRequest,
		encodeDeleteUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("PUT /updateuser", server.NewServer(
		endpoints.UpdateUserEndpoint,
		decodeUpdateUserRequest,
		encodeUpdateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /findbyemail", server.NewServer(
		endpoints.FindByEmailEndpoint,
		decodeFindByEmailRequest,
		encodeFindByEmailResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /searchusers", server.NewServer(
		endpoints.SearchUsersEndpoint,
		decodeSearchUsersRequest,
		encodeSearchUsersResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /querystats", server.NewServer(
		endpoints.QueryStatsEndpoint,
		decodeQueryStatsRequest,
		encodeQueryStatsResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("DELETE /removeexpired", server.NewServer(
		endpoints.RemoveExpiredEndpoint,
		decodeRemoveExpiredRequest,
		encodeRemoveExpiredResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("PUT /editprofile", server.NewServer(
		endpoints.EditProfileEndpoint,
		decodeEditProfileRequest,
		encodeEditProfileResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("PUT /modifyemail", server.NewServer(
		endpoints.ModifyEmailEndpoint,
		decodeModifyEmailRequest,
		encodeModifyEmailResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("PUT /patchstatus", server.NewServer(
		endpoints.PatchStatusEndpoint,
		decodePatchStatusRequest,
		encodePatchStatusResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func routePath(prefix, route string) string {
	if prefix == "" {
		return route
	}
	return prefix + route
}

// decodeCreateUserRequest uses the generated method-aware decode path.
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

// decodeGetUserRequest uses the generated method-aware decode path.
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
	if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeGetUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeListUsersRequest uses the generated method-aware decode path.
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
	if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeListUsersResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeDeleteUserRequest uses the generated method-aware decode path.
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

// decodeUpdateUserRequest uses the generated method-aware decode path.
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

// decodeFindByEmailRequest uses the generated method-aware decode path.
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
	if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeFindByEmailResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeSearchUsersRequest uses the generated method-aware decode path.
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
	if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeSearchUsersResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeQueryStatsRequest uses the generated method-aware decode path.
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
	if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeQueryStatsResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeRemoveExpiredRequest uses the generated method-aware decode path.
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

// decodeEditProfileRequest uses the generated method-aware decode path.
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

// decodeModifyEmailRequest uses the generated method-aware decode path.
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

// decodePatchStatusRequest uses the generated method-aware decode path.
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
