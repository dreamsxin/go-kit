# Tutorial: Authentication Middleware

English | [简体中文](tutorial-auth_zh.md)

This tutorial adds Bearer-key authentication and role-based authorization to a
kit service. Authentication is application-owned by design; the framework
provides the middleware boundary and the error classification. Every code block
below is complete — follow the sections in order and `main.go` assembles into a
runnable service, the same file as the runnable
[examples/auth/main.go](../examples/auth/main.go) example, which adds only a
longer package comment.

## 1. The goal

| Route | Behavior |
| --- | --- |
| `/health`, `/livez`, `/readyz` | public |
| `/api/me` | any valid key: 200 with the caller identity |
| `/api/admin` | admin role required: 403 otherwise |
| everything else | missing or unknown key: 401 |

The three health routes are registered by `kit.NewHTTP` itself, so the public
prefix list has to cover all three -- not just `/health`.

Authentication has two boundaries. HTTP middleware extracts a credential from
the wire; the transport-neutral `security` package can then resolve it to a
`security.Subject` and enforce route requirements in endpoint middleware
(`security.Middleware`, `security.RequireAuthenticated`) -- that shape keeps
the service independent of bearer headers, cookies, or another protocol, and is
covered in the [security README](../security/README.md). This tutorial
keeps the wire decisions visible in plain HTTP middleware instead; every file
it assembles uses only the standard library and `kit`.

## 2. Start the module

```bash
mkdir authsvc && cd authsvc
go mod init example.com/authsvc
go get github.com/dreamsxin/go-kit/v2@latest
```

Create `main.go` with the package clause and the imports every later section
builds on:

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"
)
```

## 3. The identity

The identity is a plain value carried through the context. Append the type,
its context helpers, the key table a real service would replace with a
database or directory lookup, and the public-prefix list:

```go
// identity is the authenticated caller. The authenticate middleware resolves
// it from credentials; handlers and the requireRole middleware consume it.
type identity struct {
	Subject string   `json:"subject"`
	Roles   []string `json:"roles"`
}

func (i identity) hasRole(role string) bool {
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type identityKey struct{}

func withIdentity(ctx context.Context, id identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

func identityFromContext(ctx context.Context) (identity, bool) {
	id, ok := ctx.Value(identityKey{}).(identity)
	return id, ok
}

// apiKeys maps bearer credentials to identities. A real service resolves
// identities from a database, directory, or token introspection endpoint.
var apiKeys = map[string]identity{
	"reader-key": {Subject: "reader", Roles: []string{"reader"}},
	"admin-key":  {Subject: "admin", Roles: []string{"reader", "admin"}},
}

// publicPrefixes lists routes reachable without credentials.
var publicPrefixes = []string{"/health", "/livez", "/readyz"}

func isPublic(path string) bool {
	for _, prefix := range publicPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
```

## 4. The middleware

Authentication middleware validates the `Authorization` header and injects the
identity. It is installed service-wide with `kit.WithHTTPMiddleware`, which
wraps the whole mux -- there is no per-route exclusion option, so the
middleware itself decides what is public:

```go
// authenticate returns HTTP middleware that validates the Authorization
// header and injects the identity into the request context. Install it
// service-wide with kit.WithHTTPMiddleware.
func authenticate(keys map[string]identity) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublic(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			const prefix = "Bearer "
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, prefix) {
				writeAuthError(w, apperror.New(
					apperror.KindUnauthenticated,
					"auth.bearer_required",
					"missing bearer token",
				))
				return
			}
			id, ok := keys[strings.TrimPrefix(header, prefix)]
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

Role checks wrap a single route, after authentication has run. Append the
middleware and the error renderer it shares with `authenticate`:

```go
// requireRole returns HTTP middleware that allows only identities holding
// the given role. Wrap it around a single route's handler.
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

// writeAuthError renders a classified apperror as the JSON error shape used
// by the HTTP transports, with the HTTP status derived from the kind.
func writeAuthError(w http.ResponseWriter, err *apperror.Error) {
	status := http.StatusInternalServerError
	switch err.ErrorKind() {
	case apperror.KindUnauthenticated:
		status = http.StatusUnauthorized
	case apperror.KindPermissionDenied:
		status = http.StatusForbidden
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    err.ErrorCode(),
		"message": err.PublicMessage(),
	})
}
```

## 6. Assembly

Append the wire-up, and the file is complete. `Handle` takes a bare path, not
a `"GET /api/admin"` pattern, so the admin route answers every method; add the
verb to the pattern if you want the mux to reject the others for you:

```go
func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	svc, err := kit.NewHTTP(*httpAddr,
		kit.WithHTTPMiddleware(authenticate(apiKeys)),
		kit.WithRequestID(),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Typed JSON endpoint reading the injected identity.
	type meRequest struct{}
	kit.HandleJSON(svc, "/api/me", func(ctx context.Context, _ meRequest) (identity, error) {
		id, ok := identityFromContext(ctx)
		if !ok {
			return identity{}, apperror.New(
				apperror.KindUnauthenticated,
				"auth.no_identity",
				"request was not authenticated",
			)
		}
		return id, nil
	})

	// Route-level authorization on a raw handler.
	svc.Handle("/api/admin", requireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"welcome"}`))
	})))

	log.Println("auth example listening on", *httpAddr)

	host, err := kit.NewHost(kit.WithLifecycle(svc))
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := host.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

## 7. Run it

In the tutorial module, run the file you just assembled and try the rows from
the goal table:

```bash
go run .
curl -i -X POST http://localhost:8080/api/me -d '{}'                                  # 401
curl -H "Authorization: Bearer reader-key" -H "Content-Type: application/json" \
     -X POST http://localhost:8080/api/me -d '{}'                           # 200
curl -i -H "Authorization: Bearer reader-key" http://localhost:8080/api/admin          # 403
curl -H "Authorization: Bearer admin-key" http://localhost:8080/api/admin              # 200
curl http://localhost:8080/health                                                      # 200
```

The same service runs from the repository checkout with `go run ./examples/auth`
in the `v2/` module directory.

## Where to go next

- [MICROGEN: declared authentication](../MICROGEN.md#declared-authentication):
  when the service is generated, the IDL doc comment can declare the
  requirement per method and the generated transport enforces it
- [middleware](middleware.md): composition and flow control
- [error handling](errors.md): the classification and wire format
- [security/http](../security/http/README.md): CORS, CSRF, and IP policy for
  browser-facing services
