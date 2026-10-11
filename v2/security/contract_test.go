package security_test

import (
	"context"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/security"
)

// authKey is the boundary key the test's HTTP boundary would use; the
// contract tests place subjects directly with WithSubject.
type authKey struct{}

type contractAuthenticator struct {
	subject security.Subject
	err     error
}

func (a contractAuthenticator) Authenticate(ctx context.Context) (security.Subject, error) {
	return a.subject, a.err
}

func contractEndpoint(next endpoint.Endpoint) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		return next(ctx, request)
	}
}

// TestContractFailsClosedWithoutAuthenticator: a contract that demands
// authentication never runs unauthenticated — a nil authenticator answers
// every request 401 with the actionable not-configured code.
func TestContractFailsClosedWithoutAuthenticator(t *testing.T) {
	ep := security.Contract(nil, nil)(contractEndpoint(func(context.Context, any) (any, error) {
		return "ran", nil
	}))
	_, err := ep(context.Background(), "req")
	appErr, ok := err.(*apperror.Error)
	if !ok {
		t.Fatalf("error = %v, want *apperror.Error", err)
	}
	if appErr.ErrorKind() != apperror.KindUnauthenticated {
		t.Errorf("kind = %v, want unauthenticated", appErr.ErrorKind())
	}
	if appErr.ErrorCode() != "auth.not_configured" {
		t.Errorf("code = %q, want auth.not_configured", appErr.ErrorCode())
	}
}

// TestContractAuthenticatesAndAuthorizes: the happy path establishes the
// subject, requires authentication, and enforces any-of roles.
func TestContractAuthenticatesAndAuthorizes(t *testing.T) {
	auth := contractAuthenticator{subject: security.Subject{
		ID: "u1", Kind: security.SubjectUser, Roles: []string{"operator"},
	}}
	ep := security.Contract(auth, []string{"admin", "operator"})(
		contractEndpoint(func(ctx context.Context, request any) (any, error) {
			subject, _ := security.SubjectFromContext(ctx)
			return subject.ID, nil
		}),
	)
	// The subject must be established by the middleware before the handler.
	got, err := ep(context.Background(), "req")
	if err != nil {
		t.Fatalf("authorized call failed: %v", err)
	}
	if got != "u1" {
		t.Errorf("subject = %v, want the authenticated id", got)
	}
}

// TestContractRejectsMissingRoles: an authenticated caller without any of
// the required roles is 403.
func TestContractRejectsMissingRoles(t *testing.T) {
	auth := contractAuthenticator{subject: security.Subject{
		ID: "u2", Kind: security.SubjectUser, Roles: []string{"reader"},
	}}
	ep := security.Contract(auth, []string{"admin"})(contractEndpoint(func(context.Context, any) (any, error) {
		return "ran", nil
	}))
	_, err := ep(context.Background(), "req")
	appErr, ok := err.(*apperror.Error)
	if !ok || appErr.ErrorKind() != apperror.KindPermissionDenied {
		t.Fatalf("error = %v, want permission_denied", err)
	}
}

// TestContractRejectsUnauthenticated: an authenticator that returns an
// anonymous subject fails the requirement, not the transport.
func TestContractRejectsUnauthenticated(t *testing.T) {
	auth := contractAuthenticator{} // zero subject, nil error: anonymous
	ep := security.Contract(auth, nil)(contractEndpoint(func(context.Context, any) (any, error) {
		return "ran", nil
	}))
	_, err := ep(context.Background(), "req")
	appErr, ok := err.(*apperror.Error)
	if !ok || appErr.ErrorCode() != "security.unauthenticated" {
		t.Fatalf("error = %v, want security.unauthenticated", err)
	}
}
