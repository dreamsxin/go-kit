package resilience

import (
	"context"
	"errors"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// The kind names this package produces. They are the apperror kind name
// strings, repeated here rather than imported: a string constant is a cheaper
// coupling than a package dependency, and the transports translate whichever
// of the two contracts an error implements. They mirror the constants the
// endpoint package keeps for its own rejection errors.
const (
	kindCanceled         = "canceled"
	kindDeadlineExceeded = "deadline_exceeded"
	kindUnavailable      = "unavailable"
)

// kindNamer is the structural classification contract endpoint documents. The
// retry classifier reads it without depending on apperror.
type kindNamer interface {
	ErrorKindName() string
}

// shedWaitError reports a call that gave up waiting for admission because its
// own context ended, not because the service refused it.
//
// The distinction is the whole point: it classifies as the context error, so a
// caller timeout stays a timeout (HTTP 504) and a client disconnect stays a
// disconnect (HTTP 499) instead of being relabeled as load shedding (503). The
// shed sentinel stays reachable through errors.Is, so saturation is still
// visible in logs and metrics.
type shedWaitError struct {
	cause error // the context error that ended the wait
	shed  error // the sentinel of the gate that was full, e.g. endpoint.ErrBulkheadFull
	kind  string
}

// newShedWaitError classifies cause, the context error that ended a wait at the
// gate reported by shed.
func newShedWaitError(cause, shed error) error {
	kind := kindDeadlineExceeded
	if errors.Is(cause, context.Canceled) {
		kind = kindCanceled
	}
	return &shedWaitError{cause: cause, shed: shed, kind: kind}
}

func (e *shedWaitError) Error() string {
	return e.shed.Error() + ": " + e.cause.Error()
}

// Unwrap exposes both the context error and the shed sentinel, so errors.Is
// matches context.DeadlineExceeded and the sentinel alike.
func (e *shedWaitError) Unwrap() []error { return []error{e.cause, e.shed} }

// ErrorKindName classifies the failure as the caller's timeout or cancellation.
func (e *shedWaitError) ErrorKindName() string { return e.kind }

// withReportedRetryAfter attaches source's retry hint to err when source knows
// one.
func withReportedRetryAfter(err error, source any) error {
	reporter, ok := source.(endpoint.RetryAfterReporter)
	if !ok {
		return err
	}
	return endpoint.NewRetryAfterError(err, reporter.RetryAfter())
}
