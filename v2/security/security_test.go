package security_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/security"
)

type credentialKey struct{}

func withCredential(ctx context.Context, credential string) context.Context {
	return context.WithValue(ctx, credentialKey{}, credential)
}

// keyAuthenticator resolves subjects from a credential stored in the context,
// mirroring what HTTP boundary middleware would place there.
var keyAuthenticator = security.AuthenticatorFunc(func(ctx context.Context) (security.Subject, error) {
	credential, _ := ctx.Value(credentialKey{}).(string)
	switch credential {
	case "":
		return security.Subject{}, nil // anonymous
	case "reader-key":
		return security.Subject{ID: "reader", Kind: security.SubjectUser, Roles: []string{"reader"}}, nil
	case "admin-key":
		return security.Subject{ID: "admin", Kind: security.SubjectUser, Roles: []string{"reader", "admin"}}, nil
	default:
		return security.Subject{}, apperror.Unauthenticated("auth.unknown_key", "unknown credentials")
	}
})

func TestMiddlewareInjectsSubject(t *testing.T) {
	var seen security.Subject
	var ok bool
	ep := security.Middleware(keyAuthenticator)(endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
		seen, ok = security.SubjectFromContext(ctx)
		return nil, nil
	}))

	if _, err := ep(withCredential(context.Background(), "reader-key"), nil); err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if !ok || seen.ID != "reader" || !seen.HasRole("reader") {
		t.Fatalf("subject = %+v, ok = %v", seen, ok)
	}
}

func TestMiddlewareAnonymousPassesWithoutSubject(t *testing.T) {
	var ok bool
	ep := security.Middleware(keyAuthenticator)(endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
		_, ok = security.SubjectFromContext(ctx)
		return nil, nil
	}))

	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if ok {
		t.Fatal("anonymous request carried a subject")
	}
}

func TestMiddlewareReturnsAuthenticatorError(t *testing.T) {
	ep := security.Middleware(keyAuthenticator)(endpoint.Nop)
	_, err := ep(withCredential(context.Background(), "bogus"), nil)

	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorKind() != apperror.KindUnauthenticated {
		t.Fatalf("error = %v, want unauthenticated apperror", err)
	}
}

func TestRequireAuthenticatedRejectsAnonymous(t *testing.T) {
	ep := security.RequireAuthenticated()(endpoint.Nop)
	_, err := ep(context.Background(), nil)

	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorKind() != apperror.KindUnauthenticated {
		t.Fatalf("error = %v, want unauthenticated apperror", err)
	}
}

func TestRequireAuthenticatedAllowsSubject(t *testing.T) {
	ctx := security.WithSubject(context.Background(), security.Subject{ID: "reader"})
	if _, err := security.RequireAuthenticated()(endpoint.Nop)(ctx, nil); err != nil {
		t.Fatalf("endpoint: %v", err)
	}
}

func TestRequireRoleEnforcesRole(t *testing.T) {
	authed := security.Middleware(keyAuthenticator)
	admin := authed(security.RequireRole("admin")(endpoint.Nop))

	if _, err := admin(withCredential(context.Background(), "admin-key"), nil); err != nil {
		t.Fatalf("admin call: %v", err)
	}

	_, err := admin(withCredential(context.Background(), "reader-key"), nil)
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorKind() != apperror.KindPermissionDenied {
		t.Fatalf("reader error = %v, want permission_denied apperror", err)
	}

	_, err = admin(context.Background(), nil)
	if !errors.As(err, &appErr) || appErr.ErrorKind() != apperror.KindPermissionDenied {
		t.Fatalf("anonymous error = %v, want permission_denied apperror", err)
	}
}

func TestSubjectHasRole(t *testing.T) {
	subject := security.Subject{Roles: []string{"reader"}}
	if !subject.HasRole("reader") || subject.HasRole("admin") {
		t.Fatal("HasRole misclassified roles")
	}
}
