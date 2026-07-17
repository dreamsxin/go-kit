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
- Repository-wide UTF-8 validation that rejects BOMs, invalid byte sequences,
  and Unicode replacement characters in maintained text files.
- External generated-project smoke coverage using `go mod tidy` and
  `go test ./...`.
- Shared HTTP path/query codec for generated transports, clients, and SDKs.
- OpenAPI 3.1 generation and a standalone JSON Schema 2020-12 bundle directly
  from the common `microgen` IR.
- Shared non-GET path parameter encoding and decoding for generated transports,
  clients, and SDKs.

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
- Generated Go clients and SDKs use the same complete HTTP paths as server route
  registration and OpenAPI output.
- Generated OpenAPI projects embed Swagger UI 5 assets and serve both
  `/openapi.json` and `/schema.json` without CDN dependencies.
- HTTP JSON client timeout construction is explicit through
  `NewJSONClientWithTimeout`.
- Service-discovery retry defaults to one attempt and only retries explicitly
  classified transient errors when additional attempts are configured.
- Service-discovery endpoint constructors return an owned closer and validate
  required dependencies and timing options before starting background work.
- v2 documentation is task-oriented and no longer duplicates v1 release history,
  temporary roadmaps, or session snapshots.

### Fixed

- Prompt render callbacks no longer run while the provider lock is held.
- Consul retry waits respond to shutdown and repeated `Stop` calls are safe.
- Endpointer shutdown no longer races with producer sends on a closed channel.
- Endpointer shutdown waits for its update loop and releases every client
  resource still owned by the endpoint cache.
- Endpoint caches no longer sort caller slices in place or expose their internal
  endpoint slice to callers.
- Generated environment values remain the highest-priority config source after
  remote loading.
- Generated Go files fail generation before a malformed partial file is written.

### Removed

- v1 compatibility claims and v1.0/v1.6 release planning from v2 documentation.
- Duplicate architecture, generator design, project snapshot, roadmap, stability,
  observability, security, and maintainer documents.
- Duplicate HandyBreaker and built-in Hystrix implementations; Gobreaker is the
  single circuit-breaker adapter in core.
- Redundant `sd.NewEndpointCloser`; lifecycle ownership is part of every
  `sd.NewEndpoint` construction.
- Swagger 2.0 annotation output, `swagger_host`, and `APP_SWAGGER_HOST`; Swagger
  UI now reads the generated `/openapi.json` contract.
