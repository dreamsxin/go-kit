package grpc

import (
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// StatusError adds the framework's transport-neutral classification to a gRPC
// status error, so a gRPC client endpoint composes with the same middleware as
// a server endpoint: retry, circuit breaking, logging, and the HTTP error
// encoders all read the same contracts.
//
// It stays a gRPC error: status.FromError, status.Code, and errors.As on the
// underlying error keep working.
//
// Beware of relaying an upstream classification to your own callers unchanged —
// a dependency's NotFound would make your handler answer 404. Translate when
// the upstream failure means something else to your clients:
//
//	if _, err := call(ctx, req); err != nil {
//	    return apperror.WrapCause(apperror.KindUnavailable, "upstream.users", err)
//	}
type StatusError struct {
	err    error
	st     *status.Status
	code   string
	domain string
	after  time.Duration
}

// ClassifyError wraps a gRPC status error in a StatusError. Errors that carry
// no gRPC status, including nil, are returned unchanged.
func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	code, domain, after := detailsFromStatus(st)
	return &StatusError{err: err, st: st, code: code, domain: domain, after: after}
}

func (e *StatusError) Error() string { return e.err.Error() }

// Unwrap returns the original gRPC error.
func (e *StatusError) Unwrap() error { return e.err }

// GRPCStatus keeps status.FromError and status.Code working on the wrapper.
func (e *StatusError) GRPCStatus() *status.Status { return e.st }

// Code reports the gRPC code of the underlying status.
func (e *StatusError) Code() codes.Code { return e.st.Code() }

// ErrorKindName implements the minimal apperror.KindNamer contract, mapping the
// gRPC code to the transport-neutral kind name.
func (e *StatusError) ErrorKindName() string { return KindNameForCode(e.st.Code()) }

// PublicMessage reports the message a server may forward to its own clients:
// the upstream code, never the upstream description.
//
// Error() keeps the description for logs, but a server that returns this error
// from an endpoint would otherwise leak it: the built-in HTTP error encoders
// fall back to err.Error() below 500, so an upstream NotFound's message would
// land verbatim in the downstream response body. This mirrors
// client.HTTPStatusError, so a gRPC hop and an HTTP hop redact the same way.
func (e *StatusError) PublicMessage() string {
	if e == nil {
		return ""
	}
	return "upstream request failed with code " + e.st.Code().String()
}

// Retryable reports whether the code marks a transient failure.
func (e *StatusError) Retryable() bool { return Retryable(e.err) }

// ErrorCode implements the stable machine-readable code contract by reading the
// google.rpc.ErrorInfo reason the server-side encoder attaches. It is empty when
// the server sent none. Relaying it keeps the application code intact across a
// gRPC hop and across a gRPC-to-HTTP hop, where it becomes the body's "code".
func (e *StatusError) ErrorCode() string { return e.code }

// ErrorDomain reports the google.rpc.ErrorInfo domain, the grouping that owns
// the code. It is empty when the server sent none.
func (e *StatusError) ErrorDomain() string { return e.domain }

// RetryAfter reports the delay from the google.rpc.RetryInfo status detail, the
// canonical gRPC retry hint. It is 0 when the server sent none.
func (e *StatusError) RetryAfter() time.Duration { return e.after }

// detailsFromStatus reads the canonical error details the framework emits:
// ErrorInfo carries the stable application code and its domain, RetryInfo the
// retry hint.
func detailsFromStatus(st *status.Status) (code, domain string, after time.Duration) {
	for _, detail := range st.Details() {
		switch info := detail.(type) {
		case *errdetails.ErrorInfo:
			if code == "" {
				code, domain = info.GetReason(), info.GetDomain()
			}
		case *errdetails.RetryInfo:
			if after > 0 {
				continue
			}
			if delay := info.GetRetryDelay(); delay != nil {
				if delayed := delay.AsDuration(); delayed > 0 {
					after = delayed
				}
			}
		}
	}
	return code, domain, after
}

// KindNameForCode maps a gRPC code to the transport-neutral kind name the
// framework uses. It is the inverse of server.CodeForErrorKind, so a code
// received by a client and a kind emitted by a server agree.
func KindNameForCode(code codes.Code) string {
	switch code {
	case codes.InvalidArgument, codes.OutOfRange:
		return "invalid_argument"
	case codes.Unauthenticated:
		return "unauthenticated"
	case codes.PermissionDenied:
		return "permission_denied"
	case codes.NotFound:
		return "not_found"
	case codes.AlreadyExists:
		return "already_exists"
	case codes.Aborted:
		return "conflict"
	case codes.FailedPrecondition:
		return "failed_precondition"
	case codes.ResourceExhausted:
		return "resource_exhausted"
	case codes.Unavailable:
		return "unavailable"
	case codes.DeadlineExceeded:
		return "deadline_exceeded"
	case codes.Canceled:
		return "canceled"
	case codes.Unimplemented:
		return "unimplemented"
	default:
		return "internal"
	}
}

var _ error = (*StatusError)(nil)
