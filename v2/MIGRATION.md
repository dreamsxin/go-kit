# Upgrade Notes

English | [简体中文](MIGRATION_zh.md)

go-kit v2 follows semantic versioning. This page records the upgrade actions
needed for the current release; the complete per-release change list lives in
[CHANGELOG.md](CHANGELOG.md).

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
