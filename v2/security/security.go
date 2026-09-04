// Package security defines transport-neutral contracts for authenticated
// subjects.
//
// Authentication establishes the calling principal at the protocol boundary
// (for example HTTP middleware that extracts and validates a bearer token);
// business authorization rules stay in endpoint or service policy. This
// package provides the missing middle: a standard way to carry the
// established principal across layers and to enforce coarse endpoint-level
// requirements such as "authenticated" or "holds role".
//
// Division of responsibilities:
//
//   - security/http (or application HTTP middleware): credential extraction
//     and validation, protocol specific;
//   - security (this package): subject propagation and endpoint-level
//     enforcement, transport neutral;
//   - service layer: business authorization rules beyond subject and roles.
//
// Failures are classified with apperror so transports map them uniformly:
// missing or invalid credentials return KindUnauthenticated (HTTP 401) and
// insufficient privileges return KindPermissionDenied (HTTP 403).
package security

import (
	"context"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// SubjectKind classifies the authenticated principal.
type SubjectKind string

const (
	// SubjectAnonymous marks a caller whose credentials were not required.
	SubjectAnonymous SubjectKind = "anonymous"
	// SubjectUser marks an interactive human principal.
	SubjectUser SubjectKind = "user"
	// SubjectService marks a machine or service-account principal.
	SubjectService SubjectKind = "service"
)

// Subject is the authenticated calling principal established at the protocol
// boundary. The zero value is not a valid subject.
//
// Claims carry authenticator-provided attributes and must be treated as
// read-only after authentication; implementations should copy caller-owned
// maps before storing them.
type Subject struct {
	ID     string
	Kind   SubjectKind
	Roles  []string
	Claims map[string]any
}

// HasRole reports whether the subject holds the given role.
func (s Subject) HasRole(role string) bool {
	for _, r := range s.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Authenticated reports whether the subject identifies a principal, which is
// what RequireAuthenticated and RequireRole enforce.
//
// Being present in a context is not the same as being authenticated: Middleware
// stores whatever the Authenticator returned, and an Authenticator marks a
// caller anonymous by returning a zero Subject or SubjectAnonymous with a nil
// error. A subject is authenticated when it has an ID and is not anonymous —
// roles and claims alone do not identify anyone.
func (s Subject) Authenticated() bool {
	return s.ID != "" && s.Kind != SubjectAnonymous
}

// Authenticator establishes a subject from credentials the transport layer
// has already placed in the context. Implementations return the resolved
// subject, or an apperror-classified failure: KindUnauthenticated for
// missing or invalid credentials, KindPermissionDenied when credentials are
// valid but the caller may not proceed.
//
// Returning a zero Subject with a nil error marks the request anonymous;
// downstream RequireAuthenticated and RequireRole middlewares then reject it
// unless the route is intentionally public.
type Authenticator interface {
	Authenticate(ctx context.Context) (Subject, error)
}

// AuthenticatorFunc adapts a function into an Authenticator.
type AuthenticatorFunc func(ctx context.Context) (Subject, error)

// Authenticate calls f.
func (f AuthenticatorFunc) Authenticate(ctx context.Context) (Subject, error) {
	return f(ctx)
}

type subjectKey struct{}

// WithSubject stores the authenticated subject in the context. Transport
// boundary code calls it after successful authentication; endpoint and
// service code reads the subject through SubjectFromContext.
func WithSubject(ctx context.Context, subject Subject) context.Context {
	return context.WithValue(ctx, subjectKey{}, subject)
}

// SubjectFromContext extracts the subject a transport boundary stored in the
// context. The boolean reports only whether a subject is present — Middleware
// stores anonymous subjects too, so use Subject.Authenticated to decide whether
// a principal was actually established.
func SubjectFromContext(ctx context.Context) (Subject, bool) {
	subject, ok := ctx.Value(subjectKey{}).(Subject)
	return subject, ok
}

// Middleware returns endpoint middleware that authenticates each request
// before the wrapped endpoint runs. On success the subject is injected into
// the context as the Authenticator returned it, anonymous ones included, so a
// downstream reader sees the authentication result rather than its absence.
// Authenticator errors are returned unchanged, so classify them with apperror to
// control the mapped transport status. A nil authenticator panics.
//
// Compose it with kit.WithEndpointMiddleware or a per-endpoint Builder; the
// HTTP-layer credential extraction that feeds it remains application owned.
func Middleware(auth Authenticator) endpoint.Middleware {
	if auth == nil {
		panic("security: authenticator cannot be nil")
	}
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			subject, err := auth.Authenticate(ctx)
			if err != nil {
				return nil, err
			}
			return next(WithSubject(ctx, subject), request)
		}
	}
}

// RequireAuthenticated returns endpoint middleware that rejects requests
// without an authenticated subject using KindUnauthenticated (HTTP 401).
// Anonymous and identity-less subjects are rejected as well: presence in the
// context is not authentication, Subject.Authenticated is. Install it after
// Middleware on routes that require a principal; public routes such as health
// checks stay unwrapped.
func RequireAuthenticated() endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if subject, ok := SubjectFromContext(ctx); !ok || !subject.Authenticated() {
				return nil, apperror.New(apperror.KindUnauthenticated,
					"security.unauthenticated", "request was not authenticated")
			}
			return next(ctx, request)
		}
	}
}

// RequireRole returns endpoint middleware that rejects subjects not holding
// the given role using KindPermissionDenied (HTTP 403). Requests without an
// authenticated subject are rejected with the same classification. Business
// authorization rules beyond coarse role checks belong in the service layer.
func RequireRole(role string) endpoint.Middleware {
	if role == "" {
		panic("security: role cannot be empty")
	}
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			subject, ok := SubjectFromContext(ctx)
			if !ok || !subject.Authenticated() || !subject.HasRole(role) {
				return nil, apperror.New(apperror.KindPermissionDenied,
					"security.role_required", "caller does not hold required role")
			}
			return next(ctx, request)
		}
	}
}
