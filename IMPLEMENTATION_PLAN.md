# Implementation Plan

This document turns the framework-boundary discussion into a concrete improvement roadmap for the repository.

## Goals

The implementation plan focuses on four outcomes:

1. Make framework responsibilities explicit.
2. Stabilize public-facing contracts.
3. Keep extensibility where it helps users.
4. Prevent framework sprawl into application-platform concerns.

## Phase 1: Clarify Product Boundaries

Objective:

- turn current architecture intent into an explicit repository contract

Actions:

- maintain `README.md` as the user-facing product overview
- maintain `FRAMEWORK_BOUNDARIES.md` as the boundary and responsibility source of truth
- maintain `PROJECT_WORKFLOW.md` as the repository development workflow source of truth
- link these documents together from the main README

Success criteria:

- new contributors can answer what the framework solves and does not solve without reading code
- user-facing and maintainer-facing docs no longer mix concerns

## Phase 2: Classify Public vs Internal APIs

Objective:

- reduce ambiguity about what users may safely depend on

Actions:

- label packages and surfaces as public, semi-stable, or internal in docs
- document stable extension points in `kit`, `endpoint`, `transport`, `sd`, and `microgen`
- identify internals that should not be treated as compatibility commitments
- avoid exposing generator internals as part of the public story

Suggested output:

- a short compatibility section per major package
- a small public-surface matrix in documentation

Success criteria:

- maintainers can evaluate compatibility impact before merging changes
- business teams rely on supported APIs instead of accidental implementation details

## Phase 3: Strengthen Extension Architecture

Objective:

- make approved customization paths obvious and safe

Actions:

- standardize documentation around middleware hooks, error encoders, balancers, retry strategies, and generator options
- audit extension points for consistency in naming and option patterns
- prefer additive extension APIs over special-case branching in core runtime code
- identify missing extension seams before teams work around them in application code

Success criteria:

- most customization requests can be answered with an existing extension path
- fewer changes require modifying framework core behavior

## Phase 4: Tighten Runtime Contracts

Objective:

- preserve the service -> endpoint -> transport model as the non-negotiable architectural spine

Actions:

- document anti-patterns, especially transport-coupled business logic
- add tests that protect separation boundaries where useful
- ensure examples consistently model the desired layering
- prevent drift where new features skip the endpoint layer or duplicate transport logic

Success criteria:

- new runtime features fit naturally into the existing layering model
- examples reinforce, rather than weaken, framework architecture

## Phase 5: Formalize `microgen` Compatibility

Objective:

- treat generated output shape as an external product contract

Actions:

- document which generated directories and files are intentional conventions
- define which CLI flags are considered stable
- add compatibility notes for template-driven output changes
- use integration tests to protect expected generated structure

Success criteria:

- template changes are evaluated as user-facing behavior changes
- users can upgrade with fewer surprises

## Phase 6: Establish A Long-Term Validation Matrix

Objective:

- keep validation aligned with the repository's actual component boundaries

Actions:

- keep the current layered workflow targets in `Makefile`
- preserve focused validation paths:
  - `make test-runtime`
  - `make test-microgen`
  - `make test-docs`
  - `make test-examples`
  - `make verify`
- continue using full `go test -race ./...` as release-level validation

Success criteria:

- contributors run the smallest sufficient test loop during development
- release confidence does not depend on ad hoc test selection

## Recommended Near-Term Work Items

These are the highest-value next steps for the repository.

### Work item 1: document package stability

- add a short stability note for `kit`, `endpoint`, `transport`, `sd`, and `cmd/microgen`
- identify which APIs are intended as public extension points

### Work item 2: define generated-project contract

- document expected generated layout and compatibility assumptions
- clarify which parts of generated output are safe for users to treat as conventions

### Work item 3: add anti-pattern guidance

- document what not to do:
  - business logic in transport
  - direct dependence on generator internals
  - private layout assumptions as public API

### Work item 4: improve contributor onboarding

- add a short "Where do I change this?" section for common tasks
- map runtime issues vs generator issues vs docs issues to the correct workflow lane

## Non-Goals For This Plan

This plan does not propose:

- building a full plugin platform first
- introducing a large internal package split immediately
- replacing current package layout wholesale
- expanding the framework into platform concerns outside microservice construction and governance

## Planning Heuristics

Use these heuristics when prioritizing future work:

- prioritize clarity before abstraction
- prefer stable extension points over bespoke feature flags
- treat generated output shape as a product decision
- avoid adding framework core for one-off business needs
- keep business logic transport-agnostic by default

## Suggested Execution Order

1. Publish boundary and responsibility docs.
2. Mark public vs internal surfaces.
3. Document extension points and anti-patterns.
4. Lock down `microgen` compatibility expectations.
5. Expand targeted tests only where a contract needs protection.

## Definition Of Done

This framework-boundary effort is in good shape when:

- maintainers share the same definition of framework scope
- business teams know what to use and what to avoid depending on
- extension points are explicit
- internal details are not mistaken for public API
- future roadmap discussions can use these documents as the default reference
