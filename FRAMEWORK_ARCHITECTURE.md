# Framework Architecture

This document defines the recommended target architecture for `go-kit`.

It is the bridge between:

- framework boundaries
- package surfaces
- generator behavior
- AI-facing capability exposure

Use this document when deciding:

- where code should live
- how layers should depend on each other
- how `microgen` should transform inputs into projects
- how service contracts should be exposed to both humans and AI agents

## Architecture Goals

`go-kit` should optimize for these properties together:

- clear service layering
- low-boilerplate service delivery
- explicit runtime governance
- definition-driven generation
- AI-ready service capability exposure
- generated output that remains readable and maintainable after generation

In short:

- business logic should remain stable even if transport or tooling changes
- generator output should feel like a clean starting codebase, not opaque codegen sludge
- AI-facing tool exposure should be derived from the same service contract as human-facing SDKs and transports

## Core Layering

The core dependency direction remains:

`service -> endpoint -> transport`

Meaning:

- `service` defines and implements business behavior
- `endpoint` wraps service methods in runtime policy
- `transport` adapts protocol requests into endpoint calls

Allowed dependency flow:

- `transport` may depend on `endpoint`
- `endpoint` may depend on `service` interfaces
- `service` should not depend on `endpoint` or `transport`

Strong rule:

- transport concerns must not leak into business logic
- business workflow logic must not be implemented in transport code
- endpoint policy composition must remain protocol-agnostic

## Recommended Repository Structure

Recommended framework structure:

```text
go-kit/
├─ kit/                     # convenience bootstrap layer
├─ endpoint/                # runtime middleware and endpoint composition
├─ transport/               # http/grpc protocol adaptation
├─ sd/                      # service discovery and client-side resilience
├─ log/                     # framework logging facade
├─ skill/                   # AI-facing schema/helpers/runtime support
├─ cmd/microgen/
│  ├─ parser/               # source parsers (Go IDL / Proto / DB schema)
│  ├─ generator/            # IR-to-project generation phases
│  ├─ templates/            # built-in templates
│  └─ ir/                   # recommended next target: shared intermediate model
├─ tools/                   # integration and generated-project validation
├─ examples/                # human-facing examples and smoke tests
├─ docs/                    # architecture and product documentation
└─ *.md                     # repo-level policy and architecture docs
```

Recommended generated service structure:

```text
generated-service/
├─ cmd/                     # service startup and wiring
├─ service/                 # business logic layer
├─ endpoint/                # policy composition around service methods
├─ transport/               # HTTP / gRPC protocol adapters
├─ client/                  # runnable demo client
├─ sdk/                     # production-facing client package
├─ skill/                   # AI tool / MCP exposure
├─ pb/                      # proto contracts and generated protobuf stubs
├─ model/                   # optional data models
├─ repository/              # optional data access layer
├─ config/                  # optional config scaffolding
├─ docs/                    # optional docs / swagger scaffolding
└─ idl.go                   # optional copied Go IDL input
```

## Package Roles

### `service/`

Purpose:

- pure domain operations
- business validation
- business orchestration
- domain-level errors

Should contain:

- service interfaces
- service implementations
- domain-friendly request/response types when not generated externally
- domain-level middleware when truly business-specific

Should not contain:

- HTTP handlers
- gRPC registration logic
- status-code selection
- transport header logic
- route binding

### `endpoint/`

Purpose:

- runtime governance around service method calls

Should contain:

- timeout
- logging
- metrics
- tracing hooks
- retry
- circuit breaking
- rate limiting
- endpoint builder/composition helpers

Should not contain:

- protocol decode/encode
- direct transport metadata parsing
- domain persistence logic

Design rule:

- endpoint should operate on typed request/response values or endpoint request/response envelopes
- endpoint middleware should be reusable across transports

### `transport/`

Purpose:

- translate external protocol contracts into endpoint calls

Should contain:

- HTTP route registration
- gRPC server/client glue
- request/response encoding
- metadata extraction and injection
- protocol-specific error encoding

Should not contain:

- business workflow branching
- data access
- cross-service orchestration logic

Design rule:

- transport owns protocol semantics
- endpoint owns execution semantics

### `kit/`

Purpose:

- fast-path bootstrap for common service setups

Should contain:

- simple service construction
- health wiring
- lifecycle wiring
- convenient middleware options

Should not replace:

- explicit endpoint/service/transport layering in production-generated services

### `skill/`

Purpose:

- AI-facing capability description and runtime support

Should contain:

- OpenAI Tool schema generation helpers
- MCP tool schema helpers
- skill endpoint helpers
- schema normalization helpers

Should not become:

- a second transport system
- a place to hide business logic

## Recommended `microgen` Architecture

`microgen` should evolve toward a two-step architecture:

1. source definition -> IR
2. IR -> generated artifacts

Recommended internal split:

### Parsers

Each source parser should only do:

- read source input
- validate shape
- translate into a shared intermediate representation

Supported sources:

- Go IDL
- Proto
- DB schema

### IR

Recommended next target: add a stable internal IR package under `cmd/microgen/ir/`.

Suggested IR shape:

```go
type ServiceDef struct {
	Name        string
	Description string
	Methods     []MethodDef
}

type MethodDef struct {
	Name        string
	Description string
	Input       MessageRef
	Output      MessageRef
	Errors      []ErrorDef
	Bindings    Bindings
}

type MessageDef struct {
	Name   string
	Fields []FieldDef
}

type FieldDef struct {
	Name        string
	Type        string
	Optional    bool
	Repeated    bool
	Description string
}
```

Why this matters:

