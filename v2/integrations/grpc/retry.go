package grpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Retryable classifies transient gRPC status errors for sd/retry.
func Retryable(err error) bool {
	statusError, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch statusError.Code() {
	case codes.Unavailable, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}
