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
// mapping. The mapper receives the kind name (from ErrorKindName); return an
// invalid codes.Code value to fall back to the built-in rules.
//
// Use it when the application defines its own error kinds:
//
//	server.ServerErrorEncoder(server.NewErrorEncoder(
//	    server.WithKindMapper(func(k string) codes.Code {
//	        if k == "payment_failed" { return codes.FailedPrecondition }
//	        return codes.Code(99) // invalid: fall back
//	    }),
//	))
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
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}

	code, message := c.classify(err)

	select {
	case <-ctx.Done():
		return status.FromContextError(ctx.Err()).Err()
	default:
		return c.statusError(code, message, err)
	}
}

// classify resolves the gRPC code and the message safe to expose. Unclassified
// errors stay Internal with a redacted message.
func (c errorEncoderConfig) classify(err error) (codes.Code, string) {
	var kinder interface{ ErrorKindName() string }
	if !errors.As(err, &kinder) {
		return codes.Internal, "internal error"
	}
	kind := kinder.ErrorKindName()

	if c.kindMapper != nil {
		if code := c.kindMapper(kind); code >= codes.OK && code <= codes.DataLoss {
			if message := publicMessage(err); message != "" {
				return code, message
			}
			return code, code.String()
		}
	}

	code := codeForErrorKind(kind)
	if message := publicMessage(err); message != "" {
		return code, message
	}
	if code != codes.Internal {
		return code, code.String()
	}
	return code, "internal error"
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
