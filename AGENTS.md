# Repository Guide For Agents

English | [简体中文](AGENTS_zh.md)

Read this before changing anything. It covers what the repository will not tell
you by being read, and points at the documents that own each subject rather than
repeating them.

## Layout

- One published module: `github.com/dreamsxin/go-kit/v2`, rooted at `v2/`.
  Runtime, generator, providers and adapters all ship from it.
- Workspace-only, never tagged: `v2/tools` (the gates and the release tooling),
  `v2/examples`, `v2/tools/contractcheck`. A second publishable module fails a
  gate.
- The gates live in their own module, so they are not run by `go test ./...` from
  `v2`. Run them with `go -C ./v2/tools test ./...`.

## Before saying a change is done

From `v2/`:

```bash
make verify
```

That is exactly what CI runs. Where `make` is unavailable:

```bash
go -C ./tools run ./releaseverify -root .. -suites fmt,test,standalone,vet,tidy,race
```

Run it from a committed tree. The `tidy` suite diffs the worktree, so any
uncommitted change fails it — including the change you are about to commit. The
`race` suite needs cgo and therefore a C toolchain on `PATH`.

## Snapshots, and the one rule about them

The reviewed files live under `v2/tools/testdata/`, each refreshed by a flag
passed after `-args`:

- `v2/tools/testdata/api_surface.sha256` — `-update-api-snapshot`
- `v2/tools/testdata/package_paths.txt` — `-update-package-paths`
- `v2/tools/testdata/protocol_behaviour.txt` — `-update-protocol-behaviour`
- `v2/tools/testdata/generated_layout.txt` — `-update-generated-layout`
- `v2/tools/testdata/contract_snapshots/` — `-update-contract-snapshots`

The rule: **refreshing is the review.** Two of these store digests rather than
content, so a failure proves only that something moved. Generate the artifact and
read it before refreshing — for the generated contracts that means running
`microgen` into a temporary directory and reading the document. And never weaken
a gate to let your own change through; if a gate blocks you, either the change is
wrong or the gate's promise has to be restated on purpose.

## A promise is a marker in the source

Behaviour the framework commits to is declared where it is implemented:

```go
// Stable: http.sse-headers — a stream answers 200 with text/event-stream, ...
// Covered by: TestSSEServer_WritesEventsAndHeaders
```

Markers must live in non-test source, every test they name must exist in that
package, and adding one requires refreshing `protocol_behaviour.txt`. Three gates
enforce those three things.

## Import rules the gates enforce

- `v2/endpoint` may import only the standard library. Contracts there are
  structural, not nominal.
- `v2/security/http` may import nothing from this module.
- Layering between the other packages is pinned by
  `TestArchitectureDependencyGates`.

## Where writing goes

Every document is a pair: `X.md` has `X_zh.md`, and changing one without the
other is a defect. Record behaviour changes in `v2/CHANGELOG.md`, plans and
findings in `v2/internal/docs/ROADMAP.md`, both with their `_zh` counterparts. Do
not add a new top-level markdown file for something those own.

## Releasing

`v2/RELEASE_MANIFEST.json` is the source of truth for the version and phase. The
procedure, including what may not be worked around, is in
`v2/internal/docs/RELEASE.md`.

## Design stance

Anything a deployment would want to customise is a seam it implements, and a seam
left nil applies no framework policy. A wrong default is not excused by the
standard library having shipped it. `v2/ARCHITECTURE.md` has the rest.
