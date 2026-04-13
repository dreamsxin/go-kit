# Framework Boundaries

This document defines what `go-kit` is responsible for, what it intentionally does not solve, which capabilities it exposes to business teams, and where customization is allowed.

## Framework Mission

`go-kit` is a Go microservice framework focused on:

- clear service layering
- low-boilerplate service construction
- reusable runtime governance
- definition-driven code generation
- AI-ready capability exposure

In short:

- `go-kit` helps teams build and evolve Go microservices consistently.
- `go-kit` does not try to be a full enterprise platform.

## Problems The Framework Solves

The framework is designed to solve these recurring engineering problems:

- Service codebases mixing business logic with HTTP or gRPC details
- Repeated hand-written boilerplate for handlers, endpoints, middleware wiring, and startup code
- Inconsistent middleware usage across services
- Lack of standard service governance primitives such as timeout, rate limiting, circuit breaking, retry, and service discovery
- High cost of keeping generated clients, service contracts, and service scaffolding aligned
- Lack of a standard way to expose service capabilities to AI agents

## Problems The Framework Does Not Solve

The framework intentionally does not own these concerns:

- Business domain modeling
- Product workflow orchestration
- Organization-wide auth, IAM, or tenant policy systems
- CI/CD pipeline design
- Database migration lifecycle management
- Event bus, job scheduler, or stream processing platforms
- Frontend delivery
- Full application platform concerns such as config center, secret management, release control, or service mesh replacement

Those concerns may integrate with `go-kit`, but they should not become framework core responsibilities.

## Component Responsibilities

The repository breaks down into several component families.

### `kit/`

Purpose:

- fast service bootstrap for common HTTP and gRPC scenarios
- minimal-ceremony developer entry point

Should own:

- simple service creation
- default health and runtime wiring
- common convenience APIs

Should not own:

- deep business abstractions
- generator internals
- advanced service discovery orchestration

### `endpoint/`

Purpose:

- runtime middleware composition around business operations

Should own:

- timeout
- metrics
- logging
- tracing
- backpressure
- circuit breaking
- rate limiting
- endpoint builder patterns

Should not own:

- protocol-specific decoding or encoding
- direct database access
- business workflow logic

### `transport/`

Purpose:

- protocol adaptation between external requests and endpoint invocation

Should own:

- HTTP and gRPC request/response mapping
- encode/decode behavior
- transport hooks
- error encoding

Should not own:

- business validation rules beyond transport-level concerns
- domain behavior
- cross-service policy orchestration

### `sd/`

Purpose:

- service discovery and invocation-side resilience

Should own:

- instance management
- cache and invalidation
- balancer strategies
- retry execution
- endpoint factory wiring

Should not own:

- business failover semantics
- protocol marshaling
- platform-wide control plane concerns

### `cmd/microgen/`

Purpose:

- generate consistent service scaffolding from IDL, Proto, or DB schema inputs

Should own:

- parsing source definitions
- generating service/endpoint/transport/sdk/config/project skeletons
- preserving framework conventions in generated output

Should not own:

- custom per-team workflow semantics
- one-off app-specific architecture opinions
- runtime feature branching that belongs in framework packages

### `tools/`, `examples/`, docs

Purpose:

- validation, onboarding, and behavior demonstration

Should own:

- example coverage
- documentation-backed tests
- integration verification

Should not own:

- production runtime behavior
- public API definitions

## Business-Facing Capabilities

Business teams should primarily consume these capabilities:

- rapid service bootstrap via `kit`
- transport-agnostic business logic via service interfaces
- middleware composition via `endpoint`
- HTTP and gRPC integration via `transport`
- client-side resilience and discovery via `sd`
- code generation via `microgen`
- generated SDKs for service-to-service use
- AI skill exposure via generated `/skill` endpoints

Business-facing promise:

- define the contract
- implement business logic
- compose standard runtime policies
- let the framework handle the repetitive wiring

## Internal Implementation Details

The following should be treated as internal details unless explicitly elevated to public contract:

- generator template file layout
- parser AST handling details
- generated file emission order
- cache storage internals
- specific test harness wiring in `tools/`
- example package layout used only for demonstration
- helper types introduced only to support internal codegen or tests

If business teams depend on these details directly, upgrade flexibility will shrink quickly.

## Allowed Extension Points

These are good extension surfaces and should remain intentionally customizable:

- endpoint middleware
- endpoint builder options
- HTTP and gRPC before/after/finalizer hooks
- error encoders and response encoders
- service discovery providers
- balancer strategies
- retry policies
- generator options and template variants
- skill output formats and metadata enrichment
- config loading integration points

Guiding rule:

- allow extension at boundaries
- avoid extension that changes the framework's layering model

## Customization That Should Be Restricted

These areas should not be freely customized because they define framework identity:

- the service -> endpoint -> transport separation
- the contract that business logic remains transport-agnostic
- generator assumptions about project layering
- middleware execution model at the endpoint layer
- public structure of the runtime governance APIs once stabilized

In practice, this means:

- do not let services bypass endpoint middleware by embedding policy logic in transport code
- do not let business packages depend on generator internals
- do not treat generated layout changes as harmless refactors

## Public Contract vs Internal Contract

The framework should gradually classify APIs into two groups.

### Public contract

These are the APIs and conventions users should be able to rely on:

- `kit` entry points
- stable middleware extension points
- `transport/http` and `transport/grpc` public constructors and hooks
- `sd` public options and endpoint creation APIs
- `microgen` CLI surface and documented output conventions
- `/skill` exposure behavior once documented

### Internal contract

These may change more freely:

- codegen template internals
- parser and introspection helpers
- test scaffolding
- non-documented helper types

## Decision Rules For Future Changes

When evaluating a new feature, use these questions:

1. Does it strengthen service layering, runtime governance, or definition-driven development?
2. Is it reusable across many services instead of one application?
3. Does it belong at framework boundary level rather than business logic level?
4. Can it be added as an extension point instead of a new core abstraction?
5. Will it force business teams to understand internal generator or transport details?

If the answer to 4 is yes, prefer extension over framework core growth.
If the answer to 5 is yes, the design likely needs to be simplified.

## Target Product Definition

The most accurate description of `go-kit` is:

- a Go microservice layering framework
- plus a definition-driven code generator
- plus runtime governance primitives
- plus AI-facing capability exposure

It should not drift into being an all-in-one enterprise platform kernel.
