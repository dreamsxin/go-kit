# Anti-Patterns

This document lists the implementation and design patterns that should be avoided in `go-kit`.

The goal is not to ban flexibility. The goal is to prevent changes that quietly erode framework clarity, compatibility, and long-term maintainability.

Use this document together with:

- [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
- [STABILITY.md](STABILITY.md)
- [PACKAGE_SURFACES.md](PACKAGE_SURFACES.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

## Category 1: Layering Anti-Patterns

These anti-patterns break the core service -> endpoint -> transport model.

### Business logic in transport

What it looks like:

- HTTP handlers performing domain decisions directly
- gRPC handlers embedding business workflows instead of delegating

Why it is harmful:

- makes transport-specific code own business behavior
- reduces testability
- makes protocol migration harder

Preferred pattern:

- business logic stays in `service`
- runtime policy stays in `endpoint`
- transport only maps requests and responses

### Skipping the endpoint layer for governance

What it looks like:

- putting timeout, logging, rate limiting, or circuit breaking directly into transport handlers

Why it is harmful:

- duplicates policy logic across protocols
- weakens the framework's central middleware model

Preferred pattern:

- apply governance behavior through endpoint middleware first

### Transport-aware service interfaces

What it looks like:

- service interfaces coupled to HTTP request types, gRPC metadata, or framework transport helpers

Why it is harmful:

- leaks protocol concerns into domain code
- makes generated and hand-written services less reusable

Preferred pattern:

- service methods should remain transport-agnostic

## Category 2: Public vs Internal Boundary Anti-Patterns

These anti-patterns confuse internal implementation detail with supported contract.

### Depending on generator internals from business code

What it looks like:

- importing or scripting against `cmd/microgen/generator`
- depending on parser or template internals from application code

Why it is harmful:

- locks users to implementation details that should remain free to evolve

Preferred pattern:

- depend on the `microgen` CLI and documented generated output only

### Treating templates as extension API

What it looks like:

- external workflows assuming specific `.tmpl` names or exact template decomposition

Why it is harmful:

- makes internal refactors feel like breaking changes

Preferred pattern:

- expose intentional CLI flags or documented variants instead of template-coupled workflows

### Treating examples as compatibility contract

What it looks like:

- copying example helper internals into production assumptions
- assuming example package layout is a stable API promise

Why it is harmful:

- examples exist to teach patterns, not to freeze incidental structure

Preferred pattern:

- rely on documented package APIs and explicit generated-project conventions

## Category 3: Extension Anti-Patterns

These anti-patterns add flexibility in ways that damage framework coherence.

### Core branching for one-off business needs

What it looks like:

- adding special-case behavior in framework runtime for a single application pattern

Why it is harmful:

- grows framework complexity without creating reusable value

Preferred pattern:

- first ask whether the need belongs in business code or an extension point

### Adding new abstractions before documenting extension points

What it looks like:

- introducing new global interfaces, flags, or packages when an existing hook would suffice

Why it is harmful:

- increases API surface faster than clarity

Preferred pattern:

- prefer extending documented middleware, transport hooks, balancer hooks, or generator options

### Expanding semi-stable packages as if they were frozen

What it looks like:

- assuming every helper in `endpoint/circuitbreaker`, `transport/grpc/*`, or `sd/endpointer/*` must now behave like a forever-stable platform API

Why it is harmful:

- blocks healthy package evolution

Preferred pattern:

- treat semi-stable packages as public but still evolving

## Category 4: microgen Anti-Patterns

These anti-patterns undermine generator compatibility.

### Changing generated top-level structure casually

What it looks like:

- moving generated business code out of `service/`
- collapsing `endpoint/` or `transport/` into different meanings

Why it is harmful:

- breaks onboarding, docs, scripts, and user expectations

Preferred pattern:

- treat generated layout meaning as a product contract

### Renaming documented flags without migration strategy

What it looks like:

- changing CLI flag names or semantics without preserving old behavior or documenting the change

Why it is harmful:

- breaks existing generation workflows immediately

Preferred pattern:

- preserve flags or add migration guidance and tests when changes are unavoidable

### Letting template cleanup change public behavior silently

What it looks like:

- calling a change "just refactoring templates" when generated structure or user-visible code meaning changes

Why it is harmful:

- hides breaking behavior inside an implementation-only framing

Preferred pattern:

- if users can feel the output change, treat it as a product change

## Category 5: Validation Anti-Patterns

These anti-patterns weaken confidence in framework evolution.

### Relying only on local package tests for public behavior changes

What it looks like:

- changing generator output but not running integration coverage
- changing docs or examples without running skill verification

Why it is harmful:

- misses regressions at the contract boundary

Preferred pattern:

- use the workflow-aligned test targets:
  - `make test-runtime`
  - `make test-microgen`
  - `make test-docs`
  - `make test-examples`
  - `go test -race ./...`

### Updating examples without checking framework guidance

What it looks like:

- examples drifting away from recommended framework patterns

Why it is harmful:

- examples become a source of accidental anti-patterns

Preferred pattern:

- keep examples aligned with documented architecture and recommended entry points

## Category 6: Product Scope Anti-Patterns

These anti-patterns pull the framework toward becoming an unfocused platform.

### Turning the framework into an enterprise platform core

What it looks like:

- adding config center responsibilities
- adding organization-specific auth policy orchestration
- adding release platform concerns into the core runtime

Why it is harmful:

- broadens scope beyond microservice construction and runtime governance
- weakens the framework's clarity

Preferred pattern:

- integrate with external platform concerns instead of absorbing them

### Solving business workflow problems in framework core

What it looks like:

- adding framework abstractions for application-specific orchestration logic

Why it is harmful:

- mixes product/domain logic into infrastructure code

Preferred pattern:

- keep workflow semantics in business services unless the pattern is truly cross-service and reusable

## Fast Review Checklist

When reviewing a new change, ask:

1. Does this move business logic into transport?
2. Does this bypass the endpoint layer for runtime governance?
3. Does this expose an internal implementation detail as if it were public API?
4. Does this make `microgen` output or flags less predictable for users?
5. Does this add platform scope that does not belong in a microservice framework?

If the answer to any of these is yes, pause and re-evaluate the design.

## Recommended Next Step

After this document, the next useful follow-up is to add small package-level stability notes or compatibility notes into package READMEs where the public surface is most important.
