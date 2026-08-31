package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/dreamsxin/go-kit/v2/apperror"
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

// DefaultErrorEncoder writes a plain-text response. Internal errors are always
// redacted. Errors may implement json.Marshaler, transporthttp.StatusCoder, or
// transporthttp.Headerer to customize non-5xx responses.
func DefaultErrorEncoder(_ context.Context, err error, w http.ResponseWriter) {
	status := httpStatus(err)
	contentType := "text/plain; charset=utf-8"
	body := []byte(http.StatusText(status))
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

func statusWithMapper(err error, mapper func(apperror.Kind) int) int {
	var kinder apperror.Kinder
	if !errors.As(err, &kinder) {
		return 0
	}
	status := mapper(kinder.ErrorKind())
	if status < 100 || status > 999 {
		return 0
	}
	return status
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

	message := http.StatusText(status)
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
	var verr *endpoint.ValidationError
	if errors.As(err, &verr) {
		errorCode = "bad_request.validation"
	}
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

// HTTPStatusForError returns the HTTP status the built-in error encoders use
// for err, honoring StatusCoder, the rejection errors
// (ErrBackpressure, ErrBulkheadFull, ErrCircuitOpen, ErrRateLimited),
// apperror kinds (endpoint.ValidationError classifies itself as
// invalid_argument through apperror.Kinder), and unclassified
// context.DeadlineExceeded (504). Custom error encoders should reuse it
// instead of duplicating the mapping.
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

	if errors.Is(err, endpoint.ErrBackpressure) || errors.Is(err, endpoint.ErrBulkheadFull) || errors.Is(err, endpoint.ErrCircuitOpen) || errors.Is(err, endpoint.ErrRateLimited) {
		return http.StatusTooManyRequests
	}

	var kinder apperror.Kinder
	if errors.As(err, &kinder) {
		return statusForErrorKind(kinder.ErrorKind())
	}

	// An unclassified context deadline maps like apperror.KindDeadlineExceeded
	// so TimeoutMiddleware and an explicitly classified timeout agree on 504,
	// matching the canonical DEADLINE_EXCEEDED -> 504 mapping the gRPC adapter
	// already uses. Explicit classification still wins over this fallback.
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
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
	default:
		return http.StatusInternalServerError
	}
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
