# go-kit v2 Book

English | [简体中文](index_zh.md)

The book is the task-oriented companion to the package guides. Package guides
(reference: `endpoint/README.md`, `transport/README.md`, ...) answer "what
does this package provide". The book answers "how do I do X end to end".

## Tutorials

Complete walkthroughs from an empty directory to a running service:

- [Getting started](getting-started.md): install, first service, first request
- [Tutorial: a CRUD service](tutorial-crud.md): SQLite storage, typed JSON
  routes, graceful shutdown
- [Tutorial: generating a service](tutorial-microgen.md): IDL to a complete
  project with clients, SDKs, and OpenAPI
- [Tutorial: authentication middleware](tutorial-auth.md): Bearer keys, roles,
  and public routes
- [Tutorial: an MCP server](tutorial-mcp.md): expose tools to AI clients over
  MCP Streamable HTTP

## Chapters

Cross-cutting flows that span more than one package:

- [Core concepts](concepts.md): the request path, error classification,
  lifecycle ownership
- [Error handling](errors.md): `apperror`, transport mapping, custom error
  formats end to end
- [Middleware](middleware.md): composition, flow control, and the built-in
  catalog
- [Lifecycle](lifecycle.md): startup, graceful shutdown, background jobs
- [Configuration](configuration.md): generated config precedence and custom
  sections
- [Testing](testing.md): testing endpoints and services
- [Troubleshooting](troubleshooting.md): symptom-based failure diagnosis —
  request correlation, status codes, database, logging, debug switches

## Component References

Package guides own the detailed API reference for each component:

- [endpoint](../endpoint/README.md), [transport](../transport/README.md),
  [kit](../README.md#build-with-kit), [sd](../sd/README.md),
  [interaction](../interaction/README.md), [security](../security/http/README.md),
  [observability](../observability/slog/README.md),
  [integrations](../integrations/consul/README.md),
  [microgen](../MICROGEN.md)

## Production

- [Production guide](../PRODUCTION.md)
- [Upgrade notes](../MIGRATION.md)
