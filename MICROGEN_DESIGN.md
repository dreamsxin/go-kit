# microgen Design Guide

Purpose:
- Define the current product direction and ownership model for `microgen`.

Read this when:
- You are deciding what `microgen` should do next.
- You need the product-level rules behind generated config, extend mode, or file ownership.

See also:
- [MICROGEN_INDEX.md](MICROGEN_INDEX.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)
- [MICROGEN_CONFIG_DESIGN.md](MICROGEN_CONFIG_DESIGN.md)
- [MICROGEN_EXTEND_DESIGN.md](MICROGEN_EXTEND_DESIGN.md)

This is the consolidated product/design entry point for `microgen`.

It replaces the older split between:

- next-phase roadmap notes
- file ownership policy notes

Use this file for the stable "why" and "what". Use the config and extend design docs for implementation detail.

## Current Product Direction

`microgen` is currently moving in two linked directions:

1. generated projects should have one consistent configuration story
2. already-generated projects should be extendable without destructive regeneration

That means:

- local config remains the default runnable path
- remote config stays optional and additive
- extend behavior is explicit and documented
- generated files and user-owned files stay structurally separate

## Current Status

The design is no longer purely roadmap-level.

Current shipped direction already includes:

- generated `config/` output with defaults, local loading, env overrides, and a remote-loading seam
- explicit config CLI surface through `-config-mode file|hybrid|remote` and `-remote-provider`
- a first real provider-backed path for `provider: consul`
- explicit extend mode through `microgen extend`
- supported extend operations for:
  - `-check`
  - `-append-service`
  - `-append-model`
  - `-append-middleware`

So the next design task is not to invent these capabilities from scratch. It is to keep their contract clear, compatibility-safe, and maintainable.

## Goals

The current `microgen` design should keep pushing toward:

- one startup-facing generated config contract
- additive remote config rather than mandatory remote infrastructure
- safe extension of generated projects through explicit generator commands
- clear ownership boundaries between generator-managed code and user-managed code
- integration-tested user-visible behavior rather than template-only confidence

## Non-Goals

`microgen` is not trying to:

- become a full application platform
- infer arbitrary handwritten intent by patching unknown files
- make remote config mandatory
- replace normal full generation for new projects
- turn extend mode into a best-effort merge engine

## Design Principles

- preserve the `service -> endpoint -> transport` layering contract
- prefer additive generated files over risky in-place rewrites
- treat generated output shape as a product contract
- keep the default generated project runnable with local config only
- separate product-level rules from implementation-level helper structure
- make generator ownership obvious in both naming and file layout

## Track 1: Generated Configuration

### Product Intent

Generated projects should include a consistent `config/` layer that supports:

- local file configuration
- environment variable overrides
- optional remote configuration

The default generated project should still be runnable without remote infrastructure.

### Current Direction

The configuration contract now centers on generated `config/` output and an explicit loading flow:

1. start from defaults
2. load local YAML
3. apply env overrides
4. optionally load remote config

The current public direction already includes:

- `-config-mode file|hybrid|remote`
- `-remote-provider`
- fallback-aware remote behavior for hybrid mode
- strict remote behavior for remote mode

### What Should Happen Next

The next config work should focus on tightening the shipped contract, especially:

- provider validation
- strict remote-failure behavior
- generated output clarity
- keeping default local startup simple

Implementation details for this track live in [MICROGEN_CONFIG_DESIGN.md](MICROGEN_CONFIG_DESIGN.md).

## Track 2: Incremental Extension

### Product Intent

`microgen` should support extending an existing generated project so users can add:

- services
- models
- middleware composition

without forcing a destructive full regeneration flow.

### Current Direction

Extend mode is now an explicit workflow:

```bash
microgen extend -out <project> ...
```

The current supported extend surface is intentionally conservative:

- `-check` is read-only
- append flows currently require Go IDL input
- append flows use full combined contracts rather than delta-only patching
- extend writes should stay concentrated in generator-owned files plus new generated files

