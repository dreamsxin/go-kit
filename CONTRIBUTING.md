# Contributing to go-kit

English | [简体中文](CONTRIBUTING_zh.md)

Thanks for looking at the internals. The framework is maintained with an
unusually strict set of machine-checked promises, and contributions land
faster once you know where the rails are. `AGENTS.md` (at the repository
root) is the full guide and is written to be readable by a tool or a human
in one sitting; this file is the short version for a first-time contributor.

## Before you write code

- Read the docs book chapter or package README your change touches. The doc
  comments in the tree carry the reasoning - a defect is usually already
  described next to the thing that has it.
- Check `v2/internal/docs/ROADMAP.md`. If your idea is recorded as a
  deliberate non-decision, the issue is not "nobody thought of it" but
  "here is why not" - say what changed.

## The change loop

1. Branch from `main`.
2. Change the smallest thing that makes the property true.
3. Verify from `v2/`: `make verify` runs everything CI runs (a committed
   tree is required). While iterating, `make test-runtime`,
   `make test-microgen`, `make test-contracts` are cheaper.
4. Record behavior changes in `v2/CHANGELOG.md` (English and Chinese are
   maintained as pairs), findings and deliberate non-decisions in
   `v2/internal/docs/ROADMAP.md`.
5. Open a pull request. The template asks for the property, the tests that
   fail without you, and the snapshot diff.

## Things the gates will hold you to

- **Formatting and vet.** `make verify` runs both across every maintained
  module.
- **Snapshots.** The exported API surface, package paths, protocol
  behaviors, generated layout and generated contracts are reviewed files.
  If your change moves one, refresh with `make update-snapshots` and read
  the diff - refreshing is the review.
- **Stable promises.** A behavior marked `// Stable:` in the source is a
  promise; it names the tests that hold it. Withdraw one and the release
  gate will ask why.
- **Documented surface.** Every exported declaration carries a doc comment;
  every document is a bilingual pair (`X.md` / `X_zh.md`).
- **Compatibility.** Before the freeze, deliberate breaks are allowed and
  must be listed in `deliberateBreaks` (`v2/tools/api_compatibility_test.go`)
  with a `!` in the commit subject. After the freeze they need a new major
  module.

## Commit messages

Conventional Commits with a `v2` scope (`fix(v2):`, `feat(v2):`, ...), `!`
for incompatible published changes, and a subject that states the property
rather than the mechanism. `AGENTS.md` has examples.

## Reviews

A maintainer reviews for the property being true, the smallest change that
makes it true, and the records being in place. Style disagreements are
resolved by the gates, not by taste - if the gates are green and the
property holds, the change is shippable.
