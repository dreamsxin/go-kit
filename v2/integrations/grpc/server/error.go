package server

import (
	"context"
	"errors"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/protoadapt"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// ErrorEncoder maps application errors to errors safe for gRPC clients.
type ErrorEncoder func(ctx context.Context, err error) error

// ErrorEncoderOption configures NewErrorEncoder.
type ErrorEncoderOption func(*errorEncoderConfig)

type errorEncoderConfig struct {
	kindMapper func(string) codes.Code
	domain     string
}

// WithKindMapper resolves the gRPC code through mapper before the built-in
// mapping. The mapper receives the kind name (from ErrorKindName); return
// codes.OK, or any code above codes.Unauthenticated, to fall back to the
// built-in rules.
//
// Use it when the application defines its own error kinds:
//
//	server.ServerErrorEncoder(server.NewErrorEncoder(
//	    server.WithKindMapper(func(k string) codes.Code {
//	        if k == "payment_failed" { return codes.FailedPrecondition }
//	        return codes.OK // no opinion: fall back
//	    }),
//	))
//
// codes.OK is the fallback sentinel on purpose. It is the zero value, so a
// mapper whose switch forgets a kind falls back instead of reporting success:
// status.New(codes.OK, ...).Err() is nil, and returning that would turn a
// failed call into a successful one.
func WithKindMapper(mapper func(string) codes.Code) ErrorEncoderOption {
	return func(c *errorEncoderConfig) {
		if mapper != nil {
			c.kindMapper = mapper
		}
	}
}

// WithErrorDomain sets the google.rpc.ErrorInfo domain, the logical grouping
// that owns the reported error codes. Use the service's registered name, as
// AIP-193 recommends: "users.example.com".
func WithErrorDomain(domain string) ErrorEncoderOption {
	return func(c *errorEncoderConfig) { c.domain = domain }
}

// NewErrorEncoder builds an ErrorEncoder. With no options it behaves like
// DefaultErrorEncoder.
func NewErrorEncoder(options ...ErrorEncoderOption) ErrorEncoder {
	config := errorEncoderConfig{}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return config.encode
}

// DefaultErrorEncoder preserves existing gRPC statuses, maps transport-neutral
// application error kinds to gRPC codes, and redacts unclassified errors. The
// stable application code travels in a google.rpc.ErrorInfo detail and a retry
// hint in google.rpc.RetryInfo, so the classification, the code, and the retry
// advice survive the hop.
var DefaultErrorEncoder ErrorEncoder = errorEncoderConfig{}.encode

// ErrorEncoderWithKindMapper returns an ErrorEncoder like DefaultErrorEncoder
// but resolves the gRPC code through the given mapper first. It is shorthand
// for NewErrorEncoder(WithKindMapper(mapper)).
func ErrorEncoderWithKindMapper(mapper func(string) codes.Code) ErrorEncoder {
	return NewErrorEncoder(WithKindMapper(mapper))
}

func (c errorEncoderConfig) encode(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}

	// Explicit classification wins over any context error, exactly as in
	// transport/http/server.httpStatus: an error that classifies itself as
	// not_found means it, whether or not a cancelled context sits in its chain
	// and whether or not the request was cancelled while it was produced. That
	// is what makes one apperror map to the same failure class over HTTP and
	// gRPC; checking the context first made the two transports disagree.
	if kind, ok := errorKindName(err); ok {
		code, message := c.classifyKind(kind, err)
		return c.statusError(code, message, err)
	}

	// Only unclassified errors fall back to the context status.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return status.FromContextError(ctxErr).Err()
	}
	return c.statusError(codes.Internal, "internal error", err)
}

// classifyKind resolves the gRPC code and the message safe to expose for an
// error that classified itself.
func (c errorEncoderConfig) classifyKind(kind string, err error) (codes.Code, string) {
	if c.kindMapper != nil {
		if code := c.kindMapper(kind); isFailureCode(code) {
			return code, publicCodeMessage(code, err)
		}
	}
	code := codeForErrorKind(kind)
	return code, publicCodeMessage(code, err)
}

// publicCodeMessage resolves the message a client may see for code.
//
// codes.Internal always reads "internal error", mirroring the HTTP encoders'
// rule for 500: Internal is where every unclassified error and every
// default-kind apperror lands, so its message was never chosen for exposure —
// apperror.PublicMessage returns whatever was passed to apperror.New, secrets
// included. Deliberate codes still carry their message, because reaching them
// takes an explicit kind or an explicit mapper entry.
func publicCodeMessage(code codes.Code, err error) string {
	if code == codes.Internal {
		return "internal error"
	}
	if message := publicMessage(err); message != "" {
		return message
	}
	return code.String()
}

