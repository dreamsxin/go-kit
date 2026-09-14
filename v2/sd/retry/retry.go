// Package retry calls a balancer repeatedly until one attempt succeeds.
//
// It differs from endpoint.RetryMiddleware in what it retries. That middleware
// repeats the same endpoint, which is the right thing when a single dependency
// might be briefly unwell. This package picks again for every attempt, so
// a later attempt can land on another instance. Re-picking does not exclude the
// previous address: the balancer's strategy decides which instance is next.
//
// Every attempt reports its outcome to the balancer through sd.Picked.Done, both
// the latency and the error. That is not bookkeeping: least-request, weighted and
// feedback-driven strategies are only correct if outcomes come back, and doing it
// here is why a caller using this package gets those strategies working without
// writing anything.
//
// New configures attempts, budget, classification, and backoff through options.
// Three convenience entry points remain: Retry for a
// simple attempt count, WithCallback when the decision to keep trying depends on
// the attempt or the error, WithClassifier when the definition of "retryable"
// is the application's too. DefaultClassifier is conservative — a context that
// ended is never retried, and an error may opt in through a Retryable method
// returning bool.
//
// The timeout is a budget for all attempts together, not per attempt. A
// non-positive timeout imposes no deadline of its own and the attempts run under
// the caller's context, because passing 0 through would otherwise hand every
// attempt an already expired one.
//
// Idempotency is the caller's to guarantee, as with any retry: this package will
// happily send the same request twice.
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

// Is and As reach the attempts, which the unwrap chain alone does not.
//
// Unwrap can only offer one cause, and Final wins when it is set. A budget expiry
// sets Final to the context error, so an upstream's kind — an apperror recorded in
// every attempt — used to be unreachable: errors.As found only
// context.DeadlineExceeded, and a mapper keyed on kinds emitted a generic timeout for
// a failure whose real kind had been in hand all along. A Callback replacement hid
// the actual error the same way.
//
// errors.Is and errors.As consult these before walking Unwrap, so both the attempts
// and the final cause are reachable. Changing Unwrap to return []error would have
// done the same thing by breaking a published signature; two additions do not.
// Newest attempt first: when several match, the most recent is the one a caller
// means.
//
// Stable: sd.retry-every-cause-is-reachable — errors.Is and errors.As reach the final error and every attempt's error, so a budget expiry does not hide the kind an upstream reported.
// Covered by: TestRetryError_UnwrapExposesEveryCause
func (e Error) Is(target error) bool {
	for i := len(e.Attempts) - 1; i >= 0; i-- {
		if err := e.Attempts[i].Err; err != nil && errors.Is(err, target) {
			return true
		}
	}
	return false
}

// As reports whether any attempt's error matches target, and fills it in when it
// does. See Is for why the attempts need their own path.
func (e Error) As(target any) bool {
	for i := len(e.Attempts) - 1; i >= 0; i-- {
		if err := e.Attempts[i].Err; err != nil && errors.As(err, target) {
			return true
		}
	}
	return false
}

// Callback decides whether another attempt should run and may replace the
// error returned to the caller.
type Callback func(attempt int, received error) (keepTrying bool, replacement error)

// Classifier reports whether an error is safe and useful to retry.
type Classifier func(error) bool

// Retry attempts a call up to maxAttempts times within timeout.
//
// A maxAttempts below 1 is clamped to 1, as in endpoint.RetryMiddleware: the
// call still runs once, because a retry policy is not a way to skip the call.
func Retry(maxAttempts int, timeout time.Duration, balancer sd.Balancer) endpoint.Endpoint {
	return New(balancer, WithMaxAttempts(maxAttempts), WithTimeout(timeout))
}

func alwaysRetry(int, error) (bool, error) { return true, nil }

// WithCallback retries calls according to callback and DefaultClassifier.
func WithCallback(timeout time.Duration, balancer sd.Balancer, callback Callback) endpoint.Endpoint {
	return WithClassifier(timeout, balancer, callback, DefaultClassifier)
}

// WithClassifier retries calls using explicit attempt and error policies.
//
// A non-positive timeout imposes no deadline of its own and the attempts run
// under the caller's context, as in endpoint.TimeoutMiddleware. Passing 0 would
// otherwise hand every attempt an already expired context.
func WithClassifier(timeout time.Duration, balancer sd.Balancer, callback Callback, classifier Classifier) endpoint.Endpoint {
	// Legacy callbacks own their attempt limit, including nil's unlimited policy.
	return New(balancer, func(s *settings) { s.maxAttempts = 0 },
		WithTimeout(timeout), WithAttemptCallback(callback), WithErrorClassifier(classifier))
}

