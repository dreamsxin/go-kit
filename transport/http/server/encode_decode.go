package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dreamsxin/go-kit/transport/http/interfaces"
)

// DecodeRequestFunc decodes an *http.Request into a domain request value.
// Implement this to extract path variables, query params, or a JSON body.
type DecodeRequestFunc func(context.Context, *http.Request) (request interface{}, err error)

// NopRequestDecoder is a DecodeRequestFunc that always returns nil.
// Use it when the endpoint does not need any request data.
func NopRequestDecoder(ctx context.Context, r *http.Request) (interface{}, error) {
	return nil, nil
}

// EncodeResponseFunc encodes a domain response value into an http.ResponseWriter.
type EncodeResponseFunc func(context.Context, http.ResponseWriter, interface{}) error

// NopResponseEncoder is an EncodeResponseFunc that discards the response.
// Useful for endpoints that return no body (e.g. 204 No Content).
func NopResponseEncoder(context.Context, http.ResponseWriter, interface{}) error {
	return nil
}

// EncodeJSONResponse is an EncodeResponseFunc that JSON-encodes the response.
// It honours two optional interfaces on the response value:
//   - interfaces.StatusCoder  → uses that HTTP status code (default 200)
//   - interfaces.Headerer     → merges those headers into the response
func EncodeJSONResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if headerer, ok := response.(interfaces.Headerer); ok {
		for k, values := range headerer.Headers() {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
	}
	code := http.StatusOK
	if sc, ok := response.(interfaces.StatusCoder); ok {
		code = sc.StatusCode()
	}
	w.WriteHeader(code)
	if code == http.StatusNoContent {
		return nil
	}
	return json.NewEncoder(w).Encode(response)
}
