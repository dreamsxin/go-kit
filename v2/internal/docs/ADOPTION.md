# Adoption

This file is the maintainer-side ledger for the parts of a framework's
reputation that are not code: where the project is listed, what its
submission materials say, and what cadence the releases keep. The guide
(`AGENTS.md`) says a summary-of-work document nobody asked for does not
belong in the tree — this one is the exception because the user-facing
promise of an adoption push is that the materials exist and stay current.

## Where the project stands

`dreamsxin/go-kit` was created 2026-08-27 and has no stars or forks yet.
The Go framework tier this project compares itself to (gin, fiber, echo,
kratos, go-zero, go-kit classic) sits at 20k-80k stars built over years of
community adoption. Stars are an outcome of adoption, not a goal to code
toward; the levers are content quality, discoverability, and a public
history of kept promises — all of which the gate system already produces
as a side effect and now needs to be shown, not just told.

## Channels and materials

- **awesome-go** — the primary list. Submission is a PR to
  `avelino/awesome-go` adding one line under Microservices:
  `- [go-kit](https://github.com/dreamsxin/go-kit) - A component-oriented Go service framework: service/endpoint/transport layering, endpoint middleware, HTTP and gRPC adapters, service discovery, MCP support, and a contract-driven generator.`

  Its acceptance bar is a documented, maintained, demonstrably useful
  repository: this repository's docs book, runnable examples, CI on two
  operating systems, and the reviewed-promise gates are that evidence. The
  PR description should link `docs/index.md`, `examples/README.md`, and the
  benchmark methodology (`docs/benchmarks.md`) rather than claim numbers.
- **r/golang and the Gophers Slack #go-projects** — a post leads with what
  is unusual here (machine-checked promises, W3C correlation by default,
  the generator), links the book, and invites critique. Honest framing:
  early project, API still pre-freeze, looking for users with opinions.
- **Go Blog / This Week in Go** — not applicable until there is adoption
  to report; revisit after the first outside contributor.

## Release cadence

Credibility compounds through a visible history of small, kept releases:

- minors ship as milestones complete, at least one a month while the
  roadmap has work — the pre-cadence history (five minors in sixteen days
  while the framework was being assembled) is not the pattern being
  promised; each release names its milestone and its deliberate
  non-decisions;
- patch releases only for defects a user could hit — doc-only and
  tools-only changes get no tag (the release guide's rule);
- the changelog stays user-facing: what changed, who pays, what to do;
- after two consecutive releases with no `!` commits, propose declaring
  the compatibility freeze — that is the moment a cautious adopter stops
  discounting the project.

## The rule this ledger must not break

Every claim on this page has to be something the repository shows, not
tells: a benchmark cites its harness, a promise cites its gate, a cadence
promise is kept or the page says it was not. Adoption material that
outruns the repository is the marketing the design stance forbids.
