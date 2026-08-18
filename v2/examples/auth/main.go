// Package main demonstrates application-owned authentication and
// authorization middleware composed with kit.
//
// Concepts shown:
//   - authenticate validates a Bearer API key and injects the identity into
//     the request context; missing or unknown credentials get 401
//   - requireRole authorizes a single route; authenticated callers without
//     the role get 403
//   - both failures are classified with apperror, so responses carry stable
//     machine-readable codes instead of prose
//   - public routes (health endpoints) stay reachable without credentials
//
// Run:
//
//	go run ./examples/auth
//
// Test with curl:
//
//	# No credentials: 401 unauthenticated
//	curl -i -X POST http://localhost:8080/api/me -d '{}'
//
//	# Reader key: 200 with the caller identity
//	curl -X POST http://localhost:8080/api/me \
//	     -H "Authorization: Bearer reader-key" -d '{}'
//
//	# Reader key on the admin route: 403 permission denied
//	curl -i -H "Authorization: Bearer reader-key" http://localhost:8080/api/admin
//
//	# Admin key: 200
//	curl -H "Authorization: Bearer admin-key" http://localhost:8080/api/admin
//
//	# Health stays public
//	curl http://localhost:8080/health
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

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	svc, err := kit.New(*httpAddr,
		kit.WithHTTPMiddleware(authenticate(apiKeys)),
		kit.WithRequestID(),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Typed JSON endpoint reading the injected identity.
	type meRequest struct{}
	kit.HandleJSONTyped(svc, "/api/me", func(ctx context.Context, _ meRequest) (identity, error) {
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := svc.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
