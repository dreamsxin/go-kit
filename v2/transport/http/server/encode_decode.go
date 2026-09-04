package server

import (
	"context"
	"encoding/json"
	"net/http"

	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

// DecodeRequestFunc decodes an *http.Request into a domain request value.
// Implement this to extract path variables, query params, or a JSON body.
type DecodeRequestFunc func(context.Context, *http.Request) (request any, err error)

// NopRequestDecoder is a DecodeRequestFunc that always returns nil.
// Use it when the endpoint does not need any request data.
func NopRequestDecoder(ctx context.Context, r *http.Request) (any, error) {
	return nil, nil
}

// EncodeResponseFunc encodes a domain response value into an http.ResponseWriter.
type EncodeResponseFunc func(context.Context, http.ResponseWriter, any) error

// NopResponseEncoder is an EncodeResponseFunc that discards the response.
// Useful for endpoints that return no body (e.g. 204 No Content).
func NopResponseEncoder(context.Context, http.ResponseWriter, any) error {
	return nil
}

// EncodeJSONResponse is an EncodeResponseFunc that JSON-encodes the response.
// It honours two optional interfaces on the response value:
//   - transporthttp.StatusCoder: uses that HTTP status code (default 200)
//   - transporthttp.Headerer: merges those headers into the response
func EncodeJSONResponse(_ context.Context, w http.ResponseWriter, response any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if headerer, ok := response.(transporthttp.Headerer); ok {
		for k, values := range headerer.Headers() {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
	}
	code := responseStatus(response)
	w.WriteHeader(code)
	if code == http.StatusNoContent {
		return nil
	}
	return json.NewEncoder(w).Encode(response)
}

// responseStatus reads a success status off the response value. A StatusCoder
// outside the range net/http accepts is ignored rather than passed to
// WriteHeader, which panics on it — the same range check the error encoders
// already apply, so a response type cannot take the process down.
func responseStatus(response any) int {
	sc, ok := response.(transporthttp.StatusCoder)
	if !ok {
		return http.StatusOK
	}
	if code := sc.StatusCode(); code >= 100 && code <= 999 {
		return code
	}
	return http.StatusOK
}

// WrapJSONResponse returns an EncodeResponseFunc that transforms the response
// value with wrap and then encodes the result exactly like EncodeJSONResponse.
// The original response's Headerer and StatusCoder interfaces are evaluated
// against the original value before wrapping, so status and headers are
// preserved while the body shape changes.
//
// Use it to assemble an envelope without rewriting the encoder:
//
//	server.ServerResponseEncoder(server.WrapJSONResponse(func(response any) any {
//	    return envelope{Code: 0, Message: "ok", Data: response}
//	}))
func WrapJSONResponse(wrap func(response any) any) EncodeResponseFunc {
	if wrap == nil {
		return EncodeJSONResponse
	}
	return func(ctx context.Context, w http.ResponseWriter, response any) error {
		wrapped := wrap(response)

		// Status and headers come from the original response so the envelope
		// does not have to reimplement the transport contracts.
		if headerer, ok := response.(transporthttp.Headerer); ok {
			for k, values := range headerer.Headers() {
				for _, v := range values {
					w.Header().Add(k, v)
				}
			}
		}
		code := responseStatus(response)

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(code)
		if code == http.StatusNoContent {
			return nil
		}
		return json.NewEncoder(w).Encode(wrapped)
	}
}
