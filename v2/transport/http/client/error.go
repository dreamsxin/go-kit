package client

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// statusClientClosedRequest mirrors server.StatusClientClosedRequest, the
// non-standard 499 nginx and gRPC-gateway emit for a caller that went away. It
// is duplicated instead of imported so the client package stays independent of
// the server package.
const statusClientClosedRequest = 499

// ErrorKind classifies the response status so that a client call composes with
// the same middleware as a server endpoint: endpoint.DefaultRetryable,
// structured logging, and the server-side error encoders all read the
// classification through apperror.Kinder.
//
// Beware of propagating an upstream classification to your own callers
// unchanged: returning a dependency's KindNotFound makes your own handler
// answer 404. Translate deliberately when the upstream failure means something
// else to your clients:
//
//	if _, err := call(ctx, req); err != nil {
//	    return apperror.WrapCause(apperror.KindUnavailable, "upstream.users", err)
//	}
func (e *HTTPStatusError) ErrorKind() apperror.Kind {
	if e == nil {
		return apperror.KindInternal
	}
	return KindForStatus(e.StatusCode)
}

// ErrorKindName implements apperror.KindNamer for transports that must not
// depend on apperror directly.
func (e *HTTPStatusError) ErrorKindName() string {
	return string(e.ErrorKind())
}

// ErrorCode implements transport/http.ErrorCoder by reading the stable
// application code from the JSON error body the framework's HTTP encoders emit
// ({"code": ..., "message": ...}). It is empty when the body is not JSON,
// carries no code, or was truncated (HTTPStatusError.Truncated) — a cut-off
// document cannot be parsed.
//
// The status code is a coarse channel, so the stable code is what identifies the
// failure. Relaying it keeps the upstream code intact instead of degrading to a
// status-derived name such as "not_found".
//
// The upstream message is deliberately not exposed as a public message: it
// belongs to another service's contract and must not leak into your own
// responses unreviewed.
func (e *HTTPStatusError) ErrorCode() string {
	if e == nil || len(e.Body) == 0 {
		return ""
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(e.Body, &body); err != nil {
		return ""
	}
	return body.Code
}

// RetryAfter implements endpoint.RetryAfterReporter by reading the Retry-After
// response header, so a retrying client waits as long as the server asked.
// Both forms of the header are understood: delay-seconds and HTTP-date
// (RFC 9110). It reports 0 when the header is absent, malformed, or already in
// the past.
func (e *HTTPStatusError) RetryAfter() time.Duration {
	if e == nil {
		return 0
	}
	return parseRetryAfter(e.Header.Get("Retry-After"), time.Now())
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	if after := when.Sub(now); after > 0 {
		return after
	}
	return 0
}

// KindForStatus maps an HTTP status code to the transport-neutral apperror kind
// the server-side encoders would have produced for it. It is the inverse of
// server.HTTPStatusForErrorKind and is exported so custom response decoders can
// classify their own errors the same way.
//
// Unknown 4xx statuses become KindInvalidArgument and every other unknown
// status becomes KindInternal.
func KindForStatus(status int) apperror.Kind {
	switch status {
	case http.StatusBadRequest:
		return apperror.KindInvalidArgument
	case http.StatusUnauthorized:
		return apperror.KindUnauthenticated
	case http.StatusForbidden:
		return apperror.KindPermissionDenied
	case http.StatusNotFound:
		return apperror.KindNotFound
	case http.StatusConflict:
		return apperror.KindConflict
	case http.StatusPreconditionFailed:
		return apperror.KindFailedPrecondition
	case http.StatusTooManyRequests:
		return apperror.KindResourceExhausted
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return apperror.KindDeadlineExceeded
	case statusClientClosedRequest:
		return apperror.KindCanceled
	case http.StatusNotImplemented:
		return apperror.KindUnimplemented
	case http.StatusBadGateway, http.StatusServiceUnavailable:
		return apperror.KindUnavailable
	}
	if status >= 400 && status < 500 {
		return apperror.KindInvalidArgument
	}
	return apperror.KindInternal
}
