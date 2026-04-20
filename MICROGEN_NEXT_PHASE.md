# microgen Next Phase Design

This document turns the next `microgen` roadmap into a concrete product and implementation design.

It focuses on two linked capabilities:

1. generated configuration with remote-config support
2. incremental extension of already-generated projects

Use this document when deciding:

- which new CLI surface to add
- how generated config should behave
- how append or extend mode should update existing projects safely
- which files are generator-owned versus user-owned
- which tests are required before shipping the next `microgen` milestone

For implementation-level detail, read:

- [MICROGEN_CONFIG_DESIGN.md](MICROGEN_CONFIG_DESIGN.md)
- [MICROGEN_EXTEND_DESIGN.md](MICROGEN_EXTEND_DESIGN.md)
- [MICROGEN_OWNERSHIP.md](MICROGEN_OWNERSHIP.md)

## Goals

The next phase should make `microgen` better in two ways:

1. every new generated project should have a clearer and more scalable configuration story
2. already-generated projects should be evolvable without destructive regeneration

In practice, that means:

- new projects should still run with local config by default
- remote config should be optional and additive
- existing generated projects should be extendable through explicit generator commands
- users should not have to choose between code generation and hand-maintainability

## Non-Goals

This phase does not aim to:

- turn `microgen` into a full application platform
- introduce a dynamic plugin system for generation
- guess user intent by patching arbitrary handwritten files
- make remote config mandatory for generated services
- replace the current full-generation flow for new projects

## Design Principles

- preserve the `service -> endpoint -> transport` layering contract
- prefer additive generated files over risky in-place rewrites
- treat append behavior as a public product contract
- keep the default generated project runnable with no external infrastructure
- separate internal generator refactors from user-visible behavior changes
- keep generator-owned files separate from user-owned implementation files

## Track 1: Generated Configuration

### Product Intent

Generated projects should include a `config/` layer that supports:

- local file configuration
- environment variable overrides
- optional remote configuration

The generated project should remain runnable with local config only.

### Generated Output Direction

The generated project should standardize around:

```text
config/
├─ config.go          # generated config model and loading entry point
├─ config.yaml        # local default config
├─ local.go           # file/env loading support
└─ remote.go          # optional remote-provider integration seam
```

Not every file has to exist in the first milestone, but the generated `config/` package should become the single startup-facing configuration contract.

### Config Model

The generated config model should include at least:

- service identity
- HTTP server settings
- gRPC server settings when enabled
- log settings
- database settings when model or db output is enabled
- remote config settings

Suggested shape:

```go
type Config struct {
	Service ServiceConfig `yaml:"service"`
	HTTP    HTTPConfig    `yaml:"http"`
	GRPC    GRPCConfig    `yaml:"grpc"`
	Log     LogConfig     `yaml:"log"`
	DB      DBConfig      `yaml:"db"`
	Remote  RemoteConfig  `yaml:"remote"`
}
```

The exact field names can still evolve, but the startup path should converge on one generated config root instead of ad hoc per-template wiring.

### Loading Model

The first stable abstraction should support three conceptual sources:

- `file`
- `env`
- `remote`

Recommended runtime behavior:

1. load local config defaults
2. apply env overrides
3. if remote config is enabled, attempt remote load
4. merge or override according to provider rules
5. continue with startup

Recommended safety policy:

- if remote config is disabled, startup should never depend on remote infrastructure
- if remote config is optional and unavailable, fallback to local config should be supported
- if users explicitly request strict remote-only behavior in the future, that should be opt-in

### Provider Abstraction

The generator should not hard-code a single remote provider into the whole generated project model.

Recommended interface direction:

```go
type Loader interface {
	Load(ctx context.Context, base Config) (Config, error)
}
```

Suggested generated loading chain:

- `LoadLocal(...)`
- `ApplyEnv(...)`
- `LoadRemote(...)`

The first provider should plug into the shared loading seam rather than redefining startup behavior.

### Remote Provider Strategy

Recommended rollout:

1. define the seam first
2. keep generated projects stable with local config only
3. add one initial remote provider
4. document how later providers would fit the same seam

The first provider should be chosen by practical usage, not by over-generalizing too early.

Good candidates:

- Nacos
- Consul
- Apollo
- etcd

The repo does not need all of them in the first milestone.

### CLI Direction For Config

Possible additive flags:

- `-config-mode file|hybrid|remote`
- `-remote-config`
- `-remote-provider nacos|consul|apollo|etcd`

Compatibility guidance:

- existing default generation should keep behaving like local-config generation
- new flags should not silently change current defaults
- if the flag set is too verbose, prefer one high-level mode plus one provider selector

Recommended first milestone:

- keep the current `-config` boolean meaning
- add only the minimal new flags needed for provider selection and optional remote loading

## Track 2: Incremental Extension Of Existing Projects

### Product Intent

`microgen` should support extending an already-generated project so users can add:

- new services
- new models
- new middleware composition

without treating full regeneration as the only safe path.

Critical policy:

- future extend support must not require overwriting user-edited business logic files
- new generated capability should land in new generated files or explicit generator-owned aggregation files

### Why This Needs An Explicit Mode

Incremental updates should not be hidden inside the normal full-generation path.

Reasons:

- users need to understand when the generator is creating a new project versus extending an existing one
- append behavior has different overwrite and ownership rules
- the generator should be able to fail clearly when a target project cannot be extended safely

### CLI Direction For Extension

Recommended product direction:

```bash
microgen extend -idl <file> -out <project>
```

Possible targeted extension flags:

- `-append-service <name>`
- `-append-model <name>`
- `-append-middleware <name[,name...]>`

Current status:

