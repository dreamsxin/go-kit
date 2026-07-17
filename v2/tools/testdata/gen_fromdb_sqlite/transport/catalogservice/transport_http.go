package catalogservice

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
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
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
	router.Handle(routePath(prefix, "/user"), server.NewServer(
		endpoints.CreateUserEndpoint,
		decodeCreateUserRequest,
		encodeCreateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("POST")

	// GET /user/{id}
	router.Handle(routePath(prefix, "/user/{id}"), server.NewServer(
		endpoints.GetUserEndpoint,
		decodeGetUserRequest,
		encodeGetUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

	// PUT /user/{id}
	router.Handle(routePath(prefix, "/user/{id}"), server.NewServer(
		endpoints.UpdateUserEndpoint,
		decodeUpdateUserRequest,
		encodeUpdateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("PUT")

	// DELETE /user/{id}
	router.Handle(routePath(prefix, "/user/{id}"), server.NewServer(
		endpoints.DeleteUserEndpoint,
		decodeDeleteUserRequest,
		encodeDeleteUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("DELETE")

	// GET /users
	router.Handle(routePath(prefix, "/users"), server.NewServer(
		endpoints.ListUsersEndpoint,
		decodeListUsersRequest,
		encodeListUsersResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("GET")

}

func registerHTTPServeMuxRoutes(m *http.ServeMux, endpoints genendpoint.CatalogServiceEndpoints) {

	m.Handle("POST /user", server.NewServer(
		endpoints.CreateUserEndpoint,
		decodeCreateUserRequest,
		encodeCreateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /user/{id}", server.NewServer(
		endpoints.GetUserEndpoint,
		decodeGetUserRequest,
		encodeGetUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("PUT /user/{id}", server.NewServer(
		endpoints.UpdateUserEndpoint,
		decodeUpdateUserRequest,
		encodeUpdateUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("DELETE /user/{id}", server.NewServer(
		endpoints.DeleteUserEndpoint,
		decodeDeleteUserRequest,
		encodeDeleteUserResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	m.Handle("GET /users", server.NewServer(
		endpoints.ListUsersEndpoint,
		decodeListUsersRequest,
		encodeListUsersResponse,
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
// @Summary      Create User
// @Description  Create User
// @Tags         CatalogService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.CreateUserRequest  true  "CreateUser request"
// @Success      200      {object}  idl.CreateUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /user [post]
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
// @Summary      Get User
// @Description  Get User details
// @Tags         CatalogService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.GetUserRequest  true  "GetUser request"
// @Success      200      {object}  idl.GetUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /user/{id} [get]
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

// decodeUpdateUserRequest uses the generated method-aware decode path.
//
// @Summary      Update User
// @Description  Update User
// @Tags         CatalogService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.UpdateUserRequest  true  "UpdateUser request"
// @Success      200      {object}  idl.UpdateUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /user/{id} [put]
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

// decodeDeleteUserRequest uses the generated method-aware decode path.
//
// @Summary      Delete User
// @Description  Delete User
// @Tags         CatalogService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.DeleteUserRequest  true  "DeleteUser request"
// @Success      200      {object}  idl.DeleteUserResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /user/{id} [delete]
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

// decodeListUsersRequest uses the generated method-aware decode path.
//
// @Summary      List Users
// @Description  List Users
// @Tags         CatalogService
// @Accept       json
// @Produce      json
// @Param        request  query     idl.ListUsersRequest  true  "ListUsers request"
// @Success      200      {object}  idl.ListUsersResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /users [get]
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
