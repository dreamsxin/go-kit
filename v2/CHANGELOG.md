# Changelog

All notable v2 changes are recorded here. v1 history remains in the repository
root.

## Unreleased

### Added

- Independent `github.com/dreamsxin/go-kit/v2` module.
- Error-returning `kit.New`, context-driven `Service.Run`, configurable graceful
  shutdown timeout, and `kit.MustNew` for explicit panic-on-invalid setup.
- Final generated configuration validation for server, logging, database,
  middleware, and remote-provider settings.
- Deterministic formatting and text normalization for generated output.
- External generated-project smoke coverage using `go mod tidy` and
  `go test ./...`.
- Shared HTTP path/query codec for generated transports, clients, and SDKs.

### Changed

- `kit` no longer installs process signal handlers or calls fatal logging during
  service lifecycle.
- `Service.GRPCServer` returns an error when gRPC is not configured.
- Generated config precedence is local YAML, optional remote config, final
  environment overrides, then validation.
- Service-discovery registration returns its initial snapshot synchronously and
  publishes later updates without closing consumer channels.
- In-memory interaction providers copy mutable resources, blobs, templates,
  prompts, and render arguments.
- Generated HTTP servers use the standard library `http.ServeMux`; generated GET
  clients and servers share one tagged query contract and do not send JSON bodies.
- HTTP JSON client timeout construction is explicit through
  `NewJSONClientWithTimeout`.
- Service-discovery retry defaults to one attempt and only retries explicitly
  classified transient errors when additional attempts are configured.
- v2 documentation is task-oriented and no longer duplicates v1 release history,
  temporary roadmaps, or session snapshots.

### Fixed

- Prompt render callbacks no longer run while the provider lock is held.
- Consul retry waits respond to shutdown and repeated `Stop` calls are safe.
- Endpointer shutdown no longer races with producer sends on a closed channel.
- Generated environment values remain the highest-priority config source after
  remote loading.
- Generated Go files fail generation before a malformed partial file is written.

### Removed

- v1 compatibility claims and v1.0/v1.6 release planning from v2 documentation.
- Duplicate architecture, generator design, project snapshot, roadmap, stability,
  observability, security, and maintainer documents.
- Duplicate HandyBreaker and built-in Hystrix implementations; Gobreaker is the
  single circuit-breaker adapter in core.
