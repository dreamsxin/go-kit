package endpoint

import (
	"context"
	"fmt"
)

// ErrorWrapper wraps an endpoint error with the name of the operation that
// produced it, enabling callers to distinguish errors from different endpoints
// using errors.As.
type ErrorWrapper struct {
	Operation string
	Err       error
}

func (e *ErrorWrapper) Error() string {
	return fmt.Sprintf("%s: %v", e.Operation, e.Err)
}

func (e *ErrorWrapper) Unwrap() error {
	return e.Err
}

// ErrorHandlingMiddleware returns a Middleware that wraps any error returned
// by the next Endpoint in an ErrorWrapper tagged with operation.
// Use errors.As to unwrap it downstream:
//
//	var ew *endpoint.ErrorWrapper
//	if errors.As(err, &ew) { ... }
func ErrorHandlingMiddleware(operation string) Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			response, err := next(ctx, request)
			if err != nil {
				return nil, &ErrorWrapper{
					Operation: operation,
					Err:       err,
				}
			}
			return response, nil
		}
	}
}
