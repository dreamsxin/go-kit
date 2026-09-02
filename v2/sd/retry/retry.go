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

// Attempt records one completed selection and call. Address is empty when
// selection failed before an instance could be identified.
type Attempt struct {
	Address string
	Err     error
	Latency time.Duration
}

// Error is returned when retry attempts are exhausted.
type Error struct {
	Attempts []Attempt
	Final    error
}

func (e Error) Error() string {
	if len(e.Attempts) == 0 {
		if e.Final != nil {
			return e.Final.Error()
		}
		return "retry failed without an error"
	}
	// Attempts are attributed to the instance that produced them: a retry
	// history is only actionable if it says which endpoint failed.
	described := make([]string, 0, len(e.Attempts))
	for _, attempt := range e.Attempts {
		if attempt.Err != nil {
			described = append(described, describe(attempt.Address, attempt.Err))
		}
	}
	if len(described) == 0 {
		if e.Final != nil {
			return e.Final.Error()
		}
		return "retry failed without an error"
	}
	last := described[len(described)-1]
	if e.Final != nil {
		// Final replaces the error of the final attempt but not its address.
		last = describe(e.Attempts[len(e.Attempts)-1].Address, e.Final)
	}
	if len(described) == 1 {
		return last
	}
	return fmt.Sprintf("%s (previously: %s)", last, strings.Join(described[:len(described)-1], "; "))
}

func describe(address string, err error) string {
	if address == "" {
		return err.Error()
	}
	return address + ": " + err.Error()
}

// Unwrap exposes the final failure for errors.Is and errors.As.
func (e Error) Unwrap() error {
	if e.Final != nil {
		return e.Final
	}
	for i := len(e.Attempts) - 1; i >= 0; i-- {
		if e.Attempts[i].Err != nil {
			return e.Attempts[i].Err
		}
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

		resultChannel := make(chan attemptResult, 1)
		result := Error{}
		delay := 10 * time.Millisecond

		for attempt := 1; ; attempt++ {
			go call(callContext, balancer, request, resultChannel)

			select {
			case <-callContext.Done():
				return nil, callContext.Err()
			case completed := <-resultChannel:
				result.Attempts = append(result.Attempts, Attempt{
					Address: completed.address,
					Err:     completed.err,
					Latency: completed.latency,
				})
				if completed.err == nil {
					return completed.response, nil
				}

				keepTrying, replacement := callback(attempt, completed.err)
				received := completed.err
				if replacement != nil {
					received = replacement
				}
				if !keepTrying || !classifier(received) {
					result.Final = received
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

type attemptResult struct {
	response any
	address  string
	err      error
	latency  time.Duration
}

func call(ctx context.Context, balancer sd.Balancer, request any, results chan<- attemptResult) {
	started := time.Now()
	picked, err := balancer.Pick(ctx, request)
	if err != nil {
		results <- attemptResult{err: err, latency: time.Since(started)}
		return
	}
	if picked.Endpoint == nil {
		err = errors.New("retry: balancer returned nil endpoint")
	} else {
		endpointStarted := time.Now()
		var response any
		response, err = picked.Endpoint(ctx, request)
		if picked.Done != nil {
			picked.Done(sd.Outcome{Err: err, Latency: time.Since(endpointStarted)})
		}
		results <- attemptResult{
			response: response,
			address:  picked.Instance.Address,
			err:      err,
			latency:  time.Since(started),
		}
		return
	}
	if picked.Done != nil {
		picked.Done(sd.Outcome{Err: err, Latency: time.Since(started)})
	}
	results <- attemptResult{address: picked.Instance.Address, err: err, latency: time.Since(started)}
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
