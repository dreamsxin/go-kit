package endpoint

import "context"

type intentKey struct{}

// OperationIntent carries the declared why of one endpoint call: why a read is
// happening and why a state change may be recorded. It is the transport-neutral
// counterpart of a query comment and a write's audit reason.
//
// The framework applies no policy of its own to an intent. It does not require
// one, does not validate one, and does not record one; a caller or middleware
// that wants writes to carry an AuditReason implements that check itself, and
// an intent that is absent or zero means nothing more than that nothing was
// declared. What the pair buys is a standard place: a Recorder, a logging
// middleware's attribute function, and a deployment's own audit middleware all
// read the same value instead of inventing private context keys.
//
// Business values that are not intent — the request payload, the caller's
// chosen names for entities — belong in the request structure, not here. An
// intent is a statement about the operation, so keep it short, stable, and free
// of secrets: whatever a deployment does with it, it ends up in logs or audit
// records.
//
// Stable: endpoint.operation-intent — WithOperationIntent stores the intent and
// OperationIntentFromContext returns it unchanged; an absent or zero intent
// reads back as the zero value, and the framework applies no policy of its own
// to either.
// Covered by: TestOperationIntentRoundTrip, TestOperationIntentZeroIsAbsent, TestOperationIntentSurvivesDerivation, TestOperationIntentRecordedAlongsideCorrelation
type OperationIntent struct {
	// Purpose states why this operation is happening, in words an operator
	// reading a log line can act on, for example "load task for status
	// transition". Empty means no purpose was declared.
	Purpose string
	// AuditReason states why this operation may change state, in words an
	// audit record can carry, for example "customer requested address
	// correction". Empty means no audit reason was declared.
	AuditReason string
}

// WithOperationIntent returns a context carrying intent. A zero intent returns
// the context unchanged, so an absent intent and an empty one are the same
// thing.
func WithOperationIntent(ctx context.Context, intent OperationIntent) context.Context {
	if intent.Purpose == "" && intent.AuditReason == "" {
		return ctx
	}
	return context.WithValue(ctx, intentKey{}, intent)
}

// OperationIntentFromContext returns the intent the context carries. The zero
// value is returned when none was declared.
func OperationIntentFromContext(ctx context.Context) OperationIntent {
	intent, _ := ctx.Value(intentKey{}).(OperationIntent)
	return intent
}
