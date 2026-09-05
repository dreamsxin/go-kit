package endpoint

// This file is what keeps the endpoint package at the bottom of the layering:
// it depends on nothing but the standard library.
//
// Classification is a policy, and apperror is where the framework's policy
// lives. If endpoint imported it, nobody could take Endpoint, Middleware, and
// Chain without also taking that policy. So endpoint speaks the structural
// contract instead — the same one apperror documents for anyone who must not
// depend on it directly, and the same one the generated SDKs use.

// kindNamer is the structural classification contract. An error that names its
// kind is classified consistently by every transport, whether or not it knows
// this framework exists.
//
// It matches apperror.KindNamer by shape. The transports read the typed
// apperror.Kinder first and fall back to this, so an error implementing either
// one maps to the same status; the names below are the apperror kind names.
type kindNamer interface {
	ErrorKindName() string
}

// The kind names this package produces and reads. They are the apperror kind
// name strings, repeated here rather than imported: a string constant is a
// cheaper coupling than a package dependency, and the transports translate
// whichever of the two contracts an error implements.
const (
	kindCanceled          = "canceled"
	kindDeadlineExceeded  = "deadline_exceeded"
	kindInternal          = "internal"
	kindInvalidArgument   = "invalid_argument"
	kindResourceExhausted = "resource_exhausted"
	kindUnavailable       = "unavailable"
)

// classifiedError is a framework-owned error that names its own kind and code.
// It is the local stand-in for an apperror value, used where this package must
// produce a classified error of its own.
type classifiedError struct {
	kind    string
	code    string
	message string
}

func (e *classifiedError) Error() string { return e.message }

// ErrorKindName implements the structural classification contract.
func (e *classifiedError) ErrorKindName() string { return e.kind }

// ErrorCode implements transport/http.ErrorCoder, so the stable machine-readable
// code survives to the client even when the message is redacted.
func (e *classifiedError) ErrorCode() string { return e.code }
