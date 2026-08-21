# Upgrade Notes

English | [简体中文](MIGRATION_zh.md)

go-kit v2 follows semantic versioning. This page records the upgrade actions
needed for the current release; the complete per-release change list lives in
[CHANGELOG.md](CHANGELOG.md).

## Upgrading To v2.4.0

`v2.4.0` is backward compatible: additive capabilities and behavioral fixes
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
- Incompatible changes require a new major module version.
- v2 does not carry deprecated forwarding APIs. Documentation for earlier
  releases remains available through the immutable release tags.
