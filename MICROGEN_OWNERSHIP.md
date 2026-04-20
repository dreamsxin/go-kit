# microgen File Ownership Policy

This document defines the ownership boundary between generator-managed files and user-managed files in `microgen` generated projects.

It exists to protect one core rule:

- generated code and user-edited code must be separated

That rule matters for both normal generation and future extend or append workflows.

Use this document when deciding:

- whether a file may be regenerated
- whether an append operation may update a file
- where new generated aggregation files should live
- how to structure templates so users can customize projects safely

## Core Policy

`microgen` should not depend on overwriting user-edited files in order to evolve a generated project.

Instead, the generator should:

- create new files for new generated capability
- update only clearly generator-owned aggregation files
- leave user-owned implementation files untouched by default

In short:

- generator-owned code should stay together
- user-owned code should stay separate
- append mode should operate by adding generated slices, not by patching arbitrary handwritten logic

## Ownership Tiers

Every generated project file should fall into one of three tiers.

### Tier 1: Generator-Owned Rebuildable Files

These files are safe for regeneration or replacement.

Typical examples:

- generated SDK files
- generated demo client files
- generated skill files
- generated docs stubs
- generated model schema files such as `model/generated_<name>.go`
- generated repository files such as `repository/generated_<name>_repository.go` and `repository/generated_base.go`
- future generated registry files such as:
  - `cmd/generated_services.go`
  - `cmd/generated_routes.go`
  - `endpoint/<service>/generated_chain.go`

Rules:

- these files may be rewritten by generation
- they should contain clear generated headers
- users should not be required to customize these files for normal development

### Tier 2: Generator-Owned Aggregation Files

These files are also generator-managed, but they are more compatibility-sensitive because they shape startup and routing behavior.

Typical examples:

- generated service registration files
- generated route registration files
- generated runtime wiring files

Rules:

- updates must be narrow and deterministic
- append mode may update these files
- these files should be named and documented clearly so users know they are generator-owned
- generation should prefer these files over patching user-owned startup files

### Tier 3: User-Owned Files

These files are where users are expected to customize business behavior.

Typical examples:

- `service/<svc>/service.go`
- custom repository logic
- custom middleware composition files
- any user-created files outside clearly generated aggregation areas

Rules:

- generator should not overwrite these files by default
- append mode should fail rather than rewrite them implicitly
- if future replace behavior is ever added, it must be explicit and opt-in

## Separation Rules By Area

### `service/`

Purpose:

- user-owned business logic

Policy:

- generated project creation may create initial service implementation files
- later reruns or append flows should not rewrite those service implementation files casually
- new generated service additions should create new service files, not rewrite existing ones

### `endpoint/`

Purpose:

- mixed area, but should move toward clearer separation

Policy:

- user-owned endpoint customization should live outside generated aggregation files
- generated middleware composition should move into explicit files such as `endpoint/<service>/generated_chain.go`
- user-owned middleware customization should live in companion seams such as `endpoint/<service>/custom_chain.go`
- append mode should only update generator-owned endpoint aggregation files

### `transport/`

Purpose:

- generated protocol adaptation

Policy:

- per-service generated transport files may be created or updated for newly generated services
- if users need custom transport behavior, it should live in separate custom files rather than inside generator-owned transport outputs
- future extension logic should avoid patching arbitrary transport files if a generator-owned routing layer can be introduced instead

### `cmd/`

Purpose:

- startup and service registration

Policy:

- `cmd/main.go` should trend toward a thin user-stable bootstrap file
- generator-owned runtime assembly should move into generated aggregation files under `cmd/`
- append mode should update generator-owned `cmd/generated_*.go` files instead of rewriting `cmd/main.go` whenever possible
- user-specific HTTP routes should live in a separate user-owned seam such as `cmd/custom_routes.go`

### `config/`

Purpose:

- generated configuration contract plus user-edited values

Policy:

- generated config schema code should remain generator-owned
- generated default config files may be created by the generator
- user value changes inside config files must not be casually overwritten
- config schema and config values should be separated when possible

Recommended direction:

- keep generated config types in code files
- treat `config/config.yaml` as user-edited data after creation

### `model/` and `repository/`

Purpose:

- generated persistence scaffolding with explicit customization seams

Policy:

- generated model schemas should live in explicit generator-owned files such as `model/generated_<name>.go`
- user model hooks or custom behavior should live in separate files such as `model/<name>.go`
- generated repositories should live in explicit generator-owned files such as `repository/generated_<name>_repository.go`
- shared generated repository helpers should live in explicit generator-owned files such as `repository/generated_base.go`
- future append-model work should update those generator-owned files instead of rewriting user hook/custom files

## Extend Mode Rules

Extend mode must preserve ownership separation.

That means:

- append-service creates new generated files for the new service
- append-model creates new generated model and repository files only where safe
- append-middleware updates only generator-owned aggregation files
- protected user-owned files are not rewritten as part of append

If extend mode cannot complete without touching a protected file, it should fail clearly.

## Template Design Rules

Templates should be authored with file ownership in mind.

Preferred template outcomes:

- generated registry files are isolated
- generated comments clearly mark generator-owned files
- user customization points are not embedded in files the generator must keep rewriting

Bad pattern:

- one large file that mixes generated registration, generated scaffolding, and handwritten business logic

Good pattern:

- a thin user-facing file plus one or more generated aggregation files

## Naming Guidance

To make ownership obvious, generator-owned aggregation files should use explicit names.

Recommended names:

- `generated_services.go`
- `generated_routes.go`
- `generated_runtime.go`
- `generated_chain.go`
- `custom_chain.go`

User-owned customization files should use neutral or custom names and should not be rewritten by the generator.

## Compatibility Guidance

Ownership boundaries are part of the `microgen` product contract.

That means:

- changing a file from user-owned to generator-owned is compatibility-sensitive
- changing a file from generator-owned to user-owned may require migration guidance
- append mode should document exactly which files it may modify

## Definition Of Done

Ownership boundaries are in good shape when:

- users can tell which files are safe to edit
- generator updates do not depend on overwriting business logic files
- extend mode only mutates generator-owned files plus new generated files
- generated aggregation files act as the stable mutation points for future evolution
