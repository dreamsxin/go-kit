# Tutorial: Authentication Middleware

English | [简体中文](tutorial-auth_zh.md)

This tutorial adds Bearer-key authentication and role-based authorization to a
kit service. Authentication is application-owned by design; the framework
provides the middleware boundary and the error classification. The complete
code is the runnable [examples/auth](../examples/README.md) example.

## 1. The goal

| Route | Behavior |
| --- | --- |
| `GET /health` | public |
| `POST /api/me` | any valid key: 200 with the caller identity |
| `GET /api/admin` | admin role required: 403 otherwise |
| everything else | missing or unknown key: 401 |

## 2. The identity

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

## 3. The middleware

Authentication middleware validates the `Authorization` header and injects the
identity. It is installed service-wide with `kit.WithHTTPMiddleware`, so it
wraps every route except the public prefixes:

```go
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

## 4. Authorization on one route

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

## 5. Assemble

```go
svc, err := kit.New(":8080",
	kit.WithHTTPMiddleware(authenticate(apiKeys)),
	kit.WithRequestID(),
)
```

Run it and try the four rows from the goal table:

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
