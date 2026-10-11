package kit

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
	"github.com/dreamsxin/go-kit/v2/validation"
)

// Stable: kit.typed-request-decoding — a typed JSON route fills the request
// struct from the request itself, one rule everywhere. GET and HEAD read the
// query string and the path values; every other method reads a strictly
// decoded JSON body and then the path values. A request that carries no body
// at all decodes to the zero value — pressing a button without typing anything
// is a validation story, not a transport error, so the handler answers it with
// the application's own message — while a body that exists is judged strictly:
// unknown fields, trailing data, oversized bodies and non-JSON media types are
// 400, 413 and 415 as before. Fields are named by the form tag, else the json
// tag, else the lowercased field name, and a path value wins over a query
// parameter of the same name — the same rules the generated transports decode
// with. A validate tag on the request struct is enforced after the fill, by
// the same package the generated transports call, so a hand-assembled service
// and a generated one reject the same request with the same error. A
// non-struct request (a map, a slice, a named scalar) cannot be filled from
// the URL; it decodes from the body only and carries no field rules.
// Covered by: TestTypedRouteDecodesQueryAndPath, TestTypedRouteDecodesBodyAndPath,
// TestTypedRouteEmptyBodyIsZeroValue, TestTypedRoutePathWinsOverQuery,
// TestTypedRouteStrictnessStays
func typedRequestDecoder[Req any](decodeOptions httpserver.JSONDecodeOptions) httpserver.DecodeRequestFunc {
	// The probe runs once per route at registration, not per request: a
	// struct can be filled from the URL; anything else is a body request.
	var probe Req
	fillsFromURL := reflect.TypeOf(&probe).Elem().Kind() == reflect.Struct
	return func(_ context.Context, r *http.Request) (any, error) {
		var req Req
		switch {
		case !fillsFromURL:
			if err := decodeBody(r, &req, decodeOptions); err != nil {
				return nil, err
			}
		case r.Method == http.MethodGet, r.Method == http.MethodHead:
			if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
				return nil, err
			}
			if err := validation.Validate(req); err != nil {
				return nil, err
			}
		default:
			if err := decodeBody(r, &req, decodeOptions); err != nil {
				return nil, err
			}
			if err := transporthttp.DecodePathRequest(r, &req); err != nil {
				return nil, err
			}
			if err := validation.Validate(req); err != nil {
				return nil, err
			}
		}
		return req, nil
	}
}

// decodeBody strictly decodes the JSON body when one is sent. An empty body is
// "nothing sent": it decodes to the zero value so a body-less request reaches
// the handler, whose validation answers in the application's own words. A
// body that exists is judged by the transport rules unchanged: a media type the
// route does not speak stays 415, unknown fields and trailing data stay 400, an
// oversized body stays 413.
func decodeBody(r *http.Request, target any, decodeOptions httpserver.JSONDecodeOptions) error {
	if err := httpserver.DecodeJSONBody(r, target, decodeOptions); err != nil {
		if errors.Is(err, httpserver.ErrJSONBodyEmpty) {
			return nil
		}
		return decodeBodyFailure(err)
	}
	return nil
}

// decodeBodyFailure classifies a body decode failure the same way the server
// package does, so the wire answers do not differ between kit and the
// transport assembled by hand: a media type the route does not speak stays
// 415 rather than being renamed a body failure.
func decodeBodyFailure(err error) error {
	var mediaType httpserver.UnsupportedMediaTypeError
	if errors.As(err, &mediaType) {
		return mediaType
	}
	return httpserver.JSONDecodeError{Err: err}
}
