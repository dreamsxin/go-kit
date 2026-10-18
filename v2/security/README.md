# security

Transport-neutral contracts for authenticated subjects.

English | [简体中文](README_zh.md)

This package answers one question: once a protocol boundary has established
**who is calling**, how does that principal travel to the endpoint and service
layers, and who checks the coarse requirements — "authenticated", "holds
role"?

It deliberately does **not** extract or validate credentials. That stays at
the protocol boundary (application HTTP middleware, or
[`security/http`](../security/http/README.md) for CORS, CSRF, headers, and IP
policy). Business authorization beyond subject and roles belongs in the
service layer.

## Division of responsibilities

| Layer | Owns |
| --- | --- |
| `security/http` or your HTTP middleware | credential extraction and validation — protocol specific |
| **this package** | subject propagation and endpoint-level enforcement — transport neutral |
| service layer | business authorization rules beyond subject and roles |

## Subject

`Subject` is the authenticated calling principal:

```go
type Subject struct {
	ID     string
	Kind   SubjectKind // SubjectAnonymous | SubjectUser | SubjectService
	Roles  []string
	Claims map[string]any // authenticator-provided; read-only after authentication
}
```

`HasRole(role)` reports role membership; `Authenticated()` reports whether the
subject identifies a principal — an ID present and not anonymous. Presence in
a context is **not** authentication: `Middleware` stores anonymous subjects
too, so always decide with `Authenticated()`, never with the context lookup
alone.

## Establishing a subject

An `Authenticator` resolves the subject from credentials a transport boundary
already placed in the context:

```go
authenticator := security.AuthenticatorFunc(func(ctx context.Context) (security.Subject, error) {
	token, _ := ctx.Value(bearerKey{}).(string) // put there by your HTTP middleware
	id, ok := keys[token]
	if !ok {
		return security.Subject{}, apperror.Unauthenticated("auth.invalid", "credentials are missing or invalid")
	}
	return security.Subject{ID: id.Subject, Kind: security.SubjectUser, Roles: id.Roles}, nil
})
```

Classify failures with `apperror`: `KindUnauthenticated` maps to HTTP 401
(gRPC `Unauthenticated`), `KindPermissionDenied` to 403. Returning a zero
subject with a nil error marks the request anonymous — the middleware stores
that result, and the enforcement middlewares below reject it.

Boundary code stores the result with `security.WithSubject(ctx, subject)`;
readers use `security.SubjectFromContext` and check `Authenticated`.

## Enforcing at the endpoint

`security.Middleware(auth)` is endpoint middleware that runs the
authenticator before the wrapped endpoint and injects its result. The
requirement middlewares then check it:

- `security.RequireAuthenticated()` — rejects without an authenticated
  subject (`security.unauthenticated`, HTTP 401);
- `security.RequireRole("admin")` — additionally requires the role
  (`security.role_required`, HTTP 403).

Compose them where endpoints are assembled:

```go
svc, err := kit.NewHTTP(addr,
	kit.WithEndpointMiddleware(
		security.Middleware(authenticator),
		security.RequireAuthenticated(),
	),
)

// A public route stays unwrapped; an admin route adds the role check
// route-locally:
kit.HandleJSON(svc, "POST /api/admin", adminHandler,
	kit.RouteMiddleware(security.RequireRole("admin")))
```

Health routes and other public endpoints registered by the component stay
outside the chain and therefore outside enforcement.

For MCP, the same contracts apply at the transport: an `Authorizer` reads
`security.SubjectFromContext` and decides per method — see the
[interaction README](../interaction/README.md) and the MCP tutorial.

## Where to go next

- [authentication tutorial](../docs/tutorial-auth.md): the application-owned
  HTTP boundary this package composes with
- [`security/http`](../security/http/README.md): CORS, CSRF, security
  headers, trusted proxies, IP policy
- [error handling](../docs/errors.md): how the classifications map to statuses
