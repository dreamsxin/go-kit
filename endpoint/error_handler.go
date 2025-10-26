package endpoint

import (
	"context"
	"fmt"
)

// ErrorWrapper 包装端点错误
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

// ErrorHandlingMiddleware 错误处理中间件
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
