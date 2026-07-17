package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd/interfaces"
	"github.com/dreamsxin/go-kit/v2/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryError is returned when all retry attempts are exhausted.
// RawErrors contains every error from each attempt; Final is the last error
// (or a replacement set by RetryCallback).
type RetryError struct {
	RawErrors []error
	Final     error // nil when the last attempt succeeded
}

func (e RetryError) Error() string {
	var suffix string
	if len(e.RawErrors) > 1 {
		a := make([]string, len(e.RawErrors)-1)
		for i := 0; i < len(e.RawErrors)-1; i++ {
			a[i] = e.RawErrors[i].Error()
		}
		suffix = fmt.Sprintf(" (previously: %s)", strings.Join(a, "; "))
	}
	if e.Final == nil {
		return fmt.Sprintf("%v%s", e.RawErrors[len(e.RawErrors)-1], suffix)
	}
	return fmt.Sprintf("%v%s", e.Final, suffix)
}

// RetryCallback is called after each failed attempt.  It returns whether the
// executor should keep trying and an optional replacement error.  Returning
// keepTrying=false stops the retry loop immediately.
type RetryCallback func(n int, received error) (keepTrying bool, replacement error)

// RetryableFunc classifies whether an error is safe and useful to retry.
type RetryableFunc func(error) bool

// Retry returns an Endpoint that retries up to max times within timeout,
// selecting a new backend from b on each attempt.
func Retry(max int, timeout time.Duration, b interfaces.Balancer) endpoint.Endpoint {
	return RetryWithCallback(timeout, b, maxRetries(max))
}

// RetryAlways returns an Endpoint that retries indefinitely until timeout
// is reached or the call succeeds.
func RetryAlways(timeout time.Duration, b interfaces.Balancer) endpoint.Endpoint {
	return RetryWithCallback(timeout, b, alwaysRetry)
}

// 最大重试次数判断
func maxRetries(max int) RetryCallback {
	return func(n int, err error) (keepTrying bool, replacement error) {
		return n < max, nil
	}
}

func alwaysRetry(int, error) (keepTrying bool, replacement error) {
	return true, nil
}

func RetryWithCallback(timeout time.Duration, b interfaces.Balancer, cb RetryCallback) endpoint.Endpoint {
	return RetryWithRetryable(timeout, b, cb, DefaultRetryable)
}

func RetryWithRetryable(timeout time.Duration, b interfaces.Balancer, cb RetryCallback, retryable RetryableFunc) endpoint.Endpoint {
	if cb == nil {
		cb = alwaysRetry
	}
	if retryable == nil {
		retryable = DefaultRetryable
	}
	if b == nil {
		panic("nil Balancer")
	}

	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		var (
			newctx, cancel = context.WithTimeout(ctx, timeout)
			responses      = make(chan interface{}, 1)
			errs           = make(chan error, 1)
			final          RetryError

			d time.Duration = 10 * time.Millisecond
		)
		defer cancel()

		for i := 1; ; i++ {
			go func() {
				e, err := b.Endpoint()
				if err != nil {
					errs <- err
					return
				}
				response, err := e(newctx, request)
				if err != nil {
					errs <- err
					return
				}
				responses <- response
			}()

			select {
			case <-newctx.Done():
				return nil, newctx.Err()

			case response := <-responses:
				return response, nil

			case err := <-errs:
				final.RawErrors = append(final.RawErrors, err)
				keepTrying, replacement := cb(i, err)
				if replacement != nil {
					err = replacement
				}
				if !keepTrying {
					final.Final = err
					return nil, final
				}
				if !retryable(err) {
					final.Final = err
					return nil, final
				}
				if err := sleepWithContext(newctx, d); err != nil {
					return nil, err
				}
				d = utils.Exponential(d)
				continue
			}
		}
	}
}

// DefaultRetryable is the conservative default classifier used by Retry.
// It avoids retrying local context cancellation, errors that explicitly opt out
// via Retryable() bool, and gRPC statuses that usually represent caller or
// authorization failures rather than a transient backend outage.
func DefaultRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var classified interface{ Retryable() bool }
	if errors.As(err, &classified) {
		return classified.Retryable()
	}

	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Canceled,
			codes.InvalidArgument,
			codes.NotFound,
			codes.AlreadyExists,
			codes.PermissionDenied,
			codes.Unauthenticated,
			codes.FailedPrecondition,
			codes.OutOfRange,
			codes.Unimplemented:
			return false
		}
	}

	return true
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
