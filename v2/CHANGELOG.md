# Changelog

English | [简体中文](CHANGELOG_zh.md)

## [2.8.0] - Release Candidate

This is the first public release of the current `go-kit/v2` product line.
The repository ships as one Go module:

```text
github.com/dreamsxin/go-kit/v2
```

Runtime packages, transport adapters, service-discovery providers,
observability adapters, and `cmd/microgen` are versioned together under the
same module and root tag. `examples`, `tools`, and
`tools/contractcheck` remain repository-only workspace modules.

### Included

- `Service -> Endpoint -> Transport` service architecture.
- Typed HTTP JSON handlers, custom codecs, SSE, gRPC adapters, and generated
  Go/TypeScript clients.
- Transport-neutral application errors with consistent HTTP and gRPC mapping.
- Endpoint middleware for validation, timeout, tracing, metrics, recovery,
  rate limiting, circuit breaking, fallback, backpressure, bulkhead, and retry.
- Service discovery snapshots, endpoint caching, balancing, retry, health
  checks, passive ejection, feedback, and long-lived connection accounting.
- Interaction runtime with sessions, tools, resources, prompts, authorization,
  audit hooks, and MCP Streamable HTTP.
- Configuration, OpenAPI, JSON Schema, database scaffolding, and deterministic
  `microgen` extension workflows.
- Explicit lifecycle ownership, bounded shutdown, request correlation, and
  package-level dependency gates.

### Release Contract

- The only published tag is the root `v2.8.0` tag.
- Historical v2 module tags (root and nested) have been removed.
- Future releases use one version and one root tag.
- Behavioral and API changes are recorded in this file; no separate migration
  history is maintained for this initial public baseline.