- one contract can generate HTTP, gRPC, SDK, docs, skill, and tests consistently
- AI-facing output becomes contract-derived rather than template-derived
- parser complexity and generator complexity can evolve independently

### Generators

Generators should only do:

- phase orchestration
- artifact selection based on flags
- template execution from IR
- compatibility-safe project updates

Recommended phase model:

1. prepare project
2. generate model/data artifacts
3. generate service/endpoint/transport artifacts
4. generate final project artifacts
5. generate AI-facing artifacts
6. run compatibility-safe project updates

## Source Mapping Strategy

### Go IDL -> service skeleton

Best source for:

- framework-native generation
- AI-first service definitions
- strong alignment between human-readable interfaces and generated code

Use Go IDL as the richest contract source when:

- teams want readable service definitions in Go
- they want generation plus easy hand-maintenance

### Proto -> service skeleton

Best source for:

- gRPC-first systems
- cross-language contract ownership
- external API compatibility

Proto generation should:

- preserve proto service/message semantics
- generate transport glue that matches current protobuf plugin output
- generate HTTP adapters only when explicitly requested
- clearly distinguish between generated contract artifacts and generated runtime code

### DB Schema -> service skeleton

Best source for:

- CRUD bootstrap
- repository/model generation
- admin/internal services

DB schema generation should be treated as lower semantic richness than Go IDL or Proto.

It should focus on:

- models
- repositories
- CRUD-oriented service baselines

It should not pretend to fully infer business contracts.

## AI Tool / MCP Generation Strategy

AI-facing output should become a first-class generation target rather than a side artifact.

Recommended generation model:

- each service method can produce a tool definition
- tool schemas should derive from the same message definitions as SDK and transport code
- output format adapters should convert one internal skill model into:
  - OpenAI Tool schema
  - MCP tool schema
  - human-readable capability docs

Suggested internal skill model:

```go
type SkillDef struct {
	Name         string
	Description  string
	InputSchema  any
	OutputSchema any
	ErrorSchema  any
	Metadata     map[string]string
}
```

Important design rule:

- AI skill generation must not invent semantics not present in the underlying service contract

## Contract Design For Humans And AI Agents

To serve both humans and AI agents well, service definitions should prefer:

- explicit request and response structs
- descriptive field names
- stable error codes
- optional field semantics that preserve intent
- machine-readable descriptions where possible

Recommended service method style:

```go
type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

type CreateUserResponse struct {
	User  *User  `json:"user"`
	Error string `json:"error"`
}

type UserService interface {
	CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)
}
```

Avoid:

- primitive-only method signatures
- overloaded request shapes
- transport-specific field naming
- ambiguous error strings without stable codes

## Errors, Metadata, Context, Observability

These should converge on one shared model across HTTP, gRPC, SDK, and Skills.

### Errors

Recommended target:

- stable framework-level error shape
- transport mappings derived from error metadata

Suggested error capabilities:

- code
- message
- retryability
- human-readable detail
- structured metadata

Example direction:

```go
type CodedError interface {
	error
	Code() string
	Retryable() bool
	Metadata() map[string]string
}
```

### Metadata

Recommended rule:

- transport owns metadata carriers
- endpoint/service consume metadata abstractions

Important metadata examples:

- request id
- trace id
- auth subject
- response headers
- trailers

### Context

Recommended rule:

- `context.Context` remains the control plane carrier
- business data should not be hidden in context unless it is request-scoped metadata

### Observability

Recommended split:

- transport: protocol-level metadata, status, size, headers
- endpoint: latency, retries, failures, circuit/rate-limit events
- service: business success/failure signals

## Testing Strategy

Recommended testing pyramid for `go-kit`:

### Package tests

Use for:

- middleware behavior
- constructor contracts
- helper logic
- parser behavior
- generator helper logic

### Generated artifact tests

Use for:

- output paths
- generated layout
- documented compatibility conventions

### Runnable generated-project tests

Use for:

- generated service startup
- health endpoints
- skill endpoints
- client/sdk usability

### Component assembly probes

Use for:

- generated `service + endpoint + transport + log` assembly
- validating internal component cooperation without full runtime complexity

### Toolchain-gated tests

Use for:

- protobuf plugin compatibility
- environment-specific codegen paths

Key rule:

- skip explicitly when external tools are missing
- never silently weaken what the test is supposed to prove

## Scaffolding Strategy

Generated code should optimize for:

- readability
- maintainability
- explicit extension points
- compatibility-safe regeneration

Good scaffolding properties:

- clear TODO placement
- obvious ownership boundaries
- easy-to-replace internals
- generated code that looks like hand-written starter code

Bad scaffolding properties:

- hidden magic
- generated files with no safe modification story
- runtime assumptions only visible inside templates

## Plugin And Extension Strategy

Recommended order of extensibility:

1. IR transforms
2. generation phase hooks
3. output format plugins
4. template variants

Do not start by exposing generator internals as a free-for-all template API.

Better extension surfaces:

- add a new output artifact from IR
- add a phase hook after service generation
- add a skill-schema formatter
- add an alternative project layout profile

This keeps the layering model stable while still allowing customization.

## Recommended Next Architecture Tasks

The highest-value next steps are:

1. Add an internal `microgen` IR package and migrate parsers toward it.
2. Define a stable framework error/metadata contract shared by HTTP, gRPC, SDK, and Skill output.
3. Document package-level layering rules in generator-facing terms so codegen can enforce them.
4. Turn AI Skill generation into a first-class IR output instead of a mostly template-driven extra.
5. Refactor `tools/integration_test.go` helper structure so the growing component/runnable probes remain maintainable.

