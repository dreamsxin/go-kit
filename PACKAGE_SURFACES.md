# Package Surfaces

This document explains the intended usage surface of each major package family in `go-kit`.

Use it together with:

- [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
- [STABILITY.md](STABILITY.md)
- [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)

## How To Read This Document

Each package family is described from four angles:

- who should use it
- recommended entry points
- approved extension points
- details users should avoid depending on

## `kit`

Who should use it:

- teams that want the fastest path to a working service
- developers prototyping or building smaller services without full code generation

Recommended entry points:

- `kit.New`
- `kit.JSON`
- documented `With...` options such as timeout, logging, metrics, rate limit, circuit breaker, request ID, and gRPC enablement

Approved extension points:

- service-level options passed to `kit.New`
- handler registration through documented `Handle` patterns

Do not depend on:

- internal service wiring details not exposed through documented options
- assumptions about how `kit` composes runtime pieces under the hood

Role in the framework:

- `kit` is the convenience entry layer
- it should stay small, approachable, and biased toward default behavior

## `endpoint`

Who should use it:

- teams that need explicit runtime policy composition
- services that want business logic separated from transport concerns

Recommended entry points:

- `Endpoint`
- `Middleware`
- `NewBuilder`
- `NewTypedBuilder`
- `Chain`
- documented middleware such as timeout, metrics, logging, tracing, and error handling

Approved extension points:

- custom endpoint middleware
- builder-based composition
- typed endpoint wrapping and unwrapping

Do not depend on:

- undocumented middleware ordering side effects beyond the documented chain model
- internal helper structs that only exist to support tests or implementation details

Role in the framework:

- `endpoint` is the core runtime policy layer
- it is the preferred place to attach governance behavior

## `endpoint/circuitbreaker`

Who should use it:

- teams that need service-level resilience around endpoint execution

Recommended entry points:

- documented breaker adapters such as Gobreaker, HandyBreaker, and Hystrix support

Approved extension points:

- plugging circuit breaker middleware into endpoint chains
- introducing additional adapters through the same middleware shape

Do not depend on:

- exact option layouts remaining frozen forever
- third-party breaker implementation details leaking into business code

Role in the framework:

- resilience add-on under the main endpoint model
- useful, but still more evolvable than `endpoint` core

## `endpoint/ratelimit`

Who should use it:

- teams that need endpoint-local rate control

Recommended entry points:

- `NewErroringLimiter`
- `NewDelayingLimiter`

Approved extension points:

- composing custom limiters behind the same middleware contract

Do not depend on:

- policy-specific implementation details beyond the documented behavior

Role in the framework:

- focused governance helper package

## `transport/http/server`

Who should use it:

- services exposing HTTP APIs on top of endpoint or typed endpoint logic

Recommended entry points:

- public server constructors
- JSON-focused helpers
- documented options for hooks, error handling, and finalizers

Approved extension points:

- request decoding
- response encoding
- before/after/finalizer hooks
- custom error encoding

Do not depend on:

- internal writer interception details
- exact internal request lifecycle implementation beyond public hooks

Role in the framework:

- default HTTP ingress surface

## `transport/http/client`

Who should use it:

- services calling HTTP APIs through framework-style endpoint abstractions

Recommended entry points:

- public client constructors
- JSON client helpers
- documented options such as before/after/finalizer and client injection

Approved extension points:

- request encoders
- response decoders
- hook-based metadata injection and extraction
- custom HTTP client selection

Do not depend on:

- exact retry or buffering internals unless documented

Role in the framework:

- default HTTP egress surface

## `transport/grpc/server`

Who should use it:

- services exposing gRPC endpoints through the framework

Recommended entry points:

- documented public constructors and server options

Approved extension points:

- request/response mapping
- interceptors and transport hooks exposed through public options

Do not depend on:

- package internals that are not yet presented as part of the stable transport story

Role in the framework:

- gRPC ingress surface
- public, but somewhat more evolvable than the HTTP path

## `transport/grpc/client`

Who should use it:

- services making gRPC calls through framework-style abstractions

Recommended entry points:

- documented public constructors and client options

Approved extension points:

- encode/decode hooks
- request/response metadata handling through public options

Do not depend on:

- internal transport execution details

Role in the framework:

- gRPC egress surface

## `sd`

Who should use it:

- teams needing discovery-aware endpoint construction with retries, balancing, and invalidation

Recommended entry points:

- `sd.NewEndpoint`
- documented `With...` options for timeout, retries, and invalidation behavior

Approved extension points:

- instance factories
- discovery-backed endpoint creation
- documented resilience tuning options

Do not depend on:

- cache implementation details
- event propagation internals

Role in the framework:

- public service discovery orchestration surface

## `sd/endpointer`, `sd/endpointer/balancer`, `sd/endpointer/executor`

Who should use it:

- infra-oriented users that need more direct control over discovery-time endpoint composition

Recommended entry points:

- documented endpointer creation flows
- balancing and retry helpers where explicitly exposed

Approved extension points:

- balancer strategies
- executor retry behavior
- invalidation policy composition

Do not depend on:

- internal cache or event model details
- helper shapes that are not documented as package-level contracts

Role in the framework:

- lower-level extension area for discovery and invocation behavior

## `log`

Who should use it:

- services wanting a small logging helper aligned with the framework examples

Recommended entry points:

- documented logger constructors

Approved extension points:

- adapter-style use around the small public logger surface

Do not depend on:

- package growth into a full logging platform abstraction

Role in the framework:

- lightweight helper package, not a centerpiece

## `cmd/microgen`

Who should use it:

- teams building services from IDL, Proto, or existing database schema definitions

Recommended entry points:

- `microgen` CLI
- documented flags and generated project structure

Approved extension points:

- new documented flags
- supported generation modes
- template-backed output variants exposed intentionally through CLI options

Do not depend on:

- parser package internals
- generator package internals
- template directory structure
- file emission order unless explicitly documented

Role in the framework:

- public definition-driven generation product

## `cmd/microgen/parser`, `generator`, `dbschema`, `templates`

Who should use it:

- framework maintainers

Recommended entry points:

- none for normal business usage

Approved extension points:

- maintainer-only evolution inside the generator implementation

Do not depend on:

- any of these packages as stable public libraries

Role in the framework:

- internal implementation of `microgen`

## `examples`

Who should use it:

- business teams learning the framework
- maintainers validating intended usage patterns

Recommended entry points:

- `quickstart`
- `best_practice`
- `middleware`
- `sd`
- `microgen_skill`

Approved extension points:

- adding examples that demonstrate supported patterns

Do not depend on:

- examples as compatibility APIs
- helper code existing only for tutorial convenience

Role in the framework:

- documentation and behavior reference, not public API

## `tools`

Who should use it:

- framework maintainers

Recommended entry points:

- integration tests
- skill verification tests
- example smoke tests

Approved extension points:

- new validation harnesses that protect public contracts

Do not depend on:

- tools internals from business code

Role in the framework:

- internal validation layer

## Maintainer Rules Of Thumb

When a change touches a package surface:

1. Ask whether the package is public, semi-stable, or internal.
2. If public, update docs and examples with the code.
3. If semi-stable, prefer additive change and document behavior shifts.
4. If internal, preserve external behavior but keep implementation freedom.

## Suggested Next Step

The next useful follow-up after this document is a generated-project compatibility guide for `microgen`, so that project layout and output expectations are treated as explicit product contracts.
