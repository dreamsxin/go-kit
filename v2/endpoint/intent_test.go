package endpoint_test

import (
	"context"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

func TestOperationIntentRoundTrip(t *testing.T) {
	intent := endpoint.OperationIntent{
		Purpose:     "load task for status transition",
		AuditReason: "customer requested address correction",
	}
	ctx := endpoint.WithOperationIntent(context.Background(), intent)
	if got := endpoint.OperationIntentFromContext(ctx); got != intent {
		t.Fatalf("intent = %+v, want %+v", got, intent)
	}
}

// TestOperationIntentZeroIsAbsent keeps an empty declaration and an absent one
// indistinguishable, so a middleware cannot tell "no intent" from "an intent
// nobody filled in" and invent a policy for the difference.
func TestOperationIntentZeroIsAbsent(t *testing.T) {
	ctx := endpoint.WithOperationIntent(context.Background(), endpoint.OperationIntent{})
	if got := endpoint.OperationIntentFromContext(ctx); got != (endpoint.OperationIntent{}) {
		t.Fatalf("zero intent read back as %+v, want the zero value", got)
	}
	if got := endpoint.OperationIntentFromContext(context.Background()); got != (endpoint.OperationIntent{}) {
		t.Fatalf("absent intent read back as %+v, want the zero value", got)
	}
}

// TestOperationIntentSurvivesDerivation: the intent is set by the caller and
// read inside the endpoint, past the middleware that derive contexts in
// between.
func TestOperationIntentSurvivesDerivation(t *testing.T) {
	type otherKey struct{}
	ctx := endpoint.WithOperationIntent(context.Background(), endpoint.OperationIntent{Purpose: "report"})
	ctx = context.WithValue(ctx, otherKey{}, "derived")

	observed := endpoint.OperationIntentFromContext(ctx)
	if observed.Purpose != "report" {
		t.Fatalf("purpose = %q after derivation, want report", observed.Purpose)
	}
}

// TestOperationIntentRecordedAlongsideCorrelation shows the reading side a
// deployment writes: a Recorder or a logging attribute function pulls the
// intent out of the context it is already handed.
func TestOperationIntentRecordedAlongsideCorrelation(t *testing.T) {
	var recorded []string
	recorder := endpoint.RecorderFunc(func(ctx context.Context, _ endpoint.Observation) {
		if intent := endpoint.OperationIntentFromContext(ctx); intent.Purpose != "" {
			recorded = append(recorded, intent.Purpose)
		}
	})

	ctx := endpoint.WithOperationIntent(context.Background(), endpoint.OperationIntent{Purpose: "audit-dump"})
	recorder.Observe(ctx, endpoint.Observation{Operation: "GET /tasks"})

	if len(recorded) != 1 || recorded[0] != "audit-dump" {
		t.Fatalf("recorded = %v, want the declared purpose", recorded)
	}
}
