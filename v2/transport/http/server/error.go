package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

// ErrorEncoder writes an endpoint or transport error to an HTTP response.
type ErrorEncoder func(ctx context.Context, err error, w http.ResponseWriter)

// ErrorResponse is the default JSON shape emitted by JSONErrorEncoder.
type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// HTTPError is a small helper for returning transport-aware errors from
// endpoints without adopting a larger application error framework.
type HTTPError struct {
	Status  int
	Code    string
	Message string
	Err     error
	Header  http.Header
}

// NewHTTPError creates an HTTPError with a public message.
func NewHTTPError(status int, code, message string) *HTTPError {
	return &HTTPError{Status: status, Code: code, Message: message}
}

// WrapHTTPError creates an HTTPError that preserves an underlying cause.
func WrapHTTPError(status int, code, message string, err error) *HTTPError {
	return &HTTPError{Status: status, Code: code, Message: message, Err: err}
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return http.StatusText(e.StatusCode())
}

func (e *HTTPError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *HTTPError) StatusCode() int {
	if e == nil || e.Status <= 0 {
		return http.StatusInternalServerError
	}
	return e.Status
}

func (e *HTTPError) ErrorCode() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func (e *HTTPError) PublicMessage() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *HTTPError) Headers() http.Header {
	if e == nil {
		return nil
	}
	return e.Header
}

// DefaultErrorEncoder writes a plain-text 500 response by default. Errors may
// implement json.Marshaler, transporthttp.StatusCoder, or
// transporthttp.Headerer to customize the response.
func DefaultErrorEncoder(_ context.Context, err error, w http.ResponseWriter) {
	contentType, body := "text/plain; charset=utf-8", []byte(err.Error())
	if marshaler, ok := err.(json.Marshaler); ok {
		if jsonBody, marshalErr := marshaler.MarshalJSON(); marshalErr == nil {
			contentType, body = "application/json; charset=utf-8", jsonBody
		}
	}
	w.Header().Set("Content-Type", contentType)
	if headerer, ok := err.(transporthttp.Headerer); ok {
		for key, values := range headerer.Headers() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}
	status := http.StatusInternalServerError
	if statusCoder, ok := err.(transporthttp.StatusCoder); ok {
		status = statusCoder.StatusCode()
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// JSONErrorEncoder is an ErrorEncoder that always writes a JSON
// error body.  It inspects the error for optional interfaces:
//
//   - transporthttp.StatusCoder: uses that HTTP status code (default 500)
//   - transporthttp.Headerer: merges those headers into the response
//   - transporthttp.ErrorCoder: sets a stable machine-readable code
//   - transporthttp.PublicMessager: overrides the public message
//
// The response body is:
// {"code": "<code>", "message": "<message>"}
//
// Use it with ServerErrorEncoder:
//
//	server.NewServer(ep, dec, enc,
//	    server.ServerErrorEncoder(server.JSONErrorEncoder),
//	)
//
// Or with NewJSONServer:
//
//	server.NewJSONServer[Req](handler,
//	    server.ServerErrorEncoder(server.JSONErrorEncoder),
//	)
var JSONErrorEncoder ErrorEncoder = func(ctx context.Context, err error, w http.ResponseWriter) {
	encodeJSONError(ctx, err, w)
}

func encodeJSONError(ctx context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	var h transporthttp.Headerer
	if errors.As(err, &h) {
		for k, vals := range h.Headers() {
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
	}

	code := http.StatusInternalServerError
	var sc transporthttp.StatusCoder
	if errors.As(err, &sc) {
		code = sc.StatusCode()
	}
	if code < 100 || code > 999 {
		code = http.StatusInternalServerError
	}

	message := http.StatusText(code)
	if message == "" {
		message = "HTTP error"
	}
	var pm transporthttp.PublicMessager
	if errors.As(err, &pm) && pm.PublicMessage() != "" {
		message = pm.PublicMessage()
	} else if code < http.StatusInternalServerError && err != nil {
		message = err.Error()
	}

	errorCode := defaultErrorCode(code)
	var ec transporthttp.ErrorCoder
	if errors.As(err, &ec) && ec.ErrorCode() != "" {
		errorCode = ec.ErrorCode()
	}

	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Code:      errorCode,
		Message:   message,
		RequestID: endpoint.RequestIDFromContext(ctx),
	})
}

func defaultErrorCode(status int) string {
	text := http.StatusText(status)
	if text == "" {
		return "http_error"
	}
	text = strings.ToLower(text)
	var b strings.Builder
	underscore := false
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			underscore = false
			continue
		}
		if !underscore && b.Len() > 0 {
			b.WriteByte('_')
			underscore = true
		}
	}
	return strings.TrimSuffix(b.String(), "_")
}
