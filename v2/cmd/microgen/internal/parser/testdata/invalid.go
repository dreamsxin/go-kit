package invalid

import "context"

// BadService has methods with invalid signatures.
type BadService interface {
	// NoParams has no parameters at all (invalid: needs context + request)
	NoParams() (string, error)
	// OnlyContext has only context (invalid: needs request param)
	OnlyContext(ctx context.Context) (string, error)
	// WrongFirstParam first param is not context.Context
	WrongFirstParam(id int, req string) (string, error)
	// ThreeReturns returns three values (invalid)
	ThreeReturns(ctx context.Context, req string) (string, int, error)
	// NoError last return is not error
	NoError(ctx context.Context, req string) (string, string)
	// ValidMethod is the only valid one
	ValidMethod(ctx context.Context, req string) (string, error)
}
