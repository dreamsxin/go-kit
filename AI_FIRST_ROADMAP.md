# AI-First Framework Roadmap

Purpose:
- Make `go-kit` easier for humans and AI agents to understand, generate, extend, and verify without weakening the existing `service -> endpoint -> transport` architecture.

## Direction

The framework should stay layered. The AI-first work should improve the product contract around generation, extension, and verification rather than replacing the runtime architecture.

Primary path:

1. Define the service contract.
2. Generate a runnable project with `microgen`.
3. Edit user-owned business files.
4. Inspect generated routes and skill metadata.
5. Extend the project through explicit `microgen extend` commands.
6. Verify with the smallest relevant test loop.

## Phase 1: Generated Project Orientation

Status:
- Implemented in generated `README.md` through the Project Map, runtime inspection endpoints, and ownership guidance.

Goal:
- Every generated project should tell a human or AI agent where the contract lives, where business code lives, what endpoints expose runtime capability, and which files are generator-owned.

Deliverables:
- Generated `README.md` describes the project map.
- Generated `README.md` distinguishes user-owned and generator-owned files.
- Generated `README.md` points to `/debug/routes`, `/skill`, and `/skill?format=mcp` when skill output is enabled.
- Generator tests protect the orientation text so it does not drift silently.

## Phase 2: Capability Contract Tightening

Status:
- Implemented for generated README/skill output through `microgen.skill.v1` metadata and tests that protect generated capability metadata.

Goal:
- Keep IR as the single source of truth for generated runtime code, docs, client SDKs, OpenAI tools, and MCP tools.

Deliverables:
- Route, skill, SDK, README, and proto output continue deriving from IR.
- Integration coverage verifies generated capability metadata for IDL, Proto, and DB inputs.
- Unsupported contract shapes produce explicit guidance instead of vague placeholders.

## Phase 3: Extension Workflow Hardening

Status:
- Implemented in generated `README.md` through explicit `microgen extend -check`, append-service, append-model, and append-middleware guidance.

Goal:
- Make incremental change the normal workflow for existing generated projects.

Deliverables:
- `microgen extend -check` remains the first diagnostic command.
- `append-service`, `append-model`, and `append-middleware` preserve user-owned files.
- Failure output explains missing generator-owned seams and full-contract requirements.
- Docs keep extend mode framed as a product contract, not a merge helper.

## Phase 4: Config And Runtime Confidence

Status:
- Implemented in generated `README.md` for config-enabled projects through `file`, `hybrid`, and `remote` mode guidance plus environment override hints.

Goal:
- Keep generated services runnable locally while supporting production config needs.

Deliverables:
- `-config-mode file|hybrid|remote` behavior stays documented and tested.
- Remote provider validation and strict remote failure behavior are covered.
- `/debug/routes` remains available as a low-friction runtime inspection endpoint.

## Phase 5: Agent Workflow Packaging

Status:
- Implemented in generated `README.md` through the Agent Workflow loop and in repository docs through the maintainer/workflow entry points.

Goal:
- Let AI agents operate safely with a small, stable command and file map.

Deliverables:
- Repository docs keep a short "start here" path for AI sessions.
- Generated projects include enough local orientation to avoid reading framework internals first.
- Tooling docs map common changes to validation commands.