// isFailureCode reports whether a kind mapper returned a code that can carry a
// failure. codes.OK is rejected because status.New(codes.OK, ...).Err() is nil:
// accepting it would answer a failed call with no error at all, and codes.OK is
// precisely what a mapper returns when its switch has no arm for the kind. The
// upper bound is codes.Unauthenticated, the highest code gRPC defines — not
// codes.DataLoss, which would reject the very code the built-in mapping emits
// for an unauthenticated error.
func isFailureCode(code codes.Code) bool {
	return code > codes.OK && code <= codes.Unauthenticated
}

// errorKindName reads the classification from either the typed apperror.Kinder
// contract or the minimal apperror.KindNamer one, in that order. Both are read
// so an error classified with apperror.Kinder alone maps to the same code here
// as it does over HTTP, instead of collapsing to Internal.
func errorKindName(err error) (string, bool) {
	var kinder apperror.Kinder
	if errors.As(err, &kinder) {
		return string(kinder.ErrorKind()), true
	}
	var namer apperror.KindNamer
	if errors.As(err, &namer) {
		return namer.ErrorKindName(), true
	}
	return "", false
}

// statusError builds the gRPC status and attaches the canonical error details:
// google.rpc.ErrorInfo carries the stable application code so it is not lost the
// way an HTTP body would lose it, and google.rpc.RetryInfo carries the
// Retry-After equivalent.
func (c errorEncoderConfig) statusError(code codes.Code, message string, err error) error {
	st := status.New(code, message)

	details := make([]protoadapt.MessageV1, 0, 2)
	if reason := errorCode(err); reason != "" {
		details = append(details, &errdetails.ErrorInfo{Reason: reason, Domain: c.domain})
	}
	if after := retryAfter(err); after > 0 {
		details = append(details, &errdetails.RetryInfo{RetryDelay: durationpb.New(after)})
	}
	if len(details) == 0 {
		return st.Err()
	}
	if detailed, detailErr := st.WithDetails(details...); detailErr == nil {
		st = detailed
	}
	return st.Err()
}

func publicMessage(err error) string {
	var messager interface{ PublicMessage() string }
	if errors.As(err, &messager) {
		return messager.PublicMessage()
	}
	return ""
}

func errorCode(err error) string {
	var coder interface{ ErrorCode() string }
	if errors.As(err, &coder) {
		return coder.ErrorCode()
	}
	return ""
}

func retryAfter(err error) time.Duration {
	var reporter interface{ RetryAfter() time.Duration }
	if errors.As(err, &reporter) {
		return reporter.RetryAfter()
	}
	return 0
}

// CodeForError returns the gRPC code the built-in encoder uses for err: an
// explicit apperror classification first (read through either apperror.Kinder or
// the minimal apperror.KindNamer contract), then unclassified context errors,
// then codes.Internal.
//
// It is the counterpart of transport/http/server.HTTPStatusForError. A custom
// error encoder should reuse it rather than re-deriving the order, which is how
// the two transports drifted into disagreeing about the same error.
func CodeForError(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	if kind, ok := errorKindName(err); ok {
		return codeForErrorKind(kind)
	}
	switch {
	case errors.Is(err, context.Canceled):
		return codes.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return codes.DeadlineExceeded
	}
	return codes.Internal
}

// CodeForErrorKind returns the gRPC code the built-in encoder uses for a
// transport-neutral error kind name. Custom kind mappers fall back to it for
// unknown kinds.
func CodeForErrorKind(kind string) codes.Code {
	return codeForErrorKind(kind)
}

func codeForErrorKind(kind string) codes.Code {
	switch kind {
	case "invalid_argument":
		return codes.InvalidArgument
	case "unauthenticated":
		return codes.Unauthenticated
	case "permission_denied":
		return codes.PermissionDenied
	case "not_found":
		return codes.NotFound
	case "already_exists":
		return codes.AlreadyExists
	case "conflict":
		return codes.Aborted
	case "failed_precondition":
		return codes.FailedPrecondition
	case "resource_exhausted":
		return codes.ResourceExhausted
	case "unavailable":
		return codes.Unavailable
	case "deadline_exceeded":
		return codes.DeadlineExceeded
	case "canceled":
		return codes.Canceled
	case "unimplemented":
		return codes.Unimplemented
	default:
		return codes.Internal
	}
}
