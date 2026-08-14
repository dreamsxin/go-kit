// Package retry executes endpoint calls against a dynamic balancer.
package retry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/retry/internal/backoff"
)

// Error is returned when retry attempts are exhausted.
type Error struct {
	RawErrors []error
	Final     error
}

func (e Error) Error() string {
	if len(e.RawErrors) == 0 {
		if e.Final != nil {
			return e.Final.Error()
		}
		return "retry failed without an error"
	}
	var suffix string
	if len(e.RawErrors) > 1 {
		previous := make([]string, len(e.RawErrors)-1)
		for i := range previous {
			previous[i] = e.RawErrors[i].Error()
		}
		suffix = fmt.Sprintf(" (previously: %s)", strings.Join(previous, "; "))
	}
	if e.Final == nil {
		return fmt.Sprintf("%v%s", e.RawErrors[len(e.RawErrors)-1], suffix)
	}
	return fmt.Sprintf("%v%s", e.Final, suffix)
}

// Unwrap exposes the final failure for errors.Is and errors.As.
func (e Error) Unwrap() error {
	if e.Final != nil {
		return e.Final
	}
	if len(e.RawErrors) > 0 {
		return e.RawErrors[len(e.RawErrors)-1]
	}
	return nil
}

// Callback decides whether another attempt should run and may replace the
// error returned to the caller.
type Callback func(attempt int, received error) (keepTrying bool, replacement error)

// Classifier reports whether an error is safe and useful to retry.
type Classifier func(error) bool

// Retry attempts a call up to maxAttempts times within timeout.
func Retry(maxAttempts int, timeout time.Duration, balancer sd.Balancer) endpoint.Endpoint {
	return WithCallback(timeout, balancer, attemptLimit(maxAttempts))
}

func attemptLimit(max int) Callback {
	return func(attempt int, _ error) (bool, error) {
		return attempt < max, nil
	}
}

func alwaysRetry(int, error) (bool, error) { return true, nil }

// WithCallback retries calls according to callback and DefaultClassifier.
func WithCallback(timeout time.Duration, balancer sd.Balancer, callback Callback) endpoint.Endpoint {
	return WithClassifier(timeout, balancer, callback, DefaultClassifier)
}

// WithClassifier retries calls using explicit attempt and error policies.
func WithClassifier(timeout time.Duration, balancer sd.Balancer, callback Callback, classifier Classifier) endpoint.Endpoint {
	if callback == nil {
		callback = alwaysRetry
	}
	if classifier == nil {
		classifier = DefaultClassifier
	}
	if balancer == nil {
		panic("retry: nil balancer")
	}

	return func(ctx context.Context, request any) (any, error) {
		callContext, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		responses := make(chan any, 1)
		errorsChannel := make(chan error, 1)
		result := Error{}
		delay := 10 * time.Millisecond

		for attempt := 1; ; attempt++ {
			go call(callContext, balancer, request, responses, errorsChannel)

			select {
			case <-callContext.Done():
				return nil, callContext.Err()
			case response := <-responses:
				return response, nil
			case callErr := <-errorsChannel:
				result.RawErrors = append(result.RawErrors, callErr)
				keepTrying, replacement := callback(attempt, callErr)
				if replacement != nil {
					callErr = replacement
				}
				if !keepTrying || !classifier(callErr) {
					result.Final = callErr
					return nil, result
				}
				if err := sleep(callContext, delay); err != nil {
					return nil, err
				}
				delay = backoff.Next(delay)
			}
		}
	}
}

func call(ctx context.Context, balancer sd.Balancer, request any, responses chan<- any, errorsChannel chan<- error) {
	selected, err := balancer.Endpoint()
	if err == nil {
		var response any
		response, err = selected(ctx, request)
		if err == nil {
			responses <- response
			return
		}
	}
	errorsChannel <- err
}

// DefaultClassifier retries only errors that explicitly opt in and temporary
// no-endpoint conditions. Protocol-specific classifiers must be supplied by
// application assembly.
func DefaultClassifier(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var classified interface{ Retryable() bool }
	if errors.As(err, &classified) {
		return classified.Retryable()
	}
	return errors.Is(err, sd.ErrNoEndpoints)
}

func sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
