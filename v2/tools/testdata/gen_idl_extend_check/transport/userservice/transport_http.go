package userservice

import (
	"context"
	"encoding/json"
	idl "example.com/gen_idl_extend_check"
	genendpoint "example.com/gen_idl_extend_check/endpoint/userservice"
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

// decodeListUsersRequest uses the generated method-aware decode path.
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
func decodeDeleteUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.DeleteUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	if err := transporthttp.DecodePathRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeDeleteUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeUpdateUserRequest uses the generated method-aware decode path.
func decodeUpdateUserRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	if err := transporthttp.DecodePathRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeUpdateUserResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeFindByEmailRequest uses the generated method-aware decode path.
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
func decodeRemoveExpiredRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.DeleteUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	if err := transporthttp.DecodePathRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeRemoveExpiredResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeEditProfileRequest uses the generated method-aware decode path.
func decodeEditProfileRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	if err := transporthttp.DecodePathRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeEditProfileResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeModifyEmailRequest uses the generated method-aware decode path.
func decodeModifyEmailRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	if err := transporthttp.DecodePathRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeModifyEmailResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodePatchStatusRequest uses the generated method-aware decode path.
func decodePatchStatusRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	if err := transporthttp.DecodePathRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodePatchStatusResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}
