# Optional HTTP security middleware

`security/http` provides composable standard-library `http.Handler`
middleware. It does not define authentication, authorization, identity, or
deployment policy.

```go
proxy, err := httpsecurity.NewTrustedProxy(httpsecurity.TrustedProxyConfig{
    TrustedProxies: []string{"10.0.0.0/8"},
})
ipPolicy, err := httpsecurity.NewIPPolicy(httpsecurity.IPPolicyConfig{
    Allow: []string{"203.0.113.0/24"},
})
cors, err := httpsecurity.NewCORS(httpsecurity.CORSConfig{
    AllowedOrigins:   []string{"https://app.example.com"},
    AllowedMethods:   []string{"GET", "POST"},
    AllowCredentials: true,
})
headers, err := httpsecurity.NewSecurityHeaders(httpsecurity.SecurityHeadersConfig{
    StrictTransportSecurity: "max-age=31536000; includeSubDomains",
})
csrf, err := httpsecurity.NewCSRF(httpsecurity.CSRFConfig{
    Secret:        csrfSecretFromEnvironment,
    RequireOrigin: true,
    SecureCookie:  true,
})

handler := httpsecurity.Chain(
    proxy,
    headers,
    ipPolicy,
    cors,
    csrf,
)(applicationHandler)
```

With `kit`, install the same policies once for health, JSON endpoint, raw HTTP,
and generated routes:

```go
service, err := kit.New(":8080",
    kit.WithHTTPMiddleware(proxy, headers, ipPolicy, cors, csrf),
)
```

Every constructor validates configuration before returning middleware. Keep the
recommended order unless the application has a specific reason to change it:

1. Trusted proxy resolution establishes the effective client IP and scheme.
2. Security headers cover later policy responses and use the trusted scheme for
   HTTPS-only HSTS.
3. IP policy evaluates the effective client IP.
4. CORS answers browser preflight before CSRF validation.
5. CSRF protects only the cookie-authenticated application routes inside it.

## Trust boundaries

- Forwarding headers are ignored unless the direct peer is in
  `TrustedProxies`. Configure only proxies that overwrite or correctly append
  `X-Forwarded-For` and `X-Forwarded-Proto`.
- Deny networks take precedence over allow networks. A non-empty allow list
  denies every unmatched address.
- CORS origins are exact HTTP(S) origins. Wildcard origin cannot be combined
  with credentials.
- CSRF uses an HMAC-signed double-submit cookie. Load its 32-byte-or-longer
  secret from application secret management and scope it only to routes that
  authenticate browsers with cookies. Bearer-token APIs generally do not need
  this middleware.
- Authentication and business authorization still belong in application
  assembly and endpoint/service logic.

## Streaming and MCP

The middleware writes headers before calling the wrapped handler and never
replaces `http.ResponseWriter`, so `Flusher`, `Hijacker`, and streaming behavior
remain available. CORS can therefore wrap SSE routes.

CSRF should not be installed globally over MCP or other non-browser POST
protocols. Apply it only when those routes actually use browser cookies and the
client can obtain the safe-request cookie and echo the token header. Keep CORS
outside CSRF so browser preflight remains token-free.
