package endpoint_test

import (
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// The endpoint package does not import apperror, so its errors implement the
// structural apperror.KindNamer contract rather than the typed apperror.Kinder.
// Comparing against the apperror constants here is the point: it pins endpoint's
// own kind strings to apperror's vocabulary without a code dependency, so the
// two cannot drift apart unnoticed.
func TestRejectionErrorsClassifyThemselves(t *testing.T) {
	cases := []struct {
		err  error
		want apperror.Kind
	}{
		{endpoint.ErrRateLimited, apperror.KindResourceExhausted},
		{endpoint.ErrBackpressure, apperror.KindUnavailable},
		{endpoint.ErrBulkheadFull, apperror.KindUnavailable},
		{endpoint.ErrCircuitOpen, apperror.KindUnavailable},
	}
	for _, tc := range cases {
		var namer apperror.KindNamer
		if !errors.As(tc.err, &namer) {
			t.Fatalf("%v does not implement apperror.KindNamer", tc.err)
		}
		if got := namer.ErrorKindName(); got != string(tc.want) {
			t.Errorf("%v: kind name %q, want %q", tc.err, got, string(tc.want))
		}
	}
}

func TestRetryAfterErrorPreservesIdentityAndKind(t *testing.T) {
	err := endpoint.NewRetryAfterError(endpoint.ErrCircuitOpen, 5*time.Second)

	if !errors.Is(err, endpoint.ErrCircuitOpen) {
		t.Fatal("errors.Is must still match the wrapped sentinel")
	}
	var namer apperror.KindNamer
	if !errors.As(err, &namer) || namer.ErrorKindName() != string(apperror.KindUnavailable) {
		t.Fatal("classification must survive the wrapper")
	}
	var reporter endpoint.RetryAfterReporter
	if !errors.As(err, &reporter) {
		t.Fatal("wrapper must report a retry delay")
	}
	if got := reporter.RetryAfter(); got != 5*time.Second {
		t.Errorf("RetryAfter: got %v, want 5s", got)
	}
	if err.Error() != endpoint.ErrCircuitOpen.Error() {
		t.Errorf("message: got %q, want %q", err.Error(), endpoint.ErrCircuitOpen.Error())
	}
}

func TestNewRetryAfterErrorSkipsUselessWrapping(t *testing.T) {
	if got := endpoint.NewRetryAfterError(nil, time.Second); got != nil {
		t.Errorf("nil error: got %v, want nil", got)
	}
	if got := endpoint.NewRetryAfterError(endpoint.ErrRateLimited, 0); got != endpoint.ErrRateLimited {
		t.Errorf("zero delay should return the error unchanged, got %v", got)
	}
}
