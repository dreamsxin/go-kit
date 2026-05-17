# Security Hardening Guide

Purpose:
- Define the recommended security posture for production `go-kit` services and generated projects.

Read this when:
- You are preparing a service for production exposure.
- You are adding authentication, authorization, request limits, or audit behavior.
- You are reviewing generated projects for safe ownership boundaries.

See also:
- [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
- [OBSERVABILITY.md](OBSERVABILITY.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)
- [transport/README.md](transport/README.md)
- [endpoint/README.md](endpoint/README.md)

## Security Boundary

`go-kit` provides composition points for security policy, but it does not own identity infrastructure.

Applications should provide:

- token validation
- session validation
- user and service identity lookup
- tenant and role policy
- durable audit storage
- edge-layer TLS and network policy

Framework code should provide:

- stable middleware and hook points
- request correlation
- transport metadata extraction
- endpoint-level policy composition
- generated-project ownership boundaries

## Authentication

Authentication belongs at the transport edge.

Recommended flow:

1. A transport `Before` hook extracts protocol credentials from headers or metadata.
2. Application code validates the credential using its identity provider.
3. The verified subject is stored in `context.Context`.
4. Endpoint middleware reads the subject and applies transport-neutral policy.

For HTTP, common inputs are:

- `Authorization: Bearer <token>`
- `X-Request-ID`
- `X-Trace-ID`

For gRPC, use metadata with equivalent keys. Do not put token parsing in service business methods.

## Authorization

Authorization should be expressed as endpoint middleware when it applies to normal service APIs.

Use endpoint middleware for:

- operation-level access rules
- tenant checks
- role checks
- ownership checks
- request-scoped policy decisions

Use service logic only for domain authorization that needs domain state and cannot be separated cleanly.

For AI-facing preview tool loops, use:

- `interaction.AuthorizationHook`
- `interaction.AuditHook`

These APIs remain preview, but they show the intended policy shape for interaction runtimes.

## Request Limits

Every production service should define limits at more than one layer.

Edge or gateway:

- TLS policy
- maximum body size
- per-client request rate
- connection limits

Transport:

- decode size limits where available
- request timeout
- request ID propagation
- metadata validation

Endpoint:

- `endpoint/ratelimit` for rate limiting
- `endpoint.BackpressureMiddleware` for in-flight concurrency limits
- `endpoint.TimeoutMiddleware` for execution deadlines
- circuit breakers for outbound or dependency-sensitive calls

Application:

- input validation
- pagination limits
- bounded batch sizes
- bounded streaming callback queues

For generated gRPC streaming code, use context deadlines/cancellation and bounded application queues for long-running stream consumers.

## Audit

Audit records should be application-owned and durable.

Recommended fields:

- timestamp
- request ID
- trace ID
- subject
- operation or tool name
- resource identifier when available
- decision or result
- error class when denied or failed
- source address when available

Use endpoint middleware for stable service API audit records. Use `interaction.AuditHook` for preview AI interaction tool loops.

Do not rely on logs alone for required audit trails. Logs are useful operational evidence, but durable audit storage should be queryable and retained by policy.

## Generated Projects

Generated projects separate generator-owned and user-owned files.

Keep security customization in user-owned seams:

- `endpoint/<service>/custom_chain.go`
- `cmd/custom_routes.go`
- service implementation files under `service/<service>/`
- application-owned config and secret loading code

Do not hand-edit generator-owned files such as:

- `cmd/generated_*.go`
- `endpoint/<service>/generated_chain.go`
- `model/generated_*.go`
- `repository/generated_*.go`
- `client/`
- `sdk/`
- `skill/`

Run `microgen extend -check -out .` before applying extend operations to a maintained generated project.

## Secrets And Config

Do not commit secrets to generated config files.

Recommended practice:

- keep local YAML values non-secret
- use environment variables for local development overrides
- use remote config or a secret manager for production secrets
- fail closed when required secrets are missing
- log secret presence or source, not secret values

Generated config modes should not make the local quick-start path depend on production secret infrastructure.

## Error Responses

Transport error encoders should avoid leaking sensitive details.

Recommended behavior:

- return structured errors with stable codes
- log internal details server-side with request and trace IDs
- return generic messages for authentication and authorization failures
- distinguish user input validation from internal failures

## Release Expectations

Before a production or stable release:

- run the release validation loop in [RELEASE.md](RELEASE.md)
- verify authentication is implemented at the transport edge
- verify authorization is implemented as endpoint middleware or interaction hooks
- verify request limits exist at edge, transport, endpoint, and application layers
- verify audit records are durable when required by the deployment
- verify generated-project security code lives in user-owned seams
