package server

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

// StatusClientClosedRequest is the non-standard 499 status used when the caller
// disconnects or cancels before the response is written. It follows the nginx
// and gRPC-gateway convention and keeps client disconnects out of the 5xx rate.
const StatusClientClosedRequest = 499

// ErrorEncoder writes an endpoint or transport error to an HTTP response.
type ErrorEncoder func(ctx context.Context, err error, w http.ResponseWriter)

// ErrorResponse is the default JSON shape emitted by JSONErrorEncoder.
type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// DefaultErrorEncoder writes a plain-text response. Internal errors are always
// redacted. Errors may implement json.Marshaler, transporthttp.StatusCoder, or
// transporthttp.Headerer to customize non-5xx responses.
func DefaultErrorEncoder(_ context.Context, err error, w http.ResponseWriter) {
	status := httpStatus(err)
	contentType := "text/plain; charset=utf-8"
	body := []byte(statusText(status))
	if status < http.StatusInternalServerError && err != nil {
		body = []byte(err.Error())
		var marshaler json.Marshaler
		if errors.As(err, &marshaler) {
			if jsonBody, marshalErr := marshaler.MarshalJSON(); marshalErr == nil {
				contentType, body = "application/json; charset=utf-8", jsonBody
			}
		}
	}
	w.Header().Set("Content-Type", contentType)
	var headerer transporthttp.Headerer
	if errors.As(err, &headerer) {
		for key, values := range headerer.Headers() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}
	applyRetryAfter(w, err)
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
//
// JSONErrorEncoderWithKindMapper returns an ErrorEncoder like JSONErrorEncoder
// but resolves the HTTP status through the given kind mapper first. The mapper
// receives the classified apperror kind; return a non-positive status to fall
// back to the built-in mapping. Unclassified and non-apperror errors still use
// the built-in rules (StatusCoder, ValidationError, rejection errors).
//
// Use it when the application defines its own error kinds with custom
// statuses:
//
//	server.ServerErrorEncoder(server.JSONErrorEncoderWithKindMapper(func(k apperror.Kind) int {
//	    if k == "payment_failed" { return http.StatusPaymentRequired }
//	    return 0 // fall back to the built-in mapping
//	}))
func JSONErrorEncoderWithKindMapper(mapper func(apperror.Kind) int) ErrorEncoder {
	if mapper == nil {
		return JSONErrorEncoder
	}
	return func(ctx context.Context, err error, w http.ResponseWriter) {
		if status := statusWithMapper(err, mapper); status > 0 {
			encodeJSONErrorWithStatus(ctx, err, w, status)
			return
		}
		encodeJSONError(ctx, err, w)
	}
}

// statusWithMapper resolves the status through the application's kind mapper.
// An explicit StatusCoder still wins, exactly as in httpStatus: an error that
// states its own status — a relayed client.HTTPStatusError, a validation error —
// means it, and the mapper exists for errors that carry only a kind.
func statusWithMapper(err error, mapper func(apperror.Kind) int) int {
	var sc transporthttp.StatusCoder
	if errors.As(err, &sc) {
		if status := sc.StatusCode(); status >= 100 && status <= 999 {
			return status
		}
	}
	kind, ok := errorKind(err)
	if !ok {
		return 0
	}
	status := mapper(kind)
	if status < 100 || status > 999 {
		return 0
	}
	return status
}

// errorKind reads the classification from either the typed apperror.Kinder
// contract or the minimal apperror.KindNamer contract.
func errorKind(err error) (apperror.Kind, bool) {
	var kinder apperror.Kinder
	if errors.As(err, &kinder) {
		return kinder.ErrorKind(), true
	}
	var namer apperror.KindNamer
	if errors.As(err, &namer) {
		return apperror.Kind(namer.ErrorKindName()), true
	}
	return "", false
}

func encodeJSONErrorWithStatus(ctx context.Context, err error, w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	var h transporthttp.Headerer
	if errors.As(err, &h) {
		for k, vals := range h.Headers() {
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
	}

	message := statusText(status)
	if message == "" {
		message = "HTTP error"
	}
	var pm transporthttp.PublicMessager
	if errors.As(err, &pm) && pm.PublicMessage() != "" {
		message = pm.PublicMessage()
	} else if status < http.StatusInternalServerError && err != nil {
		message = err.Error()
	}

	errorCode := defaultErrorCode(status)
	var verr *endpoint.ValidationError
	if errors.As(err, &verr) {
		errorCode = "bad_request.validation"
	}
	var ec transporthttp.ErrorCoder
	if errors.As(err, &ec) && ec.ErrorCode() != "" {
		errorCode = ec.ErrorCode()
	}

	applyRetryAfter(w, err)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Code:      errorCode,
		Message:   message,
		RequestID: endpoint.RequestIDFromContext(ctx),
	})
}

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

	code := httpStatus(err)

	message := statusText(code)
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
	var verr *endpoint.ValidationError
	if errors.As(err, &verr) {
		errorCode = "bad_request.validation"
	}
	var ec transporthttp.ErrorCoder
	if errors.As(err, &ec) && ec.ErrorCode() != "" {
		errorCode = ec.ErrorCode()
	}

	applyRetryAfter(w, err)
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Code:      errorCode,
		Message:   message,
		RequestID: endpoint.RequestIDFromContext(ctx),
	})
}

