# v2 Release Policy

## Current Position

v2 is under active development in the independent module:

```text
github.com/dreamsxin/go-kit/v2
```

Until v2.0.0 is tagged, exported APIs, CLI flags, and generated layouts may
change without compatibility shims. Changes must still be tested and documented.
v1 release history remains in the repository root and is not duplicated here.

## Versioning

v2 follows semantic versioning after v2.0.0:

- patch: compatible fixes and documentation corrections;
- minor: backward-compatible public capabilities;
- major: incompatible runtime API, module, CLI, configuration, or generated
  ownership changes.

The compatibility contract includes:

- exported runtime APIs;
- module and package paths;
- documented `microgen` flags;
- generated user-owned file locations;
- documented generated configuration keys and precedence;
- protocol behavior documented as stable.

Templates and packages under `cmd/microgen` are internal implementation details,
but their generated public behavior is a product surface.

## v2.0.0 Entry Criteria

- `kit`, endpoint, HTTP/gRPC transport, service discovery, and interaction
  lifecycles have explicit error and cancellation contracts.
- Generated projects use the `/v2` module and build outside the framework
  repository.
- Go IDL, Protobuf, database, config, extend, and interaction generation paths
  have deterministic integration tests.
- Generated configuration validates before runtime wiring.
- Optional `slog` and OpenTelemetry adapters pass their focused package tests
  without adding a direct adapter dependency to the main module.
- Database introspection is read-only and startup migration is opt-in.
- HTTP/MCP limits, protocol checks, streaming timeouts, and concurrency behavior
  are covered by tests.
- README quick starts and migration examples compile against the release API.
- `go test ./...` and the targeted race suite pass on a clean checkout.
- `CHANGELOG.md` contains only v2 history and has no unresolved release blockers.

## Release Validation

Install Node.js with `npx`, then run from `v2`:

```bash
make verify-release
```

The release target includes the normal Go validation plus generated OpenAPI
3.1 parsing, JSON Schema 2020-12 compilation, TypeScript SDK type-checks,
cross-SDK HTTP behavior checks, and deterministic contract snapshots for Go
IDL, Protobuf, and database source modes.

The equivalent focused Go commands are:

```bash
go test ./...
go test -race ./kit ./interaction ./sd/... ./cmd/microgen/generator
go vet ./...
```

Generate external smoke projects for each affected source mode and run:

```bash
go mod tidy
go test ./...
```

Also verify:

- `make test-contracts` passes with the pinned TypeScript compiler;
- `make test-observability` passes for the standard-library and OpenTelemetry
  adapters;
- Go and TypeScript SDKs match the shared path/query/body/error fixture;
- contract snapshot changes have been reviewed and refreshed explicitly;
- repeat generation produces no second-run diff;
- `git diff --check` passes;
- documentation links resolve;
- no temporary generated files remain;
- the tag is created from the commit containing `v2/go.mod`.

## Release Notes

Release notes should describe user-visible behavior, migration actions, and known
limitations. Internal refactor details belong in commits or pull requests unless
they explain an observable change.
