# tools/

Development and testing utilities for the go-kit framework.

## Files

| File | Purpose |
|------|---------|
| `integration_test.go` | End-to-end tests: builds examples and runs microgen integration tests |
| `skill_test.go` | Verifies every code snippet in `SKILL.md` compiles and runs correctly |
| `SKILL.md` | AI skill file — teaches an AI assistant how to use this framework |
| `testdata/service.proto` | Proto file used by the microgen integration test |

---

## Running Tests

```bash
# Run all tools tests (integration + skill verification)
go test ./tools/... -v

# Run only the example smoke tests
go test ./tools/... -run TestAllExamples -v

# Run only the microgen integration tests
go test ./tools/... -run TestMicrogenIntegration -v

# Run only the SKILL.md verification tests
go test ./tools/... -run TestSKILL -v
```

---

## integration_test.go

### TestAllExamples
Builds and smoke-tests the runnable examples:

| Example | Port | Smoke Tests |
|---------|------|-------------|
| `quickstart` | 8082 | `POST /hello` → "Hello, world!" |
| `best_practice` | 8083 | `POST /hello` → "Hello, Alice!" |
| `microgen_skill` | 8084 | `/sayhello`, `/skill`, `/skill?format=mcp` |

### TestMicrogenIntegration
Runs `microgen` against real IDL and Proto files and verifies the generated file structure:

| Sub-test | Input | Verifies |
|----------|-------|---------|
| `CLI_FailsWithoutIDLOrFromDB` | no input flags | CLI rejects missing required source selection with a clear error |
| `CLI_FailsForMissingIDLPath` | nonexistent IDL path | CLI surfaces missing-file errors clearly instead of succeeding partially |
| `CLI_FailsForUnsupportedDriver` | unsupported `-driver` value | CLI rejects invalid generator driver configuration clearly |
| `IDL_DefaultFlags` | `cmd/microgen/parser/testdata/basic.go` | default CLI generation remains usable out of the box: `go.mod`, `idl.go`, service, endpoint, HTTP transport, client, sdk, config, README, model, repository, skill, without gRPC or swag artifacts |
| `IDL_GeneratedProject_BuildsAndRuns` | `cmd/microgen/parser/testdata/basic.go` with a minimal runnable flag set | generated project can resolve deps, compile `./cmd`, start successfully, and serve `/health` plus `/skill` |
| `IDL_MinimalProject_BuildsAndRunsWithoutOptionalFeatures` | `cmd/microgen/parser/testdata/basic.go` with `-config=false -docs=false -model=false -db=false -skill=false` | the leanest generated HTTP service still builds, starts, serves `/health`, and keeps `/skill` disabled when optional layers are turned off |
| `IDL` | `cmd/microgen/parser/testdata/basic.go` | `go.mod`, `idl.go`, service, endpoint, transport, client, sdk, docs, skill, `cmd/main.go`, and route-prefix propagation |
| `Proto` | `testdata/service.proto` | `go.mod`, service, endpoint, HTTP/gRPC transport, `pb/`, client, sdk, docs, skill, `cmd/main.go`, absence of `idl.go`, and route-prefix propagation |
| `IDL_Rerun_PreservesCustomizedGoModAndDocs` | `cmd/microgen/parser/testdata/basic.go` | rerunning generation preserves customized `go.mod` content and real `docs/docs.go` content instead of overwriting them |

---

## skill_test.go

Verifies every code snippet in `SKILL.md` compiles and produces the expected output.

**Coverage:** Tests covering all major sections of SKILL.md:

| Test | SKILL.md section |
|------|-----------------|
| `TestSKILL_30SecondService` | 30-Second Service |
| `TestSKILL_ProductionServicePattern` | Production Service Pattern |
| `TestSKILL_EndpointAPI_*` | Key APIs — endpoint |
| `TestSKILL_HTTPServer_*` | Key APIs — transport/http/server |
| `TestSKILL_HTTPClient_*` | Key APIs — transport/http/client |
| `TestSKILL_SD_*` | Key APIs — sd package |
| `TestSKILL_Log_*` | Key APIs — log package |
| `TestSKILL_Hystrix_*` | Hystrix circuit breaker |
| `TestSKILL_TestingPatterns_*` | Testing Patterns |
| `TestSKILL_CommonMistakes_*` | Common Mistakes |

---

## SKILL.md

An AI skill file that teaches an AI assistant (like Kiro) how to use this framework.

**Usage:** Reference this file when asking an AI to build a service with this framework.
In Kiro, add it as a steering file or reference it with `#File tools/SKILL.md`.
