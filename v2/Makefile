.PHONY: all build test lint clean help \
        install-microgen \
        gen gen-http gen-grpc gen-full gen-from-db \
        run-demo \
        proto \
        coverage \
        deps tools \
        test-runtime test-microgen test-contracts test-docs test-examples \
        test-observability test-security test-api test-boundaries test-race test-vet test-modules \
        release-check-clean verify-cross-platform verify-published-core verify-published verify verify-release

# Default variables, overridable from the command line.
IDL         ?= ./examples/usersvc/idl.go
OUT         ?= ./generated
IMPORT      ?= github.com/example/myapp
DB_DRIVER   ?= sqlite
DB_DSN      ?= app.db
DB_NAME     ?=
SERVICE     ?= AppService
HTTP_PORT   ?= :8080
PROTO_DIR   ?= $(OUT)/pb
VERSION     ?= v2.2.0

MICROGEN    := $(shell go env GOPATH)/bin/microgen

all: build

## workflow shortcuts
test-runtime:
	@echo ">>> Running runtime package tests..."
	@go test -v ./kit ./endpoint/... ./transport/... ./sd/... ./log ./interaction/... ./security/http ./observability/slog
	@go -C ./kit/grpc test -v ./...
	@go -C ./integrations/circuitbreaker test -v ./...
	@go -C ./integrations/consul test -v ./...
	@go -C ./integrations/grpc test -v ./...
	@go -C ./integrations/ratelimit test -v ./...
	@go -C ./integrations/zap test -v ./...

test-microgen:
	@echo ">>> Running microgen tests..."
	@go -C ./cmd/microgen test -v ./...
	@echo ">>> Running microgen integration tests..."
	@go -C ./tools test -v ./... -run "Test(Microgen|ReadmeQuickStartSmoke)"

test-contracts:
	@echo ">>> Generating and validating OpenAPI, JSON Schema, and TypeScript contracts..."
	@go -C ./tools/contractcheck test ./...
	@go -C ./tools test -v . -run "TestMicrogen(IDLContractIntegration|ProtoIntegration|FromDBIntegration)" -count=1
	@go -C ./tools run ./typecheck \
		./testdata/gen_idl_integration/sdk/typescript/tsconfig.json \
		./testdata/gen_proto_integration/sdk/typescript/tsconfig.json \
		./testdata/gen_fromdb_sqlite/sdk/typescript/tsconfig.json
	@echo ">>> Comparing generated Go and TypeScript SDK HTTP behavior..."
	@go -C ./tools test -v . -run TestGeneratedSDKBehaviorContract -count=1

test-docs:
	@echo ">>> Running documentation and UTF-8 verification..."
	@go -C ./tools test -v . -run "Test(RepositoryTextFilesAreUTF8|DocumentationLinksResolve|DocumentationDoesNotRecommendRemovedAPIs)" -count=1

test-examples:
	@echo ">>> Running example package tests..."
	@go -C ./examples test -v ./...
	@echo ">>> Running example smoke tests..."
	@go -C ./tools test -v ./... -run TestAllExamples

test-observability:
	@echo ">>> Running slog adapter tests..."
	@go test -v ./observability/slog
	@echo ">>> Running Zap adapter tests..."
	@go -C ./integrations/zap test -v ./...
	@echo ">>> Running OpenTelemetry adapter tests..."
	@go -C ./observability/otel test -v ./...

test-security:
	@echo ">>> Running optional HTTP security middleware tests..."
	@go test -v ./security/http

test-api:
	@echo ">>> Verifying the reviewed public API surface..."
	@go -C ./tools test -v . -run TestPublicAPISurfaceSnapshot -count=1

test-boundaries:
	@echo ">>> Verifying architecture and module dependency boundaries..."
	@go -C ./tools test -v . -run "Test(ArchitectureDependencyGates|CoreModuleHasNoThirdPartyRequirements|MicrogenImplementationPackagesAreInternal|PublishableModulesDoNotUseLocalReplacements|ReleaseManifestMatchesRepository)" -count=1

test-race:
	@echo ">>> Running focused race tests..."
	@go test -race ./kit ./interaction/... ./transport/... ./sd/... ./security/http ./observability/slog
	@go -C ./kit/grpc test -race ./...
	@go -C ./integrations/circuitbreaker test -race ./...
	@go -C ./integrations/consul test -race ./...
	@go -C ./integrations/grpc test -race ./...
	@go -C ./integrations/ratelimit test -race ./...
	@go -C ./integrations/zap test -race ./...
	@go -C ./observability/otel test -race ./...
	@go -C ./cmd/microgen test -race ./internal/generator

