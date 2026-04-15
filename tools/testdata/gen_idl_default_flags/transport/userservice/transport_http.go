package userservice

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dreamsxin/go-kit/transport/http/server"
	idl "example.com/gen_idl_default_flags"
	genendpoint "example.com/gen_idl_default_flags/endpoint/userservice"
)

// NewHTTPHandler returns the generated HTTP handler set.
func NewHTTPHandler(endpoints genendpoint.UserServiceEndpoints) http.Handler {
	m := http.NewServeMux()


	// POST /createuser
	m.Handle("POST /createuser", server.NewJSONEndpoint[idl.CreateUserRequest](
		endpoints.CreateUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /getuser
	m.Handle("GET /getuser", server.NewJSONEndpoint[idl.GetUserRequest](
		endpoints.GetUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /listusers
	m.Handle("GET /listusers", server.NewJSONEndpoint[idl.ListUsersRequest](
		endpoints.ListUsersEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// DELETE /deleteuser
	m.Handle("DELETE /deleteuser", server.NewJSONEndpoint[idl.DeleteUserRequest](
		endpoints.DeleteUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// PUT /updateuser
	m.Handle("PUT /updateuser", server.NewJSONEndpoint[idl.UpdateUserRequest](
		endpoints.UpdateUserEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /findbyemail
	m.Handle("GET /findbyemail", server.NewJSONEndpoint[idl.GetUserRequest](
		endpoints.FindByEmailEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /searchusers
	m.Handle("GET /searchusers", server.NewJSONEndpoint[idl.ListUsersRequest](
		endpoints.SearchUsersEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /querystats
	m.Handle("GET /querystats", server.NewJSONEndpoint[idl.GetUserRequest](
		endpoints.QueryStatsEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// DELETE /removeexpired
	m.Handle("DELETE /removeexpired", server.NewJSONEndpoint[idl.DeleteUserRequest](
		endpoints.RemoveExpiredEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// PUT /editprofile
	m.Handle("PUT /editprofile", server.NewJSONEndpoint[idl.UpdateUserRequest](
		endpoints.EditProfileEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// PUT /modifyemail
	m.Handle("PUT /modifyemail", server.NewJSONEndpoint[idl.UpdateUserRequest](
		endpoints.ModifyEmailEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// PUT /patchstatus
	m.Handle("PUT /patchstatus", server.NewJSONEndpoint[idl.UpdateUserRequest](
		endpoints.PatchStatusEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))


	return m
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

// decodeListUsersRequest uses the default JSON decode path.
//
// @Summary      ListUsers lists all users.
// @Description  ListUsers lists all users.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.ListUsersRequest  true  "ListUsers request"
// @Success      200      {object}  idl.ListUsersResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /listusers [get]
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

// decodeDeleteUserRequest uses the default JSON decode path.
//
// @Summary      DeleteUser removes a user.
// @Description  DeleteUser removes a user.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.DeleteUserRequest  true  "DeleteUser request"
// @Success      200      {object}  idl.DeleteUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /deleteuser [delete]
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

// decodeUpdateUserRequest uses the default JSON decode path.
//
// @Summary      UpdateUser modifies a user.
// @Description  UpdateUser modifies a user.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.UpdateUserRequest  true  "UpdateUser request"
// @Success      200      {object}  idl.UpdateUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /updateuser [put]
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

// decodeFindByEmailRequest uses the default JSON decode path.
//
// @Summary      FindByEmail finds users by email prefix.
// @Description  FindByEmail finds users by email prefix.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.GetUserRequest  true  "FindByEmail request"
// @Success      200      {object}  idl.GetUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /findbyemail [get]
func decodeFindByEmailRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.GetUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeFindByEmailResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeSearchUsersRequest uses the default JSON decode path.
//
// @Summary      SearchUsers searches users.
// @Description  SearchUsers searches users.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.ListUsersRequest  true  "SearchUsers request"
// @Success      200      {object}  idl.ListUsersResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /searchusers [get]
func decodeSearchUsersRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.ListUsersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeSearchUsersResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeQueryStatsRequest uses the default JSON decode path.
//
// @Summary      QueryStats returns statistics.
// @Description  QueryStats returns statistics.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.GetUserRequest  true  "QueryStats request"
// @Success      200      {object}  idl.GetUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /querystats [get]
func decodeQueryStatsRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.GetUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeQueryStatsResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeRemoveExpiredRequest uses the default JSON decode path.
//
// @Summary      RemoveExpired removes expired users.
// @Description  RemoveExpired removes expired users.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.DeleteUserRequest  true  "RemoveExpired request"
// @Success      200      {object}  idl.DeleteUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /removeexpired [delete]
func decodeRemoveExpiredRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.DeleteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeRemoveExpiredResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeEditProfileRequest uses the default JSON decode path.
//
// @Summary      EditProfile edits profile.
// @Description  EditProfile edits profile.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.UpdateUserRequest  true  "EditProfile request"
// @Success      200      {object}  idl.UpdateUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /editprofile [put]
func decodeEditProfileRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeEditProfileResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodeModifyEmailRequest uses the default JSON decode path.
//
// @Summary      ModifyEmail modifies email.
// @Description  ModifyEmail modifies email.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.UpdateUserRequest  true  "ModifyEmail request"
// @Success      200      {object}  idl.UpdateUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /modifyemail [put]
func decodeModifyEmailRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodeModifyEmailResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

// decodePatchStatusRequest uses the default JSON decode path.
//
// @Summary      PatchStatus patches status.
// @Description  PatchStatus patches status.
// @Tags         UserService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.UpdateUserRequest  true  "PatchStatus request"
// @Success      200      {object}  idl.UpdateUserResponse
// @Failure      400      {object}  string
// @Failure      500      {object}  string
// @Router       /patchstatus [put]
func decodePatchStatusRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodePatchStatusResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