- `microgen extend -check -out <project>` is now implemented as a read-only compatibility scan for existing generated projects
- the first `append-service` path is now implemented behind `microgen extend`
- the current shipped contract is intentionally conservative:
  - `-check` reports current generator-owned seams and append readiness without changing files
  - Go IDL input only
  - full combined contract required
  - only generator-owned aggregation files and new generated files are mutated
  - existing user-owned service implementation files are preserved

Recommended rollout order:

1. explicit `extend` mode or equivalent
2. append-service
3. append-model
4. append-middleware

The exact syntax is open, but the CLI should communicate intentional extension, not hidden merge behavior.

### Required Project Scan

Before writing anything in extend mode, `microgen` should inspect the target project and identify:

- which services already exist
- which models already exist
- which transport packages already exist
- where startup wiring is aggregated
- whether the project still matches a supported generated layout

This scan should return a structured project state, not just ad hoc filesystem checks.

Suggested scan output:

```go
type ExistingProject struct {
	Root             string
	Services         []ExistingService
	Models           []ExistingModel
	AggregationFiles AggregationPoints
	Ownership        OwnershipMap
}
```

### File Ownership Model

Extend mode must classify files into three groups.

#### 1. Generator-owned and safe to regenerate

Examples:

- `skill/`
- generated SDK files
- generated demo client files
- generator metadata or registry files added specifically for controlled extension

#### 2. Generator-owned but compatibility-sensitive aggregation files

Examples:

- `cmd/main.go`
- generated service registration files
- generated route registration files
- generated middleware assembly files

These files may be updated, but only through narrow, testable rules.

#### 3. User-owned or user-likely-owned files

Examples:

- existing `service/<svc>/service.go` implementations after first generation
- repository logic users may have customized
- manually edited business logic and middleware chains

These files should be treated as protected by default.

### Aggregation Strategy

The safest path is to add new files and minimize edits to existing ones.

Recommended pattern:

- generate new per-service files under `service/`, `endpoint/`, `transport/`, `client/`, `sdk/`
- update only a small number of generated aggregation files

Good future candidates for controlled aggregation:

- `cmd/generated_services.go`
- `cmd/generated_routes.go`
- `endpoint/generated_middleware.go`

This keeps extend-mode writes concentrated in files that the generator clearly owns.

### Append-Service Strategy

This should be the first incremental-generation milestone.

Behavior:

- generate a new service subtree
- generate corresponding endpoint and transport files
- generate any related SDK/client additions
- update service registration in generated aggregation points
- avoid rewriting existing business-logic files

Why first:

- it exercises the broadest user-visible path
- it validates project scanning and aggregation rules
- it gives the highest product value early

### Append-Model Strategy

Append-model should come after append-service is stable.

Behavior:

- generate new model files without rewriting existing model files
- generate repository files for the new model
- update shared repository aggregation only through explicit generator-owned files if needed

Key constraint:

- users may have edited existing repository code, so model extension must avoid broad rewrites

### Append-Middleware Strategy

Middleware extension is the most likely to collide with user customization.

Recommended approach:

- introduce a dedicated generated middleware composition file
- append generated middleware wiring there
- keep user-defined custom middleware composition outside that generated file

This should be the last step in the extend roadmap, not the first.

## Required Generated File Boundaries

To support future extension safely, the generated project should make ownership clearer.

Recommended long-term direction:

- generator-owned aggregation files should be explicitly named as generated
- user-owned implementation files should remain readable and stable
- generated comments should help maintainers understand whether a file is safe to regenerate

Examples:

- `cmd/generated_services.go`
- `endpoint/generated_chain.go`
- `transport/generated_routes.go`

These are preferable to repeatedly patching user-maintained files with loose text replacements.

## Test Matrix

The next phase should add integration-heavy tests, not only template assertions.

### Config Tests

- local-config generated project builds and runs
- env overrides affect generated config loading correctly
- remote-config-enabled project still falls back safely when remote config is optional
- provider-enabled project uses the configured remote source when available

### Extend Tests

- generate a base project, then append a new service successfully
- generate a base project, hand-edit a protected file, then append and verify edits survive
- append-model adds new model artifacts without removing existing ones
- append-middleware updates only the intended generated aggregation file

### Mixed Lifecycle Tests

- generate project
- rerun generation where supported
- extend project
- verify startup, routes, skill output, SDK/client behavior, and build still work

### Compatibility Tests

- unsupported project shapes fail clearly in extend mode
- append commands fail clearly when target service or model already exists
- generator does not silently overwrite protected files

## Suggested Milestones

### Milestone 1: Stable generated config contract

- standardize generated `config/`
- keep local config runnable
- update startup templates

### Milestone 2: Remote-config seam

- add config source abstraction
- add minimal remote-config flags
- support one provider or a stubbed seam with tests

### Milestone 3: Existing-project scan and append-service

- implement extend mode
- scan current generated project layout
- append a service end to end

### Milestone 4: Append-model

- add incremental model/repository generation
- validate no destructive rewrites

### Milestone 5: Append-middleware

- introduce generated middleware aggregation
- support controlled middleware extension

## Decision Log To Resolve Before Coding

These decisions should be made explicitly before implementation starts.

### Config Track

- which remote provider should be first
- whether env overrides always apply before remote config
- whether remote config merges or replaces local values by default

### Extend Track

- whether `extend` should be a subcommand or additive flags on the existing CLI
- which files will become official generator-owned aggregation files
- what minimum project shape is required for extend mode to proceed

## Definition Of Done For This Phase

The next phase is in good shape when:

- generated projects have one consistent config story
- local config remains the default runnable path
- remote config is additive and documented
- extend mode can add at least one new service safely
- file ownership and overwrite rules are documented
- integration tests protect both config and extend workflows end to end
