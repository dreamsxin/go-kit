package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
)

// NewJSONServer creates an HTTP server that automatically handles JSON
// encoding/decoding for the given handler function.
//
// JSONErrorEncoder is used by default — errors are written as
// {"code": "...", "message": "..."} with an appropriate HTTP status code.
// Pass ServerErrorEncoder to override.
//
// Example:
//
//	handler := server.NewJSONServer[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
//	    return HelloResp{...}, nil
//	})
//	http.ListenAndServe(":8080", handler)
func NewJSONServer[Req any](
	handler func(ctx context.Context, req Req) (any, error),
	options ...ServerOption,
) *Server {
	e := endpoint.Endpoint(func(ctx context.Context, request any) (any, error) {
		return handler(ctx, request.(Req))
	})
	return NewJSONEndpoint[Req](e, options...)
}

// NewJSONEndpoint creates a strict JSON HTTP server for an existing
// endpoint.Endpoint. Use this when you have already built your endpoint with
// middleware.
func NewJSONEndpoint[Req any](
	e endpoint.Endpoint,
	options ...ServerOption,
) *Server {
	return NewJSONEndpointWithDecodeOptions[Req](e, StrictJSONDecodeOptions(DefaultMaxJSONBodyBytes), options...)
}

// NewJSONEndpointWithDecodeOptions creates an HTTP server for an existing
// endpoint.Endpoint with explicit JSON request decoding options.
func NewJSONEndpointWithDecodeOptions[Req any](
	e endpoint.Endpoint,
	decodeOptions JSONDecodeOptions,
	options ...ServerOption,
) *Server {
	opts := append([]ServerOption{ServerErrorEncoder(JSONErrorEncoder)}, options...)
	return NewServer(e, DecodeJSONRequestWithOptions[Req](decodeOptions), EncodeJSONResponse, opts...)
}

// NewStrictJSONEndpoint creates an HTTP server for public JSON APIs. It
// rejects unknown fields, trailing data, and bodies larger than maxBodyBytes.
func NewStrictJSONEndpoint[Req any](
	e endpoint.Endpoint,
	maxBodyBytes int64,
	options ...ServerOption,
) *Server {
	return NewJSONEndpointWithDecodeOptions[Req](e, StrictJSONDecodeOptions(maxBodyBytes), options...)
}

// NewStrictJSONServer creates a strict JSON HTTP server around a handler
// function with an explicit body size limit.
func NewStrictJSONServer[Req any](
	handler func(ctx context.Context, req Req) (any, error),
	maxBodyBytes int64,
	options ...ServerOption,
) *Server {
	e := endpoint.Endpoint(func(ctx context.Context, request any) (any, error) {
		return handler(ctx, request.(Req))
	})
	return NewStrictJSONEndpoint[Req](e, maxBodyBytes, options...)
}

// NewJSONServerWithMiddleware is a convenience wrapper that combines
// a handler function with a middleware chain built via endpoint.Builder.
func NewJSONServerWithMiddleware[Req any](
	handler func(ctx context.Context, req Req) (any, error),
	middleware func(*endpoint.Builder) *endpoint.Builder,
	options ...ServerOption,
) *Server {
	e := endpoint.Endpoint(func(ctx context.Context, request any) (any, error) {
		return handler(ctx, request.(Req))
	})
	ep := middleware(endpoint.NewBuilder(e)).Build()
	return NewJSONEndpoint[Req](ep, options...)
}

// DecodeJSONRequest returns a DecodeRequestFunc that strictly decodes the HTTP
// request body as JSON into a value of type T.
func DecodeJSONRequest[T any]() DecodeRequestFunc {
	return func(_ context.Context, r *http.Request) (any, error) {
		var v T
		if err := DecodeJSONBody(r, &v, StrictJSONDecodeOptions(DefaultMaxJSONBodyBytes)); err != nil {
			return nil, JSONDecodeError{Err: err}
		}
		return v, nil
	}
}

// DecodeJSONRequestWithOptions returns a DecodeRequestFunc that decodes the
// HTTP request body as JSON into T using the supplied options.
func DecodeJSONRequestWithOptions[T any](options JSONDecodeOptions) DecodeRequestFunc {
	return func(_ context.Context, r *http.Request) (any, error) {
		var v T
		if err := DecodeJSONBody(r, &v, options); err != nil {
			return nil, JSONDecodeError{Err: err}
		}
		return v, nil
	}
}

// JSONDecodeOptions controls optional safety checks for JSON request bodies.
// A zero value disables the optional checks.
type JSONDecodeOptions struct {
	// MaxBodyBytes limits the request body. A value <= 0 means unlimited.
	MaxBodyBytes int64
	// DisallowUnknownFields rejects object fields that are not in the target type.
	DisallowUnknownFields bool
	// RejectTrailingData requires exactly one JSON value followed by whitespace.
	RejectTrailingData bool
}

// DefaultMaxJSONBodyBytes is the default body limit used by generated and
// high-level strict JSON helpers.
const DefaultMaxJSONBodyBytes int64 = 1 << 20

// StrictJSONDecodeOptions returns options suitable for public JSON APIs.
func StrictJSONDecodeOptions(maxBodyBytes int64) JSONDecodeOptions {
	return JSONDecodeOptions{
		MaxBodyBytes:          maxBodyBytes,
		DisallowUnknownFields: true,
		RejectTrailingData:    true,
	}
}

var (
	// ErrJSONBodyTooLarge indicates that a JSON request body exceeded MaxBodyBytes.
	ErrJSONBodyTooLarge = errors.New("json request body too large")
	// ErrJSONTrailingData indicates that a JSON request contained more than one value.
	ErrJSONTrailingData = errors.New("json request body contains trailing data")
)

// JSONDecodeError marks request body decode failures as client errors while
// preserving the underlying error for errors.Is/errors.As.
type JSONDecodeError struct {
	Err error
}

func (e JSONDecodeError) Error() string {
	if e.Err == nil {
		return "invalid JSON request body"
	}
	return e.Err.Error()
}

func (e JSONDecodeError) Unwrap() error {
	return e.Err
}

func (e JSONDecodeError) StatusCode() int {
	return http.StatusBadRequest
}

func (e JSONDecodeError) ErrorCode() string {
	return "bad_request.invalid_json"
}

// DecodeJSONBody decodes one JSON value from r into target.
// The request body remains owned by the caller.
func DecodeJSONBody(r *http.Request, target any, options JSONDecodeOptions) error {
	if r == nil {
		return errors.New("nil HTTP request")
	}

	var reader io.Reader = r.Body
	if options.MaxBodyBytes > 0 {
		reader = &limitedBodyReader{reader: r.Body, remaining: options.MaxBodyBytes}
	}

	decoder := json.NewDecoder(reader)
	if options.DisallowUnknownFields {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if !options.RejectTrailingData {
		return nil
	}

	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if errors.Is(err, ErrJSONBodyTooLarge) {
		return err
	}
	if err == nil {
		return ErrJSONTrailingData
	}
	return fmt.Errorf("%w: %v", ErrJSONTrailingData, err)
}

type limitedBodyReader struct {
	reader    io.Reader
	remaining int64
}

func (r *limitedBodyReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, ErrJSONBodyTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}
