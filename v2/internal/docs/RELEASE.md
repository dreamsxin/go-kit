# v2 Release Policy
English | [简体中文](RELEASE_zh.md)

## Current Position

v2.8.0 is the release being published from `main` for the module:

```text
github.com/dreamsxin/go-kit/v2
```

`v2.8.0` is the release that establishes one published module.
Runtime, generator, providers, and adapters ship together: one `require`, one
tag, and no version skew between the framework and the things that plug into it.
Behaviour changes in this release are recorded in
[CHANGELOG.md](../../CHANGELOG.md).

The published module is stored in the repository's `v2` major-version
subdirectory, but consumers request normal module versions such as `v2.8.0`.
Its tag is the root tag `v2.8.0`. Historical v2 module tags, both root and
nested, have been removed; future releases use only the root tag.

## Versioning

v2 is pre-freeze. Until the freeze is declared, a minor release may change
behaviour or remove API; every such change is recorded in `CHANGELOG.md` under
the release that made it.

- patch: fixes and documentation corrections;
- minor: new capabilities, and behaviour or API changes while pre-freeze;
- major: reserved for after the freeze.

There is no per-release upgrade guide. Behaviour and API changes are recorded in
the changelog for the release that introduces them; consumers should pin the
version they have validated.

Once the freeze is declared, incompatible changes require a new major module path.

The compatibility contract at that point covers:

- exported runtime APIs;
- module and package paths;
- documented `microgen` flags;
- generated user-owned file locations;
- documented generated configuration keys and precedence;
- protocol behavior documented as stable.

Templates and packages under `cmd/microgen` are internal implementation details,
but their generated public behavior is a product surface.

## Release Entry Criteria

The v2.8.0 candidate satisfies these criteria:

- `kit`, endpoint, HTTP/gRPC transport, service discovery, and interaction
  lifecycles have explicit error and cancellation contracts.
- Generated projects use the `/v2` module and build outside the framework
  repository.
- Go IDL, Protobuf, database, config, extend, and interaction generation paths
  have deterministic integration tests.
- Generated configuration validates before runtime wiring.
- Optional `slog` and OpenTelemetry adapters pass their focused package tests;
  core package dependency gates still keep them out of HTTP-only builds.
- Optional HTTP security middleware validates policy at construction and has
  focused trusted-proxy, IP, CORS, CSRF, header, and streaming-boundary tests.
- Database introspection is read-only and startup migration is opt-in.
- HTTP/MCP limits, protocol checks, streaming timeouts, and concurrency behavior
  are covered by tests.
- README quick starts and release examples compile against the release API.
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
tidy module files across the root module and repository-only modules.

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
go test -race ./cmd/microgen/internal/generator
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
- after pushing the root tag, the published check resolves
  `github.com/dreamsxin/go-kit/v2` through `proxy.golang.org` without a local
  `replace`, and no historical v2 tag remains.

Intentional exported API changes must be reviewed before refreshing the API
snapshot:

```bash
go -C ./tools test . -run TestPublicAPISurfaceSnapshot -count=1 \
  -args -update-api-snapshot
```

## Release Procedure

`RELEASE_MANIFEST.json` is the source of truth. v2 ships as one module, so a
release is one version and one tag:

- runtime, generator, providers, and adapters: `github.com/dreamsxin/go-kit/v2`;
- `examples`, `tools`, and `tools/contractcheck`: workspace-only, never tagged.

There is one release phase. `TestOnlyOneModuleIsPublishable` and the historical
tag check fail if another published module or old v2 tag is introduced.

### Cut the release

1. Require successful Linux and Windows `v2 verify` jobs for the candidate.
2. Run `make release-check-clean` from the candidate commit. The manifest phase
   is `candidate`, and the check requires the tag to be absent.
3. Create and push the tag:

```bash
git tag -a v2.8.0 -m "go-kit v2.8.0"
git push origin v2.8.0
make verify-published
```

If the public proxy has not propagated the tag yet, wait and rerun the published
check. Do not bypass it with `GOPROXY=direct` or a local `replace`.

### Record the release

Change the manifest phase to `released`, set `releaseDate`, replace the changelog
candidate marker with that date, and commit. In this phase
`make release-check-clean` requires the tag to exist.

## Release Notes

Release notes should describe user-visible behavior and known limitations.
Internal refactor details belong in commits or pull requests unless they explain
an observable change.