test-vet:
	@echo ">>> Running go vet across maintained modules..."
	@go vet ./...
	@go -C ./cmd/microgen vet ./...
	@go -C ./examples vet ./...
	@go -C ./integrations/circuitbreaker vet ./...
	@go -C ./integrations/consul vet ./...
	@go -C ./integrations/grpc vet ./...
	@go -C ./integrations/ratelimit vet ./...
	@go -C ./integrations/zap vet ./...
	@go -C ./kit/grpc vet ./...
	@go -C ./observability/otel vet ./...
	@go -C ./tools vet ./...
	@go -C ./tools/contractcheck vet ./...

test-modules:
	@echo ">>> Verifying module files are tidy..."
	@go mod tidy
	@go -C ./observability/otel mod tidy
	@go -C ./tools/contractcheck mod tidy
	@go -C ./cmd/microgen mod tidy
	@go -C ./examples mod tidy
	@go -C ./integrations/circuitbreaker mod tidy
	@go -C ./integrations/consul mod tidy
	@go -C ./integrations/grpc mod tidy
	@go -C ./integrations/ratelimit mod tidy
	@go -C ./integrations/zap mod tidy
	@go -C ./kit/grpc mod tidy
	@go -C ./tools mod tidy
	@git diff --exit-code -- go.mod go.sum go.work.sum \
		./cmd/microgen/go.mod ./cmd/microgen/go.sum \
		./examples/go.mod ./examples/go.sum \
		./integrations/circuitbreaker/go.mod ./integrations/circuitbreaker/go.sum \
		./integrations/consul/go.mod ./integrations/consul/go.sum \
		./integrations/grpc/go.mod ./integrations/grpc/go.sum \
		./integrations/ratelimit/go.mod ./integrations/ratelimit/go.sum \
		./integrations/zap/go.mod ./integrations/zap/go.sum \
		./kit/grpc/go.mod ./kit/grpc/go.sum \
		./observability/otel/go.mod ./observability/otel/go.sum \
		./tools/go.mod ./tools/go.sum \
		./tools/contractcheck/go.mod ./tools/contractcheck/go.sum

release-check-clean:
	@echo ">>> Verifying the v2 release scope is clean..."
	@go -C ./tools run ./releasecheck -scope .. -manifest ../RELEASE_MANIFEST.json -check-tags

verify-cross-platform:
	@echo ">>> Running cross-platform release suites..."
	@go -C ./tools run ./releaseverify -root .. -suites test,standalone,vet,tidy,race

verify-published-core:
	@echo ">>> Verifying the published core module..."
	@go -C ./tools run ./releasecheck -scope .. -manifest ../RELEASE_MANIFEST.json -published-order 1

verify-published:
	@echo ">>> Verifying all published modules..."
	@go -C ./tools run ./releasecheck -scope .. -manifest ../RELEASE_MANIFEST.json -published-order 2

verify: build test-runtime test-microgen test-docs test-examples test-observability test-security
	@echo ">>> Verification pass completed."

verify-release: verify test-contracts test-api test-boundaries test-race test-vet test-modules
	@echo ">>> Release verification pass completed."

## install-microgen: build and install microgen into GOPATH/bin
install-microgen:
	@echo ">>> Installing microgen..."
	@go -C ./cmd/microgen install .
	@echo "    Done: $(MICROGEN)"

## gen: generate HTTP service with model/repository/OpenAPI
gen: install-microgen
	@echo ">>> Generating HTTP service from $(IDL) -> $(OUT)"
	@$(MICROGEN) \
		-idl    $(IDL) \
		-out    $(OUT) \
		-import $(IMPORT) \
		-protocols http \
		-config \
		-model \
		-db \
		-driver $(DB_DRIVER) \
		-openapi
	@echo ">>> Done. Next steps:"
	@echo "    cd $(OUT) && go mod tidy"
	@echo "    go run ./cmd -http.addr :8080"

## gen-http: generate HTTP-only service
gen-http: install-microgen
	@echo ">>> Generating HTTP-only service from $(IDL) -> $(OUT)"
	@$(MICROGEN) \
		-idl    $(IDL) \
		-out    $(OUT) \
		-import $(IMPORT) \
		-protocols http

## gen-grpc: generate HTTP + gRPC service with model/repository/OpenAPI
gen-grpc: install-microgen
	@echo ">>> Generating HTTP+gRPC service from $(IDL) -> $(OUT)"
	@$(MICROGEN) \
		-idl    $(IDL) \
		-out    $(OUT) \
		-import $(IMPORT) \
		-protocols http,grpc \
		-config \
		-model \
		-db \
		-driver $(DB_DRIVER) \
		-openapi

## gen-full: generate HTTP + gRPC + model + OpenAPI + tests
gen-full: install-microgen
	@echo ">>> Generating full service from $(IDL) -> $(OUT)"
	@$(MICROGEN) \
		-idl      $(IDL) \
		-out      $(OUT) \
		-import   $(IMPORT) \
		-protocols http,grpc \
		-config   \
		-model \
		-db \
		-driver $(DB_DRIVER) \
		-openapi \
		-tests

