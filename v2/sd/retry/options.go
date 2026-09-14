package retry

import (
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// Option configures New. Nil options are ignored.
type Option func(*settings)

type settings struct {
	maxAttempts int
	timeout     time.Duration
	callback    Callback
	classifier  Classifier
	clock       endpoint.Clock
	backoff     func(attempt int) time.Duration
}

// WithMaxAttempts bounds total attempts, including the first. The default is
// one; values below one also mean one. The cap still applies when a callback
// asks to keep trying.
func WithMaxAttempts(attempts int) Option {
	return func(s *settings) { s.maxAttempts = max(1, attempts) }
}

// WithTimeout sets a wall-clock budget for all attempts and backoff together.
// A non-positive value (the default) adds no deadline to the caller's context.
// WithClock does not change this budget or the deadlines seen by transports.
func WithTimeout(timeout time.Duration) Option {
	return func(s *settings) { s.timeout = timeout }
}

// WithAttemptCallback runs after each failed attempt. It may stop retrying or
// replace the error used for classification and the final report; original
// failures remain in Error.Attempts. WithMaxAttempts remains an independent cap.
// Nil keeps the existing callback; the default always permits the next attempt.
// The callback must return promptly and be safe for concurrent calls to New's
// returned endpoint.
func WithAttemptCallback(callback Callback) Option {
	return func(s *settings) {
		if callback != nil {
			s.callback = callback
		}
	}
}

// WithErrorClassifier sets which failures may be retried. It receives any
// replacement from WithAttemptCallback. Nil keeps DefaultClassifier (or the
// classifier already configured). The function must be safe for concurrent use.
func WithErrorClassifier(classifier Classifier) Option {
	return func(s *settings) {
		if classifier != nil {
			s.classifier = classifier
		}
	}
}

// WithClock sets the clock used for backoff timers. Nil keeps the configured
// clock, which defaults to endpoint.SystemClock. The total timeout and measured
// attempt latency remain wall-clock values. A clock shared by concurrent calls
// must be safe for concurrent use; endpoint.ManualClock supports this.
//
// Stable: sd.retry-configurable-backoff-clock — New schedules backoff on the configured clock, stops the timer on cancellation, and never dispatches another attempt after cancellation or its wall-clock budget expires.
// Covered by: TestNewRetryUsesConfiguredClockAndBackoff, TestNewRetryCancellationStopsBackoff, TestNewRetryWallClockBudgetBoundsManualWait
func WithClock(clock endpoint.Clock) Option {
	return func(s *settings) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// WithBackoff replaces the delay after failed attempt n, numbered from one.
// A non-positive delay retries immediately, subject to cancellation and the
// attempt cap. Nil keeps the existing schedule. By default the first wait is
// 10ms; each next wait doubles the previous delay with 50-150 percent jitter,
// capped at one minute. Custom delays are used as returned; a caller that needs
// an overall bound must provide a context deadline or WithTimeout.
// The function must return promptly and be safe for concurrent use.
func WithBackoff(backoff func(attempt int) time.Duration) Option {
	return func(s *settings) {
		if backoff != nil {
			s.backoff = backoff
		}
	}
}
