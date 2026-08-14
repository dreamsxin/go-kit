package server

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// ErrorEncoder maps application errors to errors safe for gRPC clients.
type ErrorEncoder func(ctx context.Context, err error) error

// DefaultErrorEncoder preserves existing gRPC statuses, maps apperror kinds to
// gRPC codes, and redacts all unclassified errors as Internal.
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
	var kinder apperror.Kinder
	if errors.As(err, &kinder) {
		code = codeForErrorKind(kinder.ErrorKind())
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

func codeForErrorKind(kind apperror.Kind) codes.Code {
	switch kind {
	case apperror.KindInvalidArgument:
		return codes.InvalidArgument
	case apperror.KindUnauthenticated:
		return codes.Unauthenticated
	case apperror.KindPermissionDenied:
		return codes.PermissionDenied
	case apperror.KindNotFound:
		return codes.NotFound
	case apperror.KindAlreadyExists:
		return codes.AlreadyExists
	case apperror.KindConflict:
		return codes.Aborted
	case apperror.KindFailedPrecondition:
		return codes.FailedPrecondition
	case apperror.KindResourceExhausted:
		return codes.ResourceExhausted
	case apperror.KindUnavailable:
		return codes.Unavailable
	case apperror.KindDeadlineExceeded:
		return codes.DeadlineExceeded
	default:
		return codes.Internal
	}
}