## gen-from-db: generate RESTful service from an existing database
gen-from-db: install-microgen
	@echo ">>> Generating service from database [driver=$(DB_DRIVER)] -> $(OUT)"
	@$(MICROGEN) \
		-from-db \
		-driver  $(DB_DRIVER) \
		-dsn     "$(DB_DSN)" \
		$(if $(DB_NAME),-dbname $(DB_NAME),) \
		-service $(SERVICE) \
		-out     $(OUT) \
		-import  $(IMPORT) \
		-config  \
		-openapi
	@echo ">>> Done. Next steps:"
	@echo "    cd $(OUT) && go mod tidy"
	@echo "    go run ./cmd"

## run-demo: run the generated service from OUT
run-demo:
	@go run ./tools/makeguard dir-exists "$(OUT)" "output directory '$(OUT)' not found. Run 'make gen' first."
	@echo ">>> Starting demo service on $(HTTP_PORT)..."
	@cd $(OUT) && go run ./cmd -http.addr $(HTTP_PORT)

## proto: generate protobuf Go files from PROTO_DIR
proto:
	@go run ./tools/makeguard proto "$(PROTO_DIR)"

## build: build the whole repository
build:
	@echo ">>> Building go-kit..."
	@go build ./...
	@go -C ./cmd/microgen build ./...
	@go -C ./examples build ./...
	@go -C ./integrations/circuitbreaker build ./...
	@go -C ./integrations/consul build ./...
	@go -C ./integrations/grpc build ./...
	@go -C ./integrations/ratelimit build ./...
	@go -C ./integrations/zap build ./...
	@go -C ./kit/grpc build ./...
	@go -C ./observability/otel build ./...
	@go -C ./tools build ./...
	@go -C ./tools/contractcheck build ./...

## test: run all tests with the race detector
test:
	@echo ">>> Running tests..."
	@$(MAKE) test-runtime test-microgen test-docs test-examples test-observability test-security test-race

## coverage: generate an HTML coverage report
coverage:
	@echo ">>> Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo ">>> Generated: coverage.html"

## lint: run golangci-lint
lint:
	@echo ">>> Running linter..."
	@golangci-lint run

## deps: tidy and download Go module dependencies
deps:
	@echo ">>> Tidying dependencies..."
	@$(MAKE) test-modules
	@go work sync

## tools: install local development tools
tools:
	@echo ">>> Installing tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo ">>> All tools installed"

## clean: clean build artifacts
clean:
	@echo ">>> Cleaning..."
	@go clean
	@go run ./tools/makeguard remove coverage.out coverage.html

## help: show available targets
help:
	@echo ""
	@echo "go-kit / microgen"
	@echo ""
	@echo "Generation"
	@echo "  make gen           Generate HTTP service + model + db + OpenAPI"
	@echo "  make gen-http      Generate HTTP-only service"
	@echo "  make gen-grpc      Generate HTTP + gRPC service"
	@echo "  make gen-full      Generate HTTP + gRPC + model + db + OpenAPI + tests"
	@echo "  make gen-from-db   Generate service from an existing database"
	@echo ""
	@echo "Validation"
	@echo "  make build         Build the repository"
	@echo "  make test          Run full test suite with race detector"
	@echo "  make test-runtime  Run focused runtime/framework tests"
	@echo "  make test-microgen Run generator tests and integration coverage"
	@echo "  make test-contracts Validate generated contracts and TypeScript SDKs"
	@echo "  make test-docs     Verify documentation links and UTF-8"
	@echo "  make test-examples Run example tests and smoke tests"
	@echo "  make test-observability Test slog and optional OpenTelemetry adapters"
	@echo "  make test-security Test optional HTTP security middleware"
	@echo "  make test-api      Verify the reviewed exported API snapshot"
	@echo "  make test-boundaries Verify architecture and module dependency boundaries"
	@echo "  make test-race     Run focused race tests across release-critical packages"
	@echo "  make test-vet      Run go vet across maintained modules"
	@echo "  make test-modules  Verify all maintained go.mod files are tidy"
	@echo "  make verify        Run recommended pre-merge validation pass"
	@echo "  make verify-release Run the full release validation pass"
	@echo "  make release-check-clean Verify the committed v2 scope and release tag phase"
	@echo "  make verify-cross-platform Run the same release suites used by Linux/Windows CI"
	@echo "  make verify-published-core Verify the published core module from the manifest"
	@echo "  make verify-published Verify every published module from the manifest"
	@echo "  make coverage      Generate coverage report"
	@echo "  make lint          Run golangci-lint"
	@echo ""
	@echo "Tooling"
	@echo "  make install-microgen  Install microgen into GOPATH/bin"
	@echo "  make proto             Generate protobuf Go files"
	@echo "  make tools             Install local development tools"
	@echo "  make deps              Run go mod tidy and download deps"
	@echo "  make clean             Clean build artifacts"
	@echo ""
	@echo "Examples"
	@echo "  make run-demo      Run generated demo service from OUT"
	@echo ""
