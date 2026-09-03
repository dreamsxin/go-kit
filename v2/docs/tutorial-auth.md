# Tutorial: Authentication Middleware

English | [简体中文](tutorial-auth_zh.md)

This tutorial adds Bearer-key authentication and role-based authorization to a
kit service. Authentication is application-owned by design; the framework
provides the middleware boundary and the error classification. The complete
code is the runnable [examples/auth/main.go](../examples/auth/main.go) example.

## 1. The goal

| Route | Behavior |
| --- | --- |
| `/health`, `/livez`, `/readyz` | public |
| `/api/me` | any valid key: 200 with the caller identity |
| `/api/admin` | admin role required: 403 otherwise |
| everything else | missing or unknown key: 401 |

The three health routes are registered by `kit.NewHTTP` itself, so the public
prefix list has to cover all three -- not just `/health`.

## 2. The standard split

Authentication has two boundaries. HTTP middleware extracts a credential from
the wire; the transport-neutral `security` package resolves it to a
`security.Subject` and enforces route requirements in endpoint middleware. This
keeps the service independent of bearer headers, cookies, or another protocol.

```go
type credentialKey struct{}

func extractBearer(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
        ctx := context.WithValue(r.Context(), credentialKey{}, token)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

type bearerAuthenticator struct{ keys map[string]identity }

func (a bearerAuthenticator) Authenticate(ctx context.Context) (security.Subject, error) {
    token, _ := ctx.Value(credentialKey{}).(string)
    id, ok := a.keys[token]
    if !ok || token == "" {
        return security.Subject{}, apperror.Unauthenticated(
            "auth.invalid", "credentials are missing or invalid",
        )
    }
    return security.Subject{
        ID: id.Subject, Kind: security.SubjectUser, Roles: id.Roles,
    }, nil
}
```

Install the extractor at the HTTP boundary and the resolver at the endpoint
boundary. Public health routes remain unprotected because they are raw component
routes; private JSON routes add the requirement explicitly:

```go
svc, _ := kit.NewHTTP(":8080",
    kit.WithHTTPMiddleware(extractBearer),
    kit.WithEndpointMiddleware(security.Middleware(bearerAuthenticator{keys: apiKeys})),
)

kit.HandleJSONTypedWithMiddleware(svc, "POST /api/me", meHandler,
    func(b *endpoint.Builder) *endpoint.Builder {
        return b.Use(security.RequireAuthenticated())
    })
```

Use `security.RequireRole("admin")` for a role-protected route. Authorization
that depends on resource ownership or business state belongs in the service
layer. The rest of this tutorial shows the same flow with small application
HTTP helpers so the wire decisions remain visible.

## 3. The identity

The identity is a plain value carried through the context:

```go
type identity struct {
	Subject string   `json:"subject"`
	Roles   []string `json:"roles"`
}

func identityFromContext(ctx context.Context) (identity, bool) {
	id, ok := ctx.Value(identityKey{}).(identity)
	return id, ok
}
```

## 4. The middleware

Authentication middleware validates the `Authorization` header and injects the
identity. It is installed service-wide with `kit.WithHTTPMiddleware`, which
wraps the whole mux -- there is no per-route exclusion option, so the middleware
itself decides what is public:

```go
var publicPrefixes = []string{"/health", "/livez", "/readyz"}

func authenticate(keys map[string]identity) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublic(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				writeAuthError(w, apperror.New(
					apperror.KindUnauthenticated,
					"auth.bearer_required",
					"missing bearer token",
				))
				return
			}
			id, ok := keys[strings.TrimPrefix(header, "Bearer ")]
			if !ok {
				writeAuthError(w, apperror.New(
					apperror.KindUnauthenticated,
					"auth.unknown_key",
					"unknown credentials",
				))
				return
			}
			next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), id)))
		})
	}
}
```

Failures are classified with `apperror`, so the response carries a stable
machine-readable code instead of prose.

## 5. Authorization on one route

Role checks wrap a single route, after authentication has run:

```go
func requireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := identityFromContext(r.Context())
			if !ok || !id.hasRole(role) {
				writeAuthError(w, apperror.New(
					apperror.KindPermissionDenied,
					"auth.role_required",
					"caller does not hold role "+role,
				))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

svc.Handle("/api/admin", requireRole("admin")(adminHandler))
```

`Handle` takes a bare path, not a `"GET /api/admin"` pattern, so the route
answers every method. Add the verb to the pattern if you want the mux to reject
the others for you.

## 6. Assemble

```go
httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
flag.Parse()

ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

svc, err := kit.NewHTTP(*httpAddr,
	kit.WithHTTPMiddleware(authenticate(apiKeys)),
	kit.WithRequestID(),
)
if err != nil {
	log.Fatal(err)
}
registerRoutes(svc)

host, err := kit.NewHost(kit.WithLifecycle(svc))
if err != nil {
	log.Fatal(err)
}
if err := host.Run(ctx); err != nil {
	log.Fatal(err)
}
```

Run it from the `v2/` module directory and try the rows from the goal table:

```bash
go run ./examples/auth
curl -i -X POST http://localhost:8080/api/me -d '{}'                                  # 401
curl -H "Authorization: Bearer reader-key" -X POST http://localhost:8080/api/me -d '{}'  # 200
curl -i -H "Authorization: Bearer reader-key" http://localhost:8080/api/admin          # 403
curl -H "Authorization: Bearer admin-key" http://localhost:8080/api/admin              # 200
curl http://localhost:8080/health                                                      # 200
```

## Where to go next

- [middleware](middleware.md): composition and flow control
- [error handling](errors.md): the classification and wire format
- [security/http](../security/http/README.md): CORS, CSRF, and IP policy for
  browser-facing services
