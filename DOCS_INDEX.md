# Documentation Index

This file is the fastest map of the repository's Markdown documentation.

Use it when you know you need "the right doc" but do not want to hunt through the repo first.

## Start Here

- New to `go-kit` as a user:
  Read [README.md](README.md)
- Resuming work on the repository or an AI coding session:
  Read [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md), then [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
- Working on the repository itself:
  Read [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md)

## By Goal

### Understand The Product

- [README.md](README.md)
  Product overview, quick start, architecture summary, `microgen`, skills, generated project layout.
- [examples/README.md](examples/README.md)
  Example programs and learning path.

### Resume Current Work Quickly

- [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md)
  Shortest maintainer/AI-agent entry point.
- [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
  Current repository state, recent changes, validation history, next recommended tasks.
- [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)
  Higher-level roadmap and sequencing.

### Work On The Repository Safely

- [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)
  Validation lanes, recommended commands, release/regression workflow.
- [PR_CHECKLIST.md](PR_CHECKLIST.md)
  Review and merge checklist for scope, layering, compatibility, docs, and validation.

### Understand Framework Scope And Stability

- [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
  What belongs in the framework and what should stay outside it.
- [ANTI_PATTERNS.md](ANTI_PATTERNS.md)
  Design and implementation patterns to avoid.
- [STABILITY.md](STABILITY.md)
  Stable, semi-stable, and internal surface expectations.
- [PACKAGE_SURFACES.md](PACKAGE_SURFACES.md)
  Package-level public/internal contract guidance.

### Understand Target Architecture

- [FRAMEWORK_ARCHITECTURE.md](FRAMEWORK_ARCHITECTURE.md)
  Target architecture for runtime packages, generated project layout, IR direction, AI skill generation, and shared cross-cutting guidance.

### Work On `microgen`

- [MICROGEN_INDEX.md](MICROGEN_INDEX.md)
  Shortest entry point for `microgen` docs by question and task.
- [MICROGEN_DESIGN.md](MICROGEN_DESIGN.md)
  Product-level direction for generated config, extend mode, and file ownership.
- [MICROGEN_CONFIG_DESIGN.md](MICROGEN_CONFIG_DESIGN.md)
  Implementation-level design for generated config and remote config.
- [MICROGEN_EXTEND_DESIGN.md](MICROGEN_EXTEND_DESIGN.md)
  Implementation-level design for extend mode and append operations.
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)
  Current compatibility guarantees for generated output and rerun behavior.

### Learn Specific Runtime Areas

- [endpoint/README.md](endpoint/README.md)
  Endpoint concepts, composition, and middleware layer behavior.
- [transport/README.md](transport/README.md)
  HTTP/gRPC transport hook semantics and transport-level expectations.
- [sd/README.md](sd/README.md)
  Service discovery overview.
- [sd/consul/README.md](sd/consul/README.md)
  Consul-specific service discovery support.
- [sd/events/README.md](sd/events/README.md)
  Events helpers used by service discovery components.
- [sd/endpointer/README.md](sd/endpointer/README.md)
  Endpointer helpers and composition behavior.

### Work On Tools And Skills

- [tools/README.md](tools/README.md)
  Tooling overview and test helpers.
- [tools/SKILL.md](tools/SKILL.md)
  Skill-specific guidance and verification target.

## Recommended Reading Paths

### For A New Maintainer

1. [README.md](README.md)
2. [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md)
3. [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
4. [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)
5. [FRAMEWORK_ARCHITECTURE.md](FRAMEWORK_ARCHITECTURE.md)

### For A `microgen` Change

1. [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md)
2. [MICROGEN_INDEX.md](MICROGEN_INDEX.md)
3. [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)
4. [MICROGEN_DESIGN.md](MICROGEN_DESIGN.md)
5. The relevant design doc:
   [MICROGEN_CONFIG_DESIGN.md](MICROGEN_CONFIG_DESIGN.md) or [MICROGEN_EXTEND_DESIGN.md](MICROGEN_EXTEND_DESIGN.md)

### For A Runtime / Framework Change

1. [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
2. [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)
3. [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
4. [STABILITY.md](STABILITY.md)
5. [PACKAGE_SURFACES.md](PACKAGE_SURFACES.md)

### For Release Or Review Work

1. [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)
2. [PR_CHECKLIST.md](PR_CHECKLIST.md)
3. [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)

## Maintenance Note

When adding a new top-level design, policy, or workflow document, update this index in the same change so the documentation remains navigable.
