# v2 Release Policy

## Current Position

v2.0.0 is the published stable baseline of the independent module:

```text
github.com/dreamsxin/go-kit/v2
```

The current `main/v2` source contains the incompatible direct refactor tracked
in [ROADMAP.md](ROADMAP.md). It is not covered by the `v2.0.0` compatibility
contract. A release from this source must use a semantic version appropriate
for the break after all refactor acceptance gates pass.

Do not move or recreate the existing `v2.0.0` tag. Before publishing the
incompatible refactor, the owner must choose one of these paths:

- change the module and integration paths to `/v3` and publish a normal major
  release; this is the SemVer-compliant path;
- explicitly approve a documented v2 SemVer exception and select a new,
  previously unused v2 tag.

Passing repository checks does not make this product decision or authorize a
tag. The reviewed dependency closure and baseline comparison are recorded in
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

v2 follows semantic versioning from v2.0.0 onward:

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

This checks the committed v2 scope without rejecting unrelated repository-root
work in progress.

After all gates pass from that commit and the publication path is approved,
create an annotated source tag from the repository root. Replace the example
version with the approved, previously unused version:

```bash
git tag -a <version> -m "go-kit <version>"
```

The equivalent focused Go commands are:

```bash
go test ./...
go test -race ./kit ./interaction/... ./transport/... ./sd/...
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
- after pushing the tag, `make verify-published VERSION=<version>` resolves the
  module through `proxy.golang.org` without a local `replace`.

Intentional exported API changes must be reviewed before refreshing the API
snapshot:

```bash
go -C ./tools test . -run TestPublicAPISurfaceSnapshot -count=1 \
  -args -update-api-snapshot
```

## Release Notes

Release notes should describe user-visible behavior, migration actions, and known
limitations. Internal refactor details belong in commits or pull requests unless
they explain an observable change.