### What Should Happen Next

The next extend work should focus on:

- keeping compatibility guidance clear
- preserving ownership boundaries
- improving failure reporting where the supported project shape is missing
- avoiding drift between generated aggregation seams and their documented meaning

Implementation details for this track live in [MICROGEN_EXTEND_DESIGN.md](MICROGEN_EXTEND_DESIGN.md).

## Ownership Model

Ownership boundaries are part of the `microgen` product contract.

The core rule is:

- generated code and user-edited code must stay separated

`microgen` should evolve generated projects by:

- creating new files for new generated capability
- updating only clearly generator-owned aggregation files
- leaving user-owned implementation files untouched by default

### Ownership Tiers

Every generated project file should conceptually fall into one of three tiers.

#### Tier 1: Generator-Owned Rebuildable Files

Typical examples:

- generated SDK files
- generated demo client files
- generated skill files
- generated docs stubs
- `model/generated_<name>.go`
- `repository/generated_<name>_repository.go`
- `repository/generated_base.go`

Rules:

- these files may be regenerated
- they should be clearly marked as generated
- users should not have to customize them during normal development

#### Tier 2: Generator-Owned Aggregation Files

Typical examples:

- `cmd/generated_services.go`
- `cmd/generated_routes.go`
- `cmd/generated_runtime.go`
- `endpoint/<service>/generated_chain.go`

Rules:

- these files are generator-managed
- updates must be narrow and deterministic
- append mode may update them
- they should be the preferred mutation points instead of patching user-owned files

#### Tier 3: User-Owned Files

Typical examples:

- `service/<svc>/service.go`
- custom repository logic
- user middleware composition files such as `endpoint/<service>/custom_chain.go`
- user route additions such as `cmd/custom_routes.go`

Rules:

- generator should not overwrite these files by default
- extend mode should fail rather than implicitly rewrite them
- any future replace behavior must be explicit and opt-in

## Separation Rules By Area

### `service/`

- Treat service implementation files as user-owned after initial generation.
- Extend flows should create new service files for new services, not rewrite existing implementations.

### `endpoint/`

- Keep generated middleware composition in explicit generator-owned files such as `generated_chain.go`.
- Keep user middleware customization in companion user-owned seams such as `custom_chain.go`.

### `transport/`

- Generated transport files may be created for new services.
- Custom transport behavior should live in separate user-owned files rather than inside generator-owned outputs.

### `cmd/`

- Keep `cmd/main.go` thin and readable.
- Prefer `cmd/generated_*.go` files as generator-owned mutation points.
- Keep user-specific route customization in separate seams such as `cmd/custom_routes.go`.

### `config/`

- Generated config code stays generator-owned.
- `config/config.yaml` should be treated as user-edited data after creation.
- Config schema and user values should stay separated when possible.

### `model/` and `repository/`

- Generated model and repository scaffolding should stay in explicit generated files.
- User hooks and custom behavior should stay in separate user-owned files.

## Recommended Reading Paths

### Product Direction

1. [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
2. [MICROGEN_DESIGN.md](MICROGEN_DESIGN.md)
3. [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

### Config Work

1. [MICROGEN_DESIGN.md](MICROGEN_DESIGN.md)
2. [MICROGEN_CONFIG_DESIGN.md](MICROGEN_CONFIG_DESIGN.md)
3. [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

### Extend Work

1. [MICROGEN_DESIGN.md](MICROGEN_DESIGN.md)
2. [MICROGEN_EXTEND_DESIGN.md](MICROGEN_EXTEND_DESIGN.md)
3. [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

## Definition Of Done

The design is in good shape when:

- generated projects have one consistent config story
- local config remains the default runnable path
- remote config is additive and documented
- extend mode remains explicit and compatibility-safe
- users can tell which files are safe to edit
- integration tests protect the user-visible config and extend workflows
