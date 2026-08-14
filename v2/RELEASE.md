# v2 Release Policy

## Current Position

v2.0.0 is the published stable baseline of the independent module:

```text
github.com/dreamsxin/go-kit/v2
```

The current `main/v2` source contains the incompatible direct refactor tracked
in [ROADMAP.md](ROADMAP.md). It is not covered by the `v2.0.0` compatibility
contract. On 2026-08-14, the repository owner approved publishing this refactor
under `/v2` as a documented one-time SemVer exception instead of changing the
module path to `/v3`.

This approval fixes the publication path and `RELEASE_MANIFEST.json` selects
`v2.1.0` as the root candidate. It does not authorize moving or recreating
`v2.0.0`, skipping release gates, or creating tags before the candidate passes.
The release must identify the source break prominently in release notes,
[CHANGELOG.md](CHANGELOG.md), and [MIGRATION.md](MIGRATION.md). The reviewed
dependency closure and baseline comparison are recorded in
[DEPENDENCY_REPORT.md](DEPENDENCY_REPORT.md).

The published module is stored in the repository's `v2` major-version
subdirectory, but consumers request the normal module version `v2.0.0`. Its tag
is the root tag `v2.0.0`, not `v2/v2.0.0`. A future `/v3` module would likewise
use a root `v3.0.0` tag. v1 release history remains in the repository root and
is not duplicated here.

The historical incorrect tag `v2/v2.0.0` has been removed. The published root
tag `v2.0.0` points at the release commit and resolves as
`github.com/dreamsxin/go-kit/v2@v2.0.0` through the public Go proxy.

## Versioning

Except for the explicitly approved direct-refactor release, v2 follows semantic
versioning from v2.0.0 onward:

- patch: compatible fixes and documentation corrections;
- minor: backward-compatible public capabilities;
- major: incompatible runtime API, module, CLI, configuration, or generated
  ownership changes.

The approved exception applies only to the incompatible refactor currently
tracked as Milestone 6. Normal compatibility rules resume after that release;
the exception is not precedent for unrelated breaking changes.

The compatibility contract includes:

- exported runtime APIs;
- module and package paths;
- documented `microgen` flags;
- generated user-owned file locations;
- documented generated configuration keys and precedence;
- protocol behavior documented as stable.

Templates and packages under `cmd/microgen` are internal implementation details,
but their generated public behavior is a product surface.

## v2.0.0 Entry Criteria (Complete)

The v2.0.0 release commit satisfies these criteria:

- `kit`, endpoint, HTTP/gRPC transport, service discovery, and interaction
  lifecycles have explicit error and cancellation contracts.
- Generated projects use the `/v2` module and build outside the framework
  repository.
- Go IDL, Protobuf, database, config, extend, and interaction generation paths
  have deterministic integration tests.
- Generated configuration validates before runtime wiring.
- Optional `slog` and OpenTelemetry adapters pass their focused package tests
  without adding a direct adapter dependency to the main module.
- Optional HTTP security middleware validates policy at construction and has
  focused trusted-proxy, IP, CORS, CSRF, header, and streaming-boundary tests.
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
IDL, Protobuf, and database source modes. It also verifies the reviewed public
API snapshot, documentation links and UTF-8, focused race tests, `go vet`, and
tidy module files across the main and nested modules.

After the release candidate is committed, run:

```bash
make release-check-clean
```

This checks the committed v2 scope and the manifest phase without rejecting
unrelated repository-root work in progress.

The equivalent focused Go commands are:

```bash
go test ./...
go test -race ./endpoint ./kit ./interaction/... ./transport/... ./sd/...
go -C ./cmd/microgen test -race ./internal/generator
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
- `make test-security` passes for optional browser-facing HTTP middleware;
- `make test-api`, `make test-boundaries`, `make test-race`, `make test-vet`,
  and `make test-modules` pass;
- Go and TypeScript SDKs match the shared path/query/body/error fixture;
- contract snapshot changes have been reviewed and refreshed explicitly;
- repeat generation produces no second-run diff;
- `git diff --check` passes;
- documentation links resolve;
- no temporary generated files remain;
- the approved tag points at the verified commit containing the matching
  major-version module path;
- after pushing tags, the manifest-driven published checks resolve every module
  through `proxy.golang.org` without a local `replace`.

Intentional exported API changes must be reviewed before refreshing the API
snapshot:

```bash
go -C ./tools test . -run TestPublicAPISurfaceSnapshot -count=1 \
  -args -update-api-snapshot
```

## v2.1.0 Multi-Module Release

`RELEASE_MANIFEST.json` is the source of truth. The root and nested modules do
not share one module version or one tag:

- root runtime: `github.com/dreamsxin/go-kit/v2@v2.1.0`;
- microgen and optional nested modules: independent `v0.1.0` releases;
- examples and repository tools: not published as product modules.

The release is deliberately phased because a nested module cannot require the
new core version until the root tag is available through Go module resolution.

### Phase 1: Core Candidate

The manifest phase is `core-candidate`. All candidate tags must be absent.
Core-dependent nested modules temporarily retain the last published core
requirement while workspace tests exercise them against the local refactor.

1. Require successful Linux and Windows `v2 verify` jobs for the candidate.
2. Run `make release-check-clean` from the candidate commit.
3. Create and push only the root tag:

```bash
git tag -a v2.1.0 -m "go-kit v2.1.0"
git push origin v2.1.0
make verify-published-core
```

### Phase 2: Nested Candidates

After the root module resolves publicly:

1. Change the manifest phase to `nested-candidate`.
2. Change every `dependsOnCore` module requirement from `v2.0.0` to `v2.1.0`.
3. Run `go mod tidy` and `GOWORK=off go test ./...` in every nested module.
4. Commit and rerun the full Linux/Windows release workflow.
5. Run `make release-check-clean`; it now requires the root tag and rejects any
   nested tag that already exists.
6. Create the manifest tags from that verified commit:

```bash
git tag -a v2/cmd/microgen/v0.1.0 -m "microgen v0.1.0"
git tag -a v2/integrations/circuitbreaker/v0.1.0 -m "circuitbreaker v0.1.0"
git tag -a v2/integrations/consul/v0.1.0 -m "consul integration v0.1.0"
git tag -a v2/integrations/grpc/v0.1.0 -m "gRPC integration v0.1.0"
git tag -a v2/integrations/ratelimit/v0.1.0 -m "rate-limit integration v0.1.0"
git tag -a v2/integrations/zap/v0.1.0 -m "Zap integration v0.1.0"
git tag -a v2/kit/grpc/v0.1.0 -m "kit gRPC component v0.1.0"
git tag -a v2/observability/otel/v0.1.0 -m "OpenTelemetry integration v0.1.0"
git push origin \
  v2/cmd/microgen/v0.1.0 \
  v2/integrations/circuitbreaker/v0.1.0 \
  v2/integrations/consul/v0.1.0 \
  v2/integrations/grpc/v0.1.0 \
  v2/integrations/ratelimit/v0.1.0 \
  v2/integrations/zap/v0.1.0 \
  v2/kit/grpc/v0.1.0 \
  v2/observability/otel/v0.1.0
make verify-published
```

If the public proxy has not propagated a new tag yet, wait and rerun the
published check. Do not bypass it with `GOPROXY=direct` or a local `replace`.

### Phase 3: Release Record

After every module resolves publicly, change the manifest phase to `released`,
replace the changelog candidate marker with the release date, and commit the
final release record. In this phase `make release-check-clean` requires every
manifest tag to exist.

## Release Notes

Release notes should describe user-visible behavior, migration actions, and known
limitations. Internal refactor details belong in commits or pull requests unless
they explain an observable change.
