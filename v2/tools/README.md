# tools

Integration and documentation probes for the v2 framework and `microgen`.

## What Lives Here

- `integration_test.go`: shared process and example smoke helpers.
- `microgen_*_test.go`: CLI, generation, runtime, config, extend, Proto, and
  database integration tests.
- `readme_quickstart_test.go`: generated README workflow checks.
- `skill_test.go`: runtime API probes for the patterns referenced by `SKILL.md`.
- `SKILL.md`: concise framework instructions for AI coding agents.
- `testdata/`: generated-project fixtures and source contracts.

## Run Tests

From the v2 module:

```bash
# All integration tests.
go test ./tools -count=1

# CLI and generated-project flows.
go test ./tools -run 'TestMicrogen' -count=1

# AI skill API probes.
go test ./tools -run 'TestSKILL' -count=1
```

The full suite may start local HTTP/gRPC servers, generate temporary projects,
run `go mod tidy`, compile generated commands, and use `protoc` when it is
available.

## Generator Coverage

The tools suite covers:

- Go IDL default, minimal, prefixed, and component flows;
- Protobuf HTTP/gRPC generation and streaming contracts;
- SQLite database introspection and runnable output;
- local, hybrid, and strict remote configuration;
- append-service, append-model, middleware, and read-only extend checks;
- generated clients, SDKs, skill metadata, and interaction adapters;
- repeat-generation ownership and determinism.

Tracked directories under `testdata/` are expected-output fixtures. Update them
only through the owning generation tests and verify a second run produces no
additional diff.

## AI Skill

Reference [SKILL.md](SKILL.md) when an AI coding agent is building or modifying a
v2 service. The guide follows the same `service -> endpoint -> transport`
architecture and current v2 lifecycle APIs as the top-level README.
