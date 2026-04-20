# Stability Guide

Purpose:
- Classify `go-kit` surfaces by stability so maintainers know which changes are compatibility-sensitive.

Read this when:
- You are changing public behavior, generated output, or package APIs and need to judge release risk.

See also:
- [PACKAGE_SURFACES.md](PACKAGE_SURFACES.md)
- [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

This document classifies the main `go-kit` surfaces into stability tiers so maintainers and business teams can distinguish supported contracts from implementation details.

## Stability Tiers

### Stable

Meaning:

- safe to document as the default user-facing contract
- changes should be treated as compatibility-sensitive
- behavioral changes should be reflected in examples, docs, and tests

### Semi-stable

Meaning:

- intended for use, but still evolving
- compatible changes are preferred
- maintainers should call out notable changes in release notes or migration notes

### Internal

Meaning:

- not intended as a long-term dependency surface
- can change when implementation needs change
- users should avoid depending on these details directly

## Package-Level Classification

| Area | Stability | Notes |
|------|-----------|-------|
| `kit` | Stable | Primary low-boilerplate entry point for service bootstrap |
| `endpoint` core middleware model | Stable | Central runtime composition contract |
| `endpoint/circuitbreaker` | Semi-stable | Publicly useful, but option shapes may still evolve |
| `endpoint/ratelimit` | Semi-stable | Publicly useful, but policy detail may still evolve |
| `transport/http/server` | Stable | Public transport constructor and hook surface |
| `transport/http/client` | Stable | Public client constructor and hook surface |
| `transport/grpc/server` | Semi-stable | Public, but less simplified than HTTP path |
| `transport/grpc/client` | Semi-stable | Public, but should be treated as evolving |
| `sd` | Stable | Public service discovery and endpoint wiring surface |
| `sd/endpointer` | Semi-stable | Publicly useful but still closer to infra-level composition |
| `sd/endpointer/balancer` | Semi-stable | Extension-oriented package, allowed to grow |
| `sd/endpointer/executor` | Semi-stable | Retry and execution policies are public but evolving |
| `log` | Stable | Small public helper surface |
| `utils` | Internal | Utility helpers are not a core user-facing product surface |
| `cmd/microgen` CLI | Stable | User-facing generator contract |
| `cmd/microgen/generator` | Internal | Implementation of the CLI, not a direct user contract |
| `cmd/microgen/parser` | Internal | Parsing internals |
| `cmd/microgen/dbschema` | Internal | Schema introspection internals |
| `cmd/microgen/templates` | Internal | Template layout is not a public API |
| `examples` | Stable as documentation, not API | Meant to teach supported usage patterns |
| `tools` | Internal | Validation harness, not business-facing API |

## Stable Public Contract

These are the parts users should be encouraged to rely on:

- `kit.New`, `kit.JSON`, and documented service bootstrap flows
- the service -> endpoint -> transport layering model
- endpoint middleware composition as the standard runtime policy mechanism
- documented HTTP server/client constructors and hooks
- documented gRPC transport constructors and hooks
- documented `sd` endpoint creation and resilience options
- the `microgen` CLI entry point and documented flags
- the generated `/skill` endpoint behavior once documented in public docs

## Semi-Stable Contract

These are legitimate extension surfaces, but maintainers should preserve room to refine them:

- specialized middleware packages under `endpoint/`
- finer-grained gRPC transport behavior
- balancer and retry policy helper packages
- advanced generated project options that may still be expanded

For these surfaces:

- additive evolution is preferred
- breaking changes should be deliberate and documented

## Internal Contract

The following should not be treated as compatibility promises:

- generator template file names and internal composition
- parser helper types and parsing pipeline structure
- introspection helpers in `dbschema`
- exact example package layout used for tests
- tools-based harness internals
- utility helper implementations that are not described as supported public APIs

## Extension Surface Matrix

| Surface | Stability | Allowed usage |
|---------|-----------|---------------|
| endpoint middleware | Stable | Recommended extension surface |
| endpoint builder options | Stable | Recommended extension surface |
| HTTP before/after/finalizer hooks | Stable | Recommended extension surface |
| gRPC hooks/interceptors used through public constructors | Semi-stable | Allowed, but still evolving |
| custom error encoders | Stable | Recommended extension surface |
| service discovery providers | Semi-stable | Allowed extension surface |
| balancer strategies | Semi-stable | Allowed extension surface |
| retry policies | Semi-stable | Allowed extension surface |
| `microgen` CLI flags | Stable when documented | Compatibility-sensitive |
| `microgen` templates | Internal | Not a supported direct dependency surface |

## Compatibility Rules

When changing a stable surface:

1. Update docs and examples.
2. Update workflow or validation docs if developer behavior changes.
3. Add or adjust tests that protect the intended contract.
4. Treat generated output changes as product behavior changes if the change is user-visible.

When changing a semi-stable surface:

1. Prefer additive changes.
2. Avoid needless renames or behavior shifts.
3. Document meaningful changes if users may feel them.

When changing an internal surface:

1. Keep external behavior stable where possible.
2. Do not promote the internal detail to public contract accidentally through docs or examples.

## What Business Teams Should Depend On

Business teams should depend on:

- documented package entry points
- documented generator flags and generated layout conventions
- examples that are clearly presented as recommended usage

Business teams should avoid depending on:

- template internals
- parser internals
- helper code that appears only in tests or examples
- implementation-specific file generation order

## Recommended Next Step

Use this file together with:

- [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
- [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)
- [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)

Together, these documents define scope, stability, workflow, and execution order for future framework changes.
