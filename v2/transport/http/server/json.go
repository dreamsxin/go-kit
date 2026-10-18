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

// NewJSON creates an HTTP server that decodes a strict JSON request into Req
// and encodes the handler's dynamic response as JSON.
//
// decode states how strictly the request body is read. Pass
// DefaultJSONDecodeOptions for the package default, StrictJSONDecodeOptions(n)
// for the same rules with another size cap, or a JSONDecodeOptions value built
// by the caller. The body limit is part of that value rather than a constructor
// of its own, so a route that accepts a larger body keeps every other rule the
// default installed instead of silently acquiring a different decoder.
//
// Errors are written by JSONErrorEncoder unless ServerErrorEncoder overrides it.
//
// Example:
//
//	handler := server.NewJSON[HelloReq](
//	    func(ctx context.Context, req HelloReq) (any, error) {
//	        return HelloResp{Message: "Hello, " + req.Name}, nil
//	    },
//	    server.DefaultJSONDecodeOptions(),
//	)
//	http.ListenAndServe(":8080", handler)
func NewJSON[Req any](
	handler func(ctx context.Context, req Req) (any, error),
	decode JSONDecodeOptions,
	options ...ServerOption,
) *Server {
	if handler == nil {
		panic("server: JSON handler cannot be nil")
	}
	return NewJSONEndpoint[Req](endpoint.TypedEndpoint[Req, any](handler).Wrap(), decode, options...)
}

// NewTypedJSON creates a JSON HTTP server with compile-time request and
// response types. It is NewJSON for a handler that returns a concrete response
// type rather than any.
func NewTypedJSON[Req, Resp any](
	handler func(ctx context.Context, req Req) (Resp, error),
	decode JSONDecodeOptions,
	options ...ServerOption,
) *Server {
	if handler == nil {
		panic("server: JSON handler cannot be nil")
	}
	return NewJSONEndpoint[Req](endpoint.TypedEndpoint[Req, Resp](handler).Wrap(), decode, options...)
}

// NewJSONEndpoint creates a strict JSON HTTP server for an existing
// endpoint.Endpoint. Use this when the endpoint already carries its middleware
// chain; route-local middleware is composed into the endpoint, not passed here.
func NewJSONEndpoint[Req any](
	e endpoint.Endpoint,
	decode JSONDecodeOptions,
	options ...ServerOption,
) *Server {
	opts := append([]ServerOption{ServerErrorEncoder(JSONErrorEncoder)}, options...)
	return NewServer(e, DecodeJSONRequestWithOptions[Req](decode), EncodeJSONResponse, opts...)
}

// DecodeJSONRequest returns a DecodeRequestFunc that strictly decodes the HTTP
// request body as JSON into a value of type T.
func DecodeJSONRequest[T any]() DecodeRequestFunc {
	return func(_ context.Context, r *http.Request) (any, error) {
		var v T
		if err := DecodeJSONBody(r, &v, DefaultJSONDecodeOptions()); err != nil {
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

// DefaultJSONDecodeOptions returns the strict decoding rules every JSON entry
// point in this package applies when the caller does not choose others:
// unknown fields, trailing data, a non-JSON media type and a body over
// DefaultMaxJSONBodyBytes are all refused.
func DefaultJSONDecodeOptions() JSONDecodeOptions {
	return StrictJSONDecodeOptions(DefaultMaxJSONBodyBytes)
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
//
// Stable: http.media-type-415 — a declared non-JSON body is 415 unsupported_media_type, and the received type is not echoed.
// Covered by: TestUnsupportedMediaTypeIsAnswered415, TestNonUTF8CharsetIsRejected
type UnsupportedMediaTypeError struct {
	Received string
}

func (e UnsupportedMediaTypeError) Error() string { return "unsupported media type" }

func (e UnsupportedMediaTypeError) StatusCode() int { return http.StatusUnsupportedMediaType }

func (e UnsupportedMediaTypeError) ErrorCode() string { return "unsupported_media_type" }

func (e UnsupportedMediaTypeError) PublicMessage() string { return "unsupported media type" }

// JSONDecodeError marks request body decode failures as client errors while
// preserving the underlying error for errors.Is/errors.As.
//
// Stable: http.json-decode-400 — a malformed or unknown-field body is 400 bad_request.invalid_json, an empty one bad_request.empty_body.
// Covered by: TestMalformedJSONStays400, TestNewJSONServer_RejectsUnknownFieldsByDefault, TestEmptyBodyReportsItself
//
// Stable: http.json-body-limit-413 — a body over the configured limit is 413 request_too_large.
// Covered by: TestOverLimitBodyIs413, TestRawOverLimitBodyMessageIsNotJSONSpecific
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
//
// Stable: http.json-media-types — application/json, any +json suffix, and a request with no Content-Type are accepted.
// Covered by: TestJSONMediaTypesAreAccepted
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
