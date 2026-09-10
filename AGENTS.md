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

## The change loop

1. **Read first.** For anything past a one-line fix, read the code that changes and
   the code that calls it. The doc comments in this tree carry the reasoning — a
   defect is usually already described next to the thing that has it.
2. **Change the smallest thing that makes the property true.** Not the surrounding
   code, not the naming, not a speculative option nobody asked for.
3. **Verify** (below). A change is not done because it compiles.
4. **Record.** Behaviour goes in `v2/CHANGELOG.md` under the open candidate heading;
   findings, plans and deliberate non-decisions go in
   `v2/internal/docs/ROADMAP.md`. Both have a `_zh` half, and updating one without
   the other is a defect.
5. **Commit and push** (below).

An exclusion is recorded, not skipped: when you decide not to fix something you
found, write down what and why. The next reader cannot tell a considered omission
from an oversight unless it is on paper.

## Before saying a change is done

From `v2/`:

```bash
make verify
```

That runs everything CI runs. CI splits it across two jobs — the suites without
`race`, and `race` alone, because only that one needs a C toolchain — and runs the
release-phase check beside them; the target does all of it, so it cannot come out
greener than CI. Where `make` is unavailable:

```bash
go -C ./tools run ./releaseverify -root .. -suites fmt,test,standalone,vet,tidy,race
go -C ./tools run ./releasecheck -scope .. -manifest ../RELEASE_MANIFEST.json -check-tags
```

Run it from a committed tree. The `tidy` suite diffs the worktree, so any
uncommitted change fails it — including the change you are about to commit. The
`race` suite needs cgo and therefore a C toolchain on `PATH`.

While iterating, the focused targets are cheaper: `make test-runtime`,
`make test-microgen`, `make test-contracts`, `make test-snapshots`,
`make test-boundaries`.

## Commit messages

Conventional Commits with a `v2` scope — `fix(v2):`, `feat(v2):`, `refactor(v2):`,
`docs(v2):`, `chore(v2):` — and `!` when published behaviour changes
incompatibly, which is allowed before the compatibility freeze.

The subject states the property, not the mechanism: `fix(v2)!: the request's Host
is not a credential`, not `fix(v2): change validateOrigin`. The body says what was
wrong, why that was wrong, and what you verified — the diff already says what
changed. When the fix is a regression this repository shipped, say so.

Write the message to a file and use `git commit -F`. A multi-line body passed
through `-m` loses its formatting in PowerShell.

## When a gate blocks you

Either the change is wrong, or the gate's promise has to be restated on purpose.
Weakening a gate so your own change goes through is neither. If the promise really
has moved, change the gate in its own commit that says which promise changed and
why.

A gate you add has to be able to fail. Make it fail deliberately once, read the
message you wrote, then undo it — a gate whose failure nobody has seen is a gate
whose shape nobody knows.

## Snapshots, and the one rule about them

The reviewed files live under `v2/tools/testdata/`, each refreshed by a flag
passed after `-args`:

- `v2/tools/testdata/api_surface.txt` — `-update-api-snapshot`
- `v2/tools/testdata/package_paths.txt` — `-update-package-paths`
- `v2/tools/testdata/protocol_behaviour.txt` — `-update-protocol-behaviour`
- `v2/tools/testdata/generated_layout.txt` — `-update-generated-layout`
- `v2/tools/testdata/contract_snapshots/` — `-update-contract-snapshots`

The rule: **refreshing is the review.** Every one of them stores what it pins — the
declarations, the paths, the promises, the layout, the generated artefacts
themselves — so `git diff` on the file is the review, and the failure quotes the
line that moved. Nothing here is a digest any more, and nothing here should become
one. `make update-snapshots` refreshes them together; read the diff before you
commit it.

## A promise is a marker in the source

Behaviour the framework commits to is declared where it is implemented:

```go
// Stable: http.sse-headers — a stream answers 200 with text/event-stream, ...
// Covered by: TestSSEServer_WritesEventsAndHeaders
```

To add one: write the marker in non-test source next to the behaviour, name tests
that exist in that package, refresh `protocol_behaviour.txt`, and — if the promise
belongs to one of the contract surfaces `v2/internal/docs/RELEASE.md` enumerates —
name its gate there too, because a test counts them.

## Import rules the gates enforce

- `v2/endpoint` may import only the standard library. Contracts there are
  structural, not nominal.
- `v2/security/http` may import nothing from this module.
- Layering between the other packages is pinned by
  `TestArchitectureDependencyGates`.

## Where writing goes

Every document is a pair: `X.md` has `X_zh.md`. Record behaviour changes in
`v2/CHANGELOG.md`, plans and findings in `v2/internal/docs/ROADMAP.md`. Do not add
a new top-level markdown file for something those own, and do not write a
summary-of-work document nobody asked for.

## Opening a release candidate

`v2/RELEASE_MANIFEST.json` is the source of truth: phase `candidate`, the version,
and the tag. The version is repeated in these places, and
`TestReleaseManifestMatchesRepository` fails when one of them disagrees:

- `v2/RELEASE_MANIFEST.json` — `coreVersion` and `tag`
- `v2/Makefile` — `VERSION`
- `v2/cmd/microgen/internal/generator/options.go` — `defaultGoKitVersion`
- `v2/cmd/microgen/internal/generator/generator_test.go` — the generated `go.mod`
  expectation
- `v2/examples/go.mod` — the `require` on the framework
- `v2/README.md`, `v2/ARCHITECTURE.md`, `v2/internal/docs/RELEASE.md` and their
  `_zh` halves — the released-versus-candidate sentence
- `v2/CHANGELOG.md` and `v2/CHANGELOG_zh.md` — a new heading marked as candidate

## Cutting and recording a release

The procedure and what may not be worked around are in
`v2/internal/docs/RELEASE.md`. In short:

1. `make verify` green on the candidate commit, on Linux and on Windows.
2. `make release-check-clean` — the phase is `candidate` and the tag is absent.
3. `git tag -a vX.Y.Z -m "go-kit vX.Y.Z"` and push the tag.
4. `make verify-published` — resolves the version through the public proxy. Do not
   substitute `GOPROXY=direct`, a local `replace`, or the presence of a local tag:
   none of them show that the module was published.
5. Record it: manifest phase `released` with the date, the changelog heading dated,
   the milestone marked complete in `v2/internal/docs/ROADMAP.md`.

A change confined to `v2/tools` or to documentation gets no tag. The published
module would be byte-identical, and a version that says otherwise is a lie a
consumer pays for.

## Design stance

Anything a deployment would want to customise is a seam it implements, and a seam
left nil applies no framework policy. A wrong default is not excused by the
standard library having shipped it. `v2/ARCHITECTURE.md` has the rest.
