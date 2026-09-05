package endpoint

import (
	"context"
	"errors"
	"strings"
)

// Validatable is implemented by request types that validate themselves before
// business logic runs.
type Validatable interface {
	Validate() error
}

// FieldError describes one invalid request field.
type FieldError struct {
	Field  string
	Reason string
}

func (e FieldError) Error() string {
	return e.Field + ": " + e.Reason
}

// ValidationError reports one or more invalid request fields. Transports map
// it to a client error (HTTP 400) while the field list stays available for
// structured error bodies.
type ValidationError struct {
	Fields []FieldError
}

// ErrorKindName classifies validation failures through the structural contract
// every transport reads, so they map like any other invalid-argument
// application error.
func (e *ValidationError) ErrorKindName() string {
	return kindInvalidArgument
}

// NewValidationError builds a ValidationError from one field/reason pair.
func NewValidationError(field, reason string) *ValidationError {
	return &ValidationError{Fields: []FieldError{{Field: field, Reason: reason}}}
}

// Add appends one more field failure and returns the error for chaining.
func (e *ValidationError) Add(field, reason string) *ValidationError {
	e.Fields = append(e.Fields, FieldError{Field: field, Reason: reason})
	return e
}

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "invalid request"
	}
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Error())
	}
	return "invalid request: " + strings.Join(parts, "; ")
}

// ValidationMiddleware validates requests that implement Validatable before
// the wrapped endpoint runs. Requests without a Validate method pass through
// unchanged.
//
// Validate may return a *ValidationError to report field-level failures, or
// any other error, which is wrapped into a single-field ValidationError.
//
// Example:
//
//	type CreateUserRequest struct {
//	    Name string
//	}
//
//	func (r CreateUserRequest) Validate() error {
//	    if r.Name == "" {
//	        return endpoint.NewValidationError("name", "is required")
//	    }
//	    return nil
//	}
//
//	ep := endpoint.NewBuilder(createUser).
//	    WithValidation().
//	    Build()
func ValidationMiddleware() Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if v, ok := request.(Validatable); ok {
				if err := v.Validate(); err != nil {
					var verr *ValidationError
					if !errors.As(err, &verr) {
						verr = &ValidationError{Fields: []FieldError{{Field: "", Reason: err.Error()}}}
					}
					return nil, verr
				}
			}
			return next(ctx, request)
		}
	}
}

// WithValidation appends ValidationMiddleware to the Builder.
func (b *Builder) WithValidation() *Builder {
	return b.UseNamed("validation", ValidationMiddleware())
}
