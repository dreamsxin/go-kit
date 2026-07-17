package userservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
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
	router.Handle(routePath(prefix, "/createuser"), server.NewServer(
		endpoints.CreateUserEndpoint,
		decodeCreateUserRequest,
		encodeCreateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("POST")

	// GET /getuser
	router.Handle(routePath(prefix, "/getuser"), server.NewServer(
		endpoints.GetUserEndpoint,
		decodeGetUserRequest,
		encodeGetUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// GET /listusers
	router.Handle(routePath(prefix, "/listusers"), server.NewServer(
		endpoints.ListUsersEndpoint,
		decodeListUsersRequest,
		encodeListUsersResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// DELETE /deleteuser
	router.Handle(routePath(prefix, "/deleteuser"), server.NewServer(
		endpoints.DeleteUserEndpoint,
		decodeDeleteUserRequest,
		encodeDeleteUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("DELETE")

	// PUT /updateuser
	router.Handle(routePath(prefix, "/updateuser"), server.NewServer(
		endpoints.UpdateUserEndpoint,
		decodeUpdateUserRequest,
		encodeUpdateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("PUT")

	// GET /findbyemail
	router.Handle(routePath(prefix, "/findbyemail"), server.NewServer(
		endpoints.FindByEmailEndpoint,
		decodeFindByEmailRequest,
		encodeFindByEmailResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// GET /searchusers
	router.Handle(routePath(prefix, "/searchusers"), server.NewServer(
		endpoints.SearchUsersEndpoint,
		decodeSearchUsersRequest,
		encodeSearchUsersResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// GET /querystats
	router.Handle(routePath(prefix, "/querystats"), server.NewServer(
		endpoints.QueryStatsEndpoint,
		decodeQueryStatsRequest,
		encodeQueryStatsResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// DELETE /removeexpired
	router.Handle(routePath(prefix, "/removeexpired"), server.NewServer(
		endpoints.RemoveExpiredEndpoint,
		decodeRemoveExpiredRequest,
		encodeRemoveExpiredResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("DELETE")

	// PUT /editprofile
	router.Handle(routePath(prefix, "/editprofile"), server.NewServer(
		endpoints.EditProfileEndpoint,
		decodeEditProfileRequest,
		encodeEditProfileResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("PUT")

	// PUT /modifyemail
	router.Handle(routePath(prefix, "/modifyemail"), server.NewServer(
		endpoints.ModifyEmailEndpoint,
		decodeModifyEmailRequest,
		encodeModifyEmailResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("PUT")

	// PUT /patchstatus
	router.Handle(routePath(prefix, "/patchstatus"), server.NewServer(
		endpoints.PatchStatusEndpoint,
		decodePatchStatusRequest,
		encodePatchStatusResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("PUT")

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


var (
	queryTimeType     = reflect.TypeOf(time.Time{})
	queryDurationType = reflect.TypeOf(time.Duration(0))
)

type queryDecodeError struct {
	err error
}

func (e queryDecodeError) Error() string {
	if e.err == nil {
		return "invalid query request"
	}
	return e.err.Error()
}

func (e queryDecodeError) Unwrap() error {
	return e.err
}

func (e queryDecodeError) StatusCode() int {
	return http.StatusBadRequest
}

func (e queryDecodeError) ErrorCode() string {
	return "bad_request.invalid_query"
}

func decodeQueryRequest(r *http.Request, target any) error {
	if r == nil {
		return fmt.Errorf("nil HTTP request")
	}
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return fmt.Errorf("query target must be a non-nil pointer")
	}
	elem := value.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("query target must point to a struct")
	}
	return decodeQueryStruct(r, elem, r.URL.Query(), mux.Vars(r))
}

func decodeQueryStruct(r *http.Request, target reflect.Value, values url.Values, pathVars map[string]string) error {
	targetType := target.Type()
	for i := 0; i < target.NumField(); i++ {
		fieldInfo := targetType.Field(i)
		field := target.Field(i)
		if fieldInfo.PkgPath != "" || !field.CanSet() {
			continue
		}
		if fieldInfo.Anonymous && indirectKind(field) == reflect.Struct {
			if err := decodeQueryStruct(r, indirectValue(field), values, pathVars); err != nil {
				return err
			}
			continue
		}
		names := queryFieldNames(fieldInfo)
		if len(names) == 0 {
			continue
		}
		raw, ok := queryFieldValue(r, values, pathVars, names)
		if !ok {
			continue
		}
		if err := setQueryField(field, raw); err != nil {
			return fmt.Errorf("%s: %w", fieldInfo.Name, err)
		}
	}
	return nil
}

func queryFieldNames(field reflect.StructField) []string {
	var names []string
	for _, tagName := range []string{"form", "json"} {
		tag := field.Tag.Get(tagName)
		if tag == "-" {
			return nil
		}
		if name := strings.TrimSpace(strings.Split(tag, ",")[0]); name != "" {
			names = append(names, name)
		}
	}
	names = append(names, field.Name, strings.ToLower(field.Name))
	return names
}

func queryFieldValue(r *http.Request, values url.Values, pathVars map[string]string, names []string) (string, bool) {
	for _, name := range names {
		if vals, ok := values[name]; ok && len(vals) > 0 {
			return vals[len(vals)-1], true
		}
	}
	for _, name := range names {
		if value, ok := pathVars[name]; ok {
			return value, true
		}
	}
	for _, name := range names {
		if value := r.PathValue(name); value != "" {
			return value, true
		}
	}
	return "", false
}

func setQueryField(field reflect.Value, raw string) error {
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setQueryField(field.Elem(), raw)
	}
	if field.Type() == queryTimeType {
		parsed, err := parseQueryTime(raw)
		if err != nil {
			return err
		}
		field.Set(reflect.ValueOf(parsed))
		return nil
	}
	if field.Type() == queryDurationType {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		field.SetInt(int64(parsed))
		return nil
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		field.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		parsed, err := strconv.ParseUint(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(raw, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(parsed)
	default:
		return fmt.Errorf("unsupported query field type %s", field.Type())
	}
	return nil
}

func parseQueryTime(raw string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02", raw)
}

func indirectKind(v reflect.Value) reflect.Kind {
	return indirectValue(v).Kind()
}

func indirectValue(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return v.Elem()
	}
	return v
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
	if err := decodeQueryRequest(r, &req); err != nil {
		return nil, queryDecodeError{err: err}
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
	if err := decodeQueryRequest(r, &req); err != nil {
		return nil, queryDecodeError{err: err}
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
	if err := decodeQueryRequest(r, &req); err != nil {
		return nil, queryDecodeError{err: err}
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
	if err := decodeQueryRequest(r, &req); err != nil {
		return nil, queryDecodeError{err: err}
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
	if err := decodeQueryRequest(r, &req); err != nil {
		return nil, queryDecodeError{err: err}
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

