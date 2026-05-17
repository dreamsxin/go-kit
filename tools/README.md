# tools/

Development and testing utilities for the go-kit framework.

## Files

| File | Purpose |
|------|---------|
| `integration_test.go` | Shared end-to-end test helpers and example smoke tests |
| `microgen_*_test.go` | Focused microgen CLI, generation, runtime, config, proto, and from-db integration tests |
| `readme_quickstart_test.go` | Verifies README quick-start commands stay runnable |
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

# Run only the microgen and README quick-start tests
go test ./tools/... -run 'Test(Microgen|ReadmeQuickStartSmoke)' -v

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

## microgen integration tests

Runs `microgen` against real IDL, Proto, config, and database inputs, then verifies generated projects compile and run where applicable:

| Test | Input | Verifies |
|------|-------|---------|
| `TestMicrogenCLIValidation` | CLI flag combinations | missing source, missing IDL, and unsupported driver errors stay clear |
| `CLI_FailsWithoutIDLOrFromDB` | no input flags | CLI rejects missing required source selection with a clear error |
| `CLI_FailsForMissingIDLPath` | nonexistent IDL path | CLI surfaces missing-file errors clearly instead of succeeding partially |
| `CLI_FailsForUnsupportedDriver` | unsupported `-driver` value | CLI rejects invalid generator driver configuration clearly |
| `TestMicrogenIDLDefaultFlags` | `cmd/microgen/parser/testdata/basic.go` | default CLI generation remains usable out of the box |
| `IDL_DefaultFlags` | `cmd/microgen/parser/testdata/basic.go` | default CLI generation remains usable out of the box: `go.mod`, `idl.go`, service, endpoint, HTTP transport, client, sdk, config, README, model, repository, skill, without gRPC or swag artifacts |
| `TestMicrogenIDLRuntimeIntegration` | IDL-generated runtime projects | generated HTTP services compile, start, and serve health/business routes |
| `IDL_GeneratedProject_BuildsAndRuns` | `cmd/microgen/parser/testdata/basic.go` with a minimal runnable flag set | generated project can resolve deps, compile `./cmd`, start successfully, serve `/health` plus `/skill`, and route a real business request through `/createuser` with the scaffold's expected JSON error |
| `IDL_MinimalProject_BuildsAndRunsWithoutOptionalFeatures` | `cmd/microgen/parser/testdata/basic.go` with `-config=false -docs=false -model=false -db=false -skill=false` | the leanest generated HTTP service still builds, its generated `service/endpoint/transport` packages can be assembled with framework logging in a component probe, it starts, serves `/health`, and keeps `/skill` disabled when optional layers are turned off |
| `IDL_PrefixedProject_BuildsAndServesPrefixedBusinessRoute` | `cmd/microgen/parser/testdata/basic.go` with `-prefix /api/runtime` and optional layers off | generated project still builds and runs with prefixed business routes, serves the scaffolded business endpoint at the prefixed path, and does not leave the old unprefixed path active |
| `IDL_FullGeneratedComponents_AreUsable` | `cmd/microgen/parser/testdata/basic.go` with `-skill` and runtime-friendly flags | the generated `cmd/`, `service/`, `endpoint/`, `transport/`, `client/`, `sdk/`, and `skill/` components all compile; a component probe can assemble `service + endpoint + transport + log`; the service starts; the demo client runs against it; and an SDK caller can hit the scaffolded API and receive the expected structured error |
| `TestMicrogenProtoIntegration` | Proto inputs | generated proto, HTTP/gRPC, client, sdk, skill, and README contracts stay aligned |
| `Proto_ComponentFlow_WhenProtocAvailable` | `testdata/service.proto` with `grpc` enabled and runtime-friendly flags | when `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc` are present, the generated proto stubs are built; the generated `service/`, `endpoint/`, `transport/`, `client/`, `sdk/`, and `skill/` components must compile together; a component probe can assemble `service + endpoint + transport + log`; and the generated gRPC transport stays compatible with modern `protoc-gen-go-grpc` server interfaces by embedding `Unimplemented...Server` |
| `TestMicrogenIDLContractIntegration` | IDL contract and rerun cases | generated structure, route prefixes, customized docs, and custom routes are preserved |
| `IDL` | `cmd/microgen/parser/testdata/basic.go` | `go.mod`, `idl.go`, service, endpoint, transport, client, sdk, docs, skill, `cmd/main.go`, and route-prefix propagation |
| `Proto` | `testdata/service.proto` | `go.mod`, service, endpoint, HTTP/gRPC transport, `pb/`, client, sdk, docs, skill, `cmd/main.go`, absence of `idl.go`, route-prefix propagation, generated proto message fields that stay aligned with the current contract, and a generated README that tells users to review the proto contract and run `protoc` before starting the service |
| `IDL_Rerun_PreservesCustomizedGoModAndDocs` | `cmd/microgen/parser/testdata/basic.go` | rerunning generation preserves customized `go.mod` content and real `docs/docs.go` content instead of overwriting them |
| `TestMicrogenExtendIntegration` | Existing generated projects | append-service, append-model, middleware, and check flows remain usable |
| `TestMicrogenFromDBIntegration` | SQLite schema | from-db generation builds and starts a runnable HTTP service |
| `TestMicrogenConfigIntegration` | Local and remote config | remote Consul config generation can fall back locally or fail in strict mode as expected |

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
