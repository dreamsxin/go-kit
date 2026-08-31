# Upgrade Notes

English | [简体中文](MIGRATION_zh.md)

go-kit v2 follows semantic versioning. This page records the upgrade actions
needed for the current release; the complete per-release change list lives in
[CHANGELOG.md](CHANGELOG.md).

## Migrating From Legacy go-kit (v0/v1 Style)

Codebases built on the old go-kit style (`github.com/go-kit/kit`: per-route
`transport/http.NewServer` with decode/encode function pairs, `endpoint.Set`
structs, `NewGRPCServer`/`NewGRPCClient` assembly blocks) map to v2
constructs like this:

| Legacy construct | go-kit v2 equivalent |
| --- | --- |
| `httptransport.NewServer(ep, decode, encode, opts...)` per route | `kit.HandleJSONTyped` / `kit.HandleJSONWithMiddleware`; `server.NewServer` for custom codecs |
| Per-route decode/encode function pairs | Typed handlers need no codecs; `server.RawBodyCodec` covers custom wire formats |
| `endpoint.Set` struct wiring | Route registration on the `kit.HTTP` component; microgen-generated `endpoints.go` |
| `NewGRPCServer` / grpc handler struct | `integrations/grpc` server + `transport.Binding` (one endpoint serves HTTP and gRPC) |
| `NewGRPCClient` endpoint blocks | `integrations/grpc/client`; `sd/client` composes discovery, balancing, and retry |
| `jwt.HTTPToContext()` / `jwt.GRPCToContext()` | `security` subject contract + application-owned credential extraction at the protocol boundary |
| `opentracing.HTTPToContext` / `TraceClient` | `endpoint.TracingMiddleware` (W3C) or `observability/otel` |
| `ratelimit.NewErroringLimiter` | Built-in `endpoint.RateLimitMiddleware` |
| `switch err.Error() {...}` status mapping | `apperror` classification; transports map kinds; `JSONErrorEncoderWithKindMapper` for custom statuses |
| `Err string` response fields (error-as-string over the wire) | Return errors; `endpoint.Failer` when a proto message must carry the failure |
| Hand-written proto↔domain mappers | Generated transports (`microgen` from `.proto`) |

Pitfalls the legacy style is prone to, fixed by design in v2:

- **Silent middleware drift**: per-method middleware applied by copy-paste or
  comment toggles leaves routes without rate limiting or tracing without any
  signal. In v2 middleware is declared once at component level or explicitly
  per route, and chains are introspectable via `endpoint.Builder.Describe()`.
- **Error classification loss**: errors serialized as strings lose their kind
  across the wire. `apperror` kinds survive across HTTP and gRPC.
- **Mapper drift**: hand-written proto↔domain converters drift from the
  schema over time; generated codecs stay in sync with the contract.

Recommended migration order: move one service at a time; start with typed
JSON handlers (`kit`), then replace error-string conventions with `apperror`,
then collapse duplicate HTTP/gRPC assemblies into `transport.Binding`, and
finally let `microgen` own regenerated transports.

## Upgrading From v2.6.0 (Unreleased)

Two source changes and one behavior change:

1. `endpoint.Metrics` counter fields are unexported. Read through `Snapshot()`,
   which keeps the same field names:

   ```go
   // before
   count := metrics.RequestCount

   // after
   count := metrics.Snapshot().RequestCount
   ```

2. `endpoint.TypeAssertError.Want` is a `reflect.Type`. Build the error with the
   generic constructor instead of a composite literal:

   ```go
   // before
   var zero Req
   return &endpoint.TypeAssertError{Got: request, Want: zero}

   // after
   return endpoint.NewTypeAssertError[Req](request)
   ```

3. An endpoint timeout now encodes as HTTP 504 instead of 500, matching
   `apperror.KindDeadlineExceeded` and the gRPC adapter. Clients and alerting
   rules that treated an endpoint timeout as a 500 must accept 504.

`Fallback` also stops running the fallback for an already-cancelled caller and
joins both errors when the fallback fails; no source change is required, but
callers that relied on the fallback masking every failure will now see the
primary cause.

## Upgrading To v2.6.0

`v2.6.0` is the approved architecture-evolution release and contains
intentional breaking changes. Required source changes:

1. Assembly layer: `kit.Service`, `kit.New`, `kit.MustNew`, and `Service.Run`
   are removed. Split the wiring:

   ```go
   // before
   svc, err := kit.New(":8080", opts...)
   kit.HandleJSONTyped(svc, "/hello", handler)
   svc.Run(ctx)

   // after
   svc, err := kit.NewHTTP(":8080", opts...)
   kit.HandleJSONTyped(svc, "/hello", handler)
   host, err := kit.NewHost(kit.WithLifecycle(svc))
   host.Run(ctx)
   ```

   `Handle`, `HandleFunc`, and `HandleSSE` are now methods on `*kit.HTTP`.
2. Error model: `server.HTTPError`, `server.NewHTTPError`, and
   `server.WrapHTTPError` are removed. Classify failures with `apperror`
   (transports map kinds uniformly, including gRPC); protocol-specific
   customization uses `transporthttp.StatusCoder`, `ErrorCoder`,
   `PublicMessager`, and `Headerer`.
3. The deprecated `log` facade is removed; use `log/slog` directly.
4. SSE: `kit.SSEWriter` is removed. Use `server.SSEStream` (same method set)
   with `kit.HandleSSETyped` (endpoint middleware applies) or the
   `HTTP.HandleSSE` method for raw streams.
5. Generated projects (`microgen` `v0.3.0`): the user-owned
   `cmd/custom_routes.go` hook is standard-library only now —
   `func registerCustomRoutes(r *http.ServeMux)` registers routes directly;
   move listing entries into `customRouteDescriptions() []string`. Extend
   refuses pre-v3 manifests (`microgen.project.v2`) with this migration hint;
   rerun a full generation afterwards to refresh the manifest.

New capabilities worth adopting: `security` subject contracts
(`RequireAuthenticated`, `RequireRole`), `slogadapter.NewTelemetry`,
`kit.NamedLifecycle` / `kit.ReadinessProvider`, and `apperror` convenience
constructors.

## Upgrading To v2.4.1

`v2.4.1` is backward compatible: additive capabilities and behavioral fixes
only. No source changes are required.

Behavioral notes worth reviewing when jumping from `v2.2.0`:

- Trace IDs minted by `endpoint.TracingMiddleware` are 32 lowercase hex
  characters (W3C Trace Context format) instead of 16 (changed in `v2.3.0`).
  Callers that treat the ID as an opaque string are unaffected.
- `endpoint.ErrBackpressure` and `endpoint.ErrBulkheadFull` are encoded as
  HTTP 429 instead of 500 (changed in `v2.3.0`).

## Compatibility Policy

- Patch releases fix behavior; minor releases add capabilities. Both are
  backward compatible within `/v2`.
- Incompatible changes require a new major module version, except approved
  and recorded deviations: the `v2.1.0` direct refactor, the `v2.2.0`
  metrics return-type fix, and the `v2.6.0` architecture evolution.
- v2 does not carry deprecated forwarding APIs. Documentation for earlier
  releases remains available through the immutable release tags.
