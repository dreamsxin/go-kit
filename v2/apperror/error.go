// Package apperror defines transport-neutral application errors.
//
// Application code classifies failures here; transports map those classes to
// protocol-specific statuses such as HTTP status codes or gRPC codes.
package apperror

// Kind classifies an application failure independently of any transport.
type Kind string

const (
	KindInternal           Kind = "internal"
	KindInvalidArgument    Kind = "invalid_argument"
	KindUnauthenticated    Kind = "unauthenticated"
	KindPermissionDenied   Kind = "permission_denied"
	KindNotFound           Kind = "not_found"
	KindAlreadyExists      Kind = "already_exists"
	KindConflict           Kind = "conflict"
	KindFailedPrecondition Kind = "failed_precondition"
	KindResourceExhausted  Kind = "resource_exhausted"
	KindUnavailable        Kind = "unavailable"
	KindDeadlineExceeded   Kind = "deadline_exceeded"
)

// Kinder is implemented by errors that expose a transport-neutral Kind.
type Kinder interface {
	ErrorKind() Kind
}

// KindNamer is the minimal structural contract used by optional transports
// that must not depend on this package directly.
type KindNamer interface {
	ErrorKindName() string
}

// Error is a classified application error with a stable machine-readable code
// and an optional message that is safe to expose to clients.
type Error struct {
	kind    Kind
	code    string
	message string
	cause   error
}

// New creates a classified application error.
func New(kind Kind, code, message string) *Error {
	return &Error{kind: normalizeKind(kind), code: code, message: message}
}

// Wrap creates a classified application error that preserves cause.
func Wrap(kind Kind, code, message string, cause error) *Error {
	return &Error{kind: normalizeKind(kind), code: code, message: message, cause: cause}
}

// WrapCause creates a classified application error with an empty public
// message that preserves cause. Use it when the cause must stay internal
// and only the kind and code should drive transport mapping.
func WrapCause(kind Kind, code string, cause error) *Error {
	return &Error{kind: normalizeKind(kind), code: code, cause: cause}
}

// Convenience constructors, one per kind. They keep call sites short without
// hiding the transport-neutral classification.

// Internal creates a KindInternal error (HTTP 500 / gRPC Internal). The
// message stays opaque to clients because the transports redact 5xx bodies.
func Internal(code, message string) *Error {
	return New(KindInternal, code, message)
}

// InvalidArgument creates a KindInvalidArgument error (HTTP 400 / gRPC InvalidArgument).
func InvalidArgument(code, message string) *Error {
	return New(KindInvalidArgument, code, message)
}

// Unauthenticated creates a KindUnauthenticated error (HTTP 401 / gRPC Unauthenticated).
func Unauthenticated(code, message string) *Error {
	return New(KindUnauthenticated, code, message)
}

// PermissionDenied creates a KindPermissionDenied error (HTTP 403 / gRPC PermissionDenied).
func PermissionDenied(code, message string) *Error {
	return New(KindPermissionDenied, code, message)
}

// NotFound creates a KindNotFound error (HTTP 404 / gRPC NotFound).
func NotFound(code, message string) *Error {
	return New(KindNotFound, code, message)
}

// AlreadyExists creates a KindAlreadyExists error (HTTP 409 / gRPC AlreadyExists).
func AlreadyExists(code, message string) *Error {
	return New(KindAlreadyExists, code, message)
}

// Conflict creates a KindConflict error (HTTP 409 / gRPC Aborted). Use it for
// concurrent-update conflicts; use AlreadyExists when a unique resource is
// being created twice.
func Conflict(code, message string) *Error {
	return New(KindConflict, code, message)
}

// FailedPrecondition creates a KindFailedPrecondition error
// (HTTP 412 / gRPC FailedPrecondition).
func FailedPrecondition(code, message string) *Error {
	return New(KindFailedPrecondition, code, message)
}

// ResourceExhausted creates a KindResourceExhausted error
// (HTTP 429 / gRPC ResourceExhausted).
func ResourceExhausted(code, message string) *Error {
	return New(KindResourceExhausted, code, message)
}

// Unavailable creates a KindUnavailable error (HTTP 503 / gRPC Unavailable).
func Unavailable(code, message string) *Error {
	return New(KindUnavailable, code, message)
}

// DeadlineExceeded creates a KindDeadlineExceeded error
// (HTTP 504 / gRPC DeadlineExceeded).
func DeadlineExceeded(code, message string) *Error {
	return New(KindDeadlineExceeded, code, message)
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.cause != nil {
		if e.message != "" {
			return e.message + ": " + e.cause.Error()
		}
		if e.code != "" {
			return e.code + ": " + e.cause.Error()
		}
		return e.cause.Error()
	}
	if e.message != "" {
		return e.message
	}
	if e.code != "" {
		return e.code
	}
	return string(e.kind)
}

// Unwrap returns the underlying cause, if any.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// ErrorKind returns the transport-neutral failure class.
func (e *Error) ErrorKind() Kind {
	if e == nil {
		return KindInternal
	}
	return normalizeKind(e.kind)
}

// ErrorKindName returns the transport-neutral failure class as a string.
func (e *Error) ErrorKindName() string {
	return string(e.ErrorKind())
}

// ErrorCode returns the stable machine-readable application code.
func (e *Error) ErrorCode() string {
	if e == nil {
		return ""
	}
	return e.code
}

// PublicMessage returns the message that may be exposed to clients.
func (e *Error) PublicMessage() string {
	if e == nil {
		return ""
	}
	return e.message
}

func normalizeKind(kind Kind) Kind {
	if kind == "" {
		return KindInternal
	}
	return kind
}
