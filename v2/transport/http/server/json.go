package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// NewJSONServer creates an HTTP server that automatically handles JSON
// encoding/decoding for the given handler function.
//
// Every JSON entry point in this package decodes strictly: unknown object
// fields, a second JSON value, and bodies over DefaultMaxJSONBodyBytes are
// rejected with 400. Use the *WithBodyLimit variants for a different size cap
// and NewJSONEndpointWithDecodeOptions to change strictness itself.
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
	return NewJSONEndpoint[Req](endpoint.TypedEndpoint[Req, any](handler).Wrap(), options...)
}

// NewTypedJSONServer creates an HTTP server with compile-time request and
// response types.
func NewTypedJSONServer[Req, Resp any](
	handler func(ctx context.Context, req Req) (Resp, error),
	options ...ServerOption,
) *Server {
	return NewJSONEndpoint[Req](endpoint.TypedEndpoint[Req, Resp](handler).Wrap(), options...)
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

// NewJSONEndpointWithBodyLimit is NewJSONEndpoint with an explicit maximum
// request body size instead of DefaultMaxJSONBodyBytes.
func NewJSONEndpointWithBodyLimit[Req any](
	e endpoint.Endpoint,
	maxBodyBytes int64,
	options ...ServerOption,
) *Server {
	return NewJSONEndpointWithDecodeOptions[Req](e, StrictJSONDecodeOptions(maxBodyBytes), options...)
}

// NewJSONServerWithBodyLimit is NewJSONServer with an explicit maximum request
// body size instead of DefaultMaxJSONBodyBytes.
func NewJSONServerWithBodyLimit[Req any](
	handler func(ctx context.Context, req Req) (any, error),
	maxBodyBytes int64,
	options ...ServerOption,
) *Server {
	return NewJSONEndpointWithBodyLimit[Req](endpoint.TypedEndpoint[Req, any](handler).Wrap(), maxBodyBytes, options...)
}

// NewTypedJSONServerWithBodyLimit is NewTypedJSONServer with an explicit
// maximum request body size instead of DefaultMaxJSONBodyBytes.
func NewTypedJSONServerWithBodyLimit[Req, Resp any](
	handler func(ctx context.Context, req Req) (Resp, error),
	maxBodyBytes int64,
	options ...ServerOption,
) *Server {
	return NewJSONEndpointWithBodyLimit[Req](endpoint.TypedEndpoint[Req, Resp](handler).Wrap(), maxBodyBytes, options...)
}

// NewJSONServerWithMiddleware is a convenience wrapper that combines
// a handler function with a middleware chain built via endpoint.Builder.
func NewJSONServerWithMiddleware[Req any](
	handler func(ctx context.Context, req Req) (any, error),
	middleware func(*endpoint.Builder) *endpoint.Builder,
	options ...ServerOption,
) *Server {
	e := endpoint.TypedEndpoint[Req, any](handler).Wrap()
	ep := middleware(endpoint.NewBuilder(e)).Build()
	return NewJSONEndpoint[Req](ep, options...)
}

// NewTypedJSONServerWithMiddleware combines a fully typed handler with an
// endpoint middleware chain.
func NewTypedJSONServerWithMiddleware[Req, Resp any](
	handler func(ctx context.Context, req Req) (Resp, error),
	middleware func(*endpoint.Builder) *endpoint.Builder,
	options ...ServerOption,
) *Server {
	ep := middleware(endpoint.NewTypedBuilder(endpoint.TypedEndpoint[Req, Resp](handler))).Build()
	return NewJSONEndpoint[Req](ep, options...)
}

// DecodeJSONRequest returns a DecodeRequestFunc that strictly decodes the HTTP
// request body as JSON into a value of type T.
func DecodeJSONRequest[T any]() DecodeRequestFunc {
	return func(_ context.Context, r *http.Request) (any, error) {
		var v T
		if err := DecodeJSONBody(r, &v, StrictJSONDecodeOptions(DefaultMaxJSONBodyBytes)); err != nil {
			return nil, decodeFailure(err)
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
			return nil, decodeFailure(err)
		}
		return v, nil
	}
}

// decodeFailure classifies a decode error for the wire.
//
// A refused media type is passed through as itself rather than wrapped: the body
// was never read, so calling it a body decode failure would answer 400 for a
// question HTTP answers with 415.
func decodeFailure(err error) error {
	var mediaType UnsupportedMediaTypeError
	if errors.As(err, &mediaType) {
		return mediaType
	}
	return JSONDecodeError{Err: err}
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
	// RequireJSONContentType answers a request whose media type is not JSON with
	// 415, before the body is read.
	//
	// A request with no Content-Type at all is accepted: a body-less request
	// carries none, and demanding one would refuse requests that are correct.
	// What this rejects is a caller that named a type the route does not speak,
	// which without the check was accepted whenever the bytes happened to parse.
	RequireJSONContentType bool
}

// DefaultMaxJSONBodyBytes is the default body limit used by generated and
// high-level strict JSON helpers.
const DefaultMaxJSONBodyBytes int64 = 1 << 20

// StrictJSONDecodeOptions returns options suitable for public JSON APIs.
func StrictJSONDecodeOptions(maxBodyBytes int64) JSONDecodeOptions {
	return JSONDecodeOptions{
		MaxBodyBytes:           maxBodyBytes,
		DisallowUnknownFields:  true,
		RejectTrailingData:     true,
		RequireJSONContentType: true,
	}
}

var (
	// ErrJSONBodyTooLarge indicates that a JSON request body exceeded MaxBodyBytes.
	ErrJSONBodyTooLarge = errors.New("json request body too large")
	// ErrJSONTrailingData indicates that a JSON request contained more than one value.
	ErrJSONTrailingData = errors.New("json request body contains trailing data")
	// ErrJSONBodyEmpty indicates that a request that needed a JSON body had none.
	// It is separated from a parse failure because "EOF" — which is what the
	// decoder reports and what the caller used to receive — describes the
	// decoder's situation rather than the caller's mistake.
	ErrJSONBodyEmpty = errors.New("request body is empty")
)

// UnsupportedMediaTypeError reports a request whose media type the route does
// not speak. It classifies as 415, the status HTTP defines for exactly this.
//
// The received type is kept for logs and is not put on the wire: the value is
// caller-supplied, and a public error message is not the place to echo it.
type UnsupportedMediaTypeError struct {
	Received string
}

func (e UnsupportedMediaTypeError) Error() string { return "unsupported media type" }

func (e UnsupportedMediaTypeError) StatusCode() int { return http.StatusUnsupportedMediaType }

func (e UnsupportedMediaTypeError) ErrorCode() string { return "unsupported_media_type" }

func (e UnsupportedMediaTypeError) PublicMessage() string { return "unsupported media type" }

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
	// An over-limit body is not a malformed body. It classifies as 413, the
	// same status ParseMultipartForm uses and the one RawBodyCodec documents.
	if errors.Is(e.Err, ErrJSONBodyTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func (e JSONDecodeError) ErrorCode() string {
	if errors.Is(e.Err, ErrJSONBodyTooLarge) {
		return "request_too_large"
	}
	if errors.Is(e.Err, ErrJSONBodyEmpty) {
		return "bad_request.empty_body"
	}
	return "bad_request.invalid_json"
}

// DecodeJSONBody decodes one JSON value from r into target.
// The request body remains owned by the caller.
func DecodeJSONBody(r *http.Request, target any, options JSONDecodeOptions) error {
	if r == nil {
		return errors.New("nil HTTP request")
	}
	if options.RequireJSONContentType {
		if contentType := r.Header.Get("Content-Type"); contentType != "" && !isJSONMediaType(contentType) {
			return UnsupportedMediaTypeError{Received: contentType}
		}
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
		if errors.Is(err, io.EOF) {
			return ErrJSONBodyEmpty
		}
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

// isJSONMediaType reports whether a Content-Type names JSON.
//
// It accepts application/json and any structured suffix such as
// application/merge-patch+json, because those are JSON as far as decoding is
// concerned. A charset parameter must be UTF-8: JSON is UTF-8 by RFC 8259, so a
// caller declaring another encoding is describing bytes this decoder will not
// read correctly.
func isJSONMediaType(contentType string) bool {
	mediaType, parameters, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	if charset, ok := parameters["charset"]; ok && !strings.EqualFold(charset, "utf-8") {
		return false
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

// bodyTooLargeError is what limitedBodyReader reports. It unwraps to
// ErrJSONBodyTooLarge so errors.Is keeps working, and classifies itself as 413
// so callers that surface the read error directly — RawBodyCodec does — get the
// status its documentation promises without a decode wrapper in between.
//
// Its message deliberately omits "json": the same reader bounds protobuf and
// other raw bodies, and below 500 the message is what the encoders put on the
// wire, so a protobuf route must not answer with a JSON error string.
type bodyTooLargeError struct{}

func (bodyTooLargeError) Error() string { return "request body too large" }

func (bodyTooLargeError) Unwrap() error { return ErrJSONBodyTooLarge }

func (bodyTooLargeError) StatusCode() int { return http.StatusRequestEntityTooLarge }

func (bodyTooLargeError) ErrorCode() string { return "request_too_large" }

type limitedBodyReader struct {
	reader    io.Reader
	remaining int64
}

func (r *limitedBodyReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, bodyTooLargeError{}
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