// HTTPStatusForError returns the HTTP status the built-in error encoders use
// for err, honoring StatusCoder, apperror kinds (through either apperror.Kinder
// or the minimal apperror.KindNamer contract; the endpoint rejection errors and
// endpoint.ValidationError classify themselves), and unclassified context
// errors (context.DeadlineExceeded is 504, context.Canceled is 499). Custom
// error encoders should reuse it instead of duplicating the mapping.
func HTTPStatusForError(err error) int {
	return httpStatus(err)
}

func httpStatus(err error) int {
	var sc transporthttp.StatusCoder
	if errors.As(err, &sc) {
		if status := sc.StatusCode(); status >= 100 && status <= 999 {
			return status
		}
	}

	if kind, ok := errorKind(err); ok {
		return statusForErrorKind(kind)
	}

	// Unclassified context errors map like their apperror kinds so a timeout
	// or a client disconnect reads the same whether the endpoint classified it
	// or not. Explicit classification still wins over these fallbacks.
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, context.Canceled):
		return StatusClientClosedRequest
	}
	return http.StatusInternalServerError
}

// HTTPStatusForErrorKind returns the HTTP status the built-in encoders use for
// an apperror kind. Custom kind mappers fall back to it for unknown kinds.
func HTTPStatusForErrorKind(kind apperror.Kind) int {
	return statusForErrorKind(kind)
}

func statusForErrorKind(kind apperror.Kind) int {
	switch kind {
	case apperror.KindInvalidArgument:
		return http.StatusBadRequest
	case apperror.KindUnauthenticated:
		return http.StatusUnauthorized
	case apperror.KindPermissionDenied:
		return http.StatusForbidden
	case apperror.KindNotFound:
		return http.StatusNotFound
	case apperror.KindAlreadyExists, apperror.KindConflict:
		return http.StatusConflict
	case apperror.KindFailedPrecondition:
		return http.StatusPreconditionFailed
	case apperror.KindResourceExhausted:
		return http.StatusTooManyRequests
	case apperror.KindUnavailable:
		return http.StatusServiceUnavailable
	case apperror.KindDeadlineExceeded:
		return http.StatusGatewayTimeout
	case apperror.KindCanceled:
		return StatusClientClosedRequest
	case apperror.KindUnimplemented:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

// statusText is http.StatusText extended with the non-standard statuses this
// package emits, so error bodies never fall back to a generic message.
func statusText(status int) string {
	if status == StatusClientClosedRequest {
		return "Client Closed Request"
	}
	return http.StatusText(status)
}

// applyRetryAfter emits a Retry-After header when the error knows how long the
// client should wait. An explicit Headerer value already on the response wins.
func applyRetryAfter(w http.ResponseWriter, err error) {
	if w.Header().Get("Retry-After") != "" {
		return
	}
	var reporter endpoint.RetryAfterReporter
	if !errors.As(err, &reporter) {
		return
	}
	after := reporter.RetryAfter()
	if after <= 0 {
		return
	}
	seconds := int(math.Ceil(after.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
}

func defaultErrorCode(status int) string {
	text := statusText(status)
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
