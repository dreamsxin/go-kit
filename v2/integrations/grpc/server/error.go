package server

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorEncoder maps application errors to errors safe for gRPC clients.
type ErrorEncoder func(ctx context.Context, err error) error

// DefaultErrorEncoder preserves existing gRPC statuses, maps transport-neutral
// application error kinds to gRPC codes, and redacts unclassified errors.
func DefaultErrorEncoder(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}

	code := codes.Internal
	message := "internal error"
	var kinder interface{ ErrorKindName() string }
	if errors.As(err, &kinder) {
		code = codeForErrorKind(kinder.ErrorKindName())
		var publicMessager interface{ PublicMessage() string }
		if errors.As(err, &publicMessager) && publicMessager.PublicMessage() != "" {
			message = publicMessager.PublicMessage()
		} else if code != codes.Internal {
			message = code.String()
		}
	}

	select {
	case <-ctx.Done():
		return status.FromContextError(ctx.Err()).Err()
	default:
		return status.Error(code, message)
	}
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
	default:
		return codes.Internal
	}
}