// New builds an endpoint that selects an instance on every attempt. By default
// it makes one attempt, adds no deadline, uses DefaultClassifier and waits on
// the system clock. It owns neither the balancer nor its source: close them
// after callers finish. Nil balancers panic at construction.
//
// Options configure a reusable endpoint; its attempts and default backoff state
// are local to each call. Callers remain responsible for request idempotency.
func New(balancer sd.Balancer, options ...Option) endpoint.Endpoint {
	if balancer == nil {
		panic("retry: nil balancer")
	}
	settings := settings{
		maxAttempts: 1,
		callback:    alwaysRetry,
		classifier:  DefaultClassifier,
		clock:       endpoint.SystemClock(),
	}
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}

	return func(ctx context.Context, request any) (any, error) {
		// Cancellable either way: returning must stop the attempt goroutines
		// even when no deadline was asked for.
		var (
			callContext context.Context
			cancel      context.CancelFunc
		)
		if settings.timeout > 0 {
			callContext, cancel = context.WithTimeout(ctx, settings.timeout)
		} else {
			callContext, cancel = context.WithCancel(ctx)
		}
		defer cancel()

		resultChannel := make(chan attemptResult, 1)
		result := Error{}
		delay := 10 * time.Millisecond

		for attempt := 1; ; attempt++ {
			// The budget is checked before dispatching, not only after. The select
			// below cannot unsend a request: a caller whose context is already
			// cancelled, or whose budget expired while the backoff timer ran, would
			// otherwise have one more attempt put on the wire and then be told the
			// deadline had passed. An upstream write nobody is waiting for is worse
			// than a call that reports the deadline one attempt earlier.
			//
			// Stable: sd.retry-no-attempt-after-the-budget — once a call's budget is spent, no further attempt is dispatched.
			// Covered by: TestRetry_DispatchesNoAttemptOnceTheBudgetIsSpent
			if err := callContext.Err(); err != nil {
				return nil, budgetError(result, err)
			}
			go call(callContext, balancer, request, resultChannel)

			select {
			case <-callContext.Done():
				// Only a success outranks the deadline. select chooses uniformly
				// among ready cases, so an attempt completing in the same instant as
				// the budget expiring had a coin-flip chance of being discarded — and
				// a caller told "deadline exceeded" for a non-idempotent request that
				// in fact succeeded cannot compensate for what it never learned
				// happened.
				//
				// An *error* delivered in that instant is deliberately ignored. It is
				// almost always the context error the attempt observed for itself, so
				// it says nothing the return value does not, and recording it as an
				// attempt would make the shape of the reported error depend on which
				// of the two landed first: the bare context error becomes a
				// retry.Error wrapper on a scheduling coin flip. That regression is
				// what TestRetry_BudgetTimeoutWithoutAttemptsStaysBare caught, under
				// -race only, after this drain was first written to accept both.
				if completed, delivered := delivered(resultChannel); delivered && completed.err == nil {
					result.Attempts = append(result.Attempts, attemptOf(completed))
					return completed.response, nil
				}
				// The budget (or the caller) ended the call. Keep the attempts
				// made so far: "deadline exceeded" alone does not say which
				// instances were tried or how they failed, which is the whole
				// point of retry.Error. Unwrap still exposes the context error,
				// so errors.Is keeps working.
				return nil, budgetError(result, callContext.Err())
			case completed := <-resultChannel:
				result.Attempts = append(result.Attempts, attemptOf(completed))
				if completed.err == nil {
					return completed.response, nil
				}

				keepTrying, replacement := settings.callback(attempt, completed.err)
				received := completed.err
				if replacement != nil {
					received = replacement
				}
				if !keepTrying || (settings.maxAttempts > 0 && attempt >= settings.maxAttempts) || !settings.classifier(received) {
					result.Final = received
					return nil, result
				}
				wait := delay
				if settings.backoff != nil {
					wait = settings.backoff(attempt)
				}
				if err := sleep(callContext, settings.clock, wait); err != nil {
					return nil, budgetError(result, err)
				}
				if settings.backoff == nil {
					delay = backoff.Next(delay)
				}
			}
		}
	}
}

// delivered takes a result an attempt has already produced, without waiting for
// one.
//
// This covers what the handover ordering cannot: a caller's context can be
// cancelled by an unrelated goroutine in the same instant the attempt fills the
// buffered slot, and then both cases of the select are ready and select chooses
// uniformly. No test can produce that window deterministically, which is why it
// carries no Stable marker — the reachable half of the same defect is pinned on
// the handover instead.
func delivered(results <-chan attemptResult) (attemptResult, bool) {
	select {
	case completed := <-results:
		return completed, true
	default:
		return attemptResult{}, false
	}
}

func attemptOf(completed attemptResult) Attempt {
	return Attempt{
		Address: completed.address,
		Err:     completed.err,
		Latency: completed.latency,
	}
}

// budgetError reports a context failure with the attempt history behind it. With
// no attempts there is nothing to add, so the bare context error is returned and
// callers see exactly what the standard library gave us.
func budgetError(result Error, cause error) error {
	if len(result.Attempts) == 0 {
		return cause
	}
	result.Final = cause
	return result
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
		// Hand the result over before reporting the outcome. Done is deployment
		// code — a feedback table, a metric, a log — and every instant it spends
		// between the endpoint returning and the result reaching the loop is an
		// instant in which the budget can expire and an answer already in hand be
		// reported as a timeout.
		//
		// Stable: sd.retry-handover-precedes-the-outcome-callback — an attempt hands its result to the retry loop before the outcome callback runs, so code in Done cannot turn a delivered answer into a deadline error.
		// Covered by: TestRetry_ADeliveredResultOutranksTheDeadline
		results <- attemptResult{
			response: response,
			address:  picked.Instance.Address,
			err:      err,
			latency:  time.Since(started),
		}
		if picked.Done != nil {
			picked.Done(sd.Outcome{Err: err, Latency: time.Since(endpointStarted)})
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

func sleep(ctx context.Context, clock endpoint.Clock, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	timer := clock.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C():
		return ctx.Err()
	}
}
