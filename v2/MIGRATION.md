# Upgrade Notes

English | [简体中文](MIGRATION_zh.md)

go-kit v2 follows semantic versioning. This page records the upgrade actions
needed for the current release; the complete per-release change list lives in
[CHANGELOG.md](CHANGELOG.md).

## Upgrading To v2.3.0

`v2.3.0` is backward compatible: additive capabilities and behavioral fixes
only. No source changes are required.

Two behavioral notes are worth reviewing during upgrade:

- Trace IDs minted by `endpoint.TracingMiddleware` are 32 lowercase hex
  characters (W3C Trace Context format) instead of 16. Callers that treat the
  ID as an opaque string are unaffected.
- `endpoint.ErrBackpressure` and the new `endpoint.ErrBulkheadFull` are
  encoded as HTTP 429 instead of 500.

## Compatibility Policy

- Patch releases fix behavior; minor releases add capabilities. Both are
  backward compatible within `/v2`.
- Incompatible changes require a new major module version.
- v2 does not carry deprecated forwarding APIs. Documentation for earlier
  releases remains available through the immutable release tags.
