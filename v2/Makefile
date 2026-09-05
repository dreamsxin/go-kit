.PHONY: all build test lint clean help \
        install-microgen \
        gen gen-http gen-grpc gen-full gen-from-db \
        run-demo \
        proto \
        coverage \
        deps tools \
        test-runtime test-microgen test-contracts test-docs test-examples \
        test-observability test-security test-api test-snapshots update-snapshots test-boundaries test-race test-fmt test-vet test-modules \
        release-check-clean verify-published verify

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
VERSION     ?= v2.8.1

MICROGEN    := $(shell go env GOPATH)/bin/microgen

all: build

## workflow shortcuts
test-runtime:
	@echo ">>> Running runtime package tests..."
	@go test -v ./kit/... ./endpoint/... ./transport/... ./sd/... ./security/http ./observability/slog ./interaction/... ./integrations/...

test-microgen:
	@echo ">>> Running microgen tests..."
	@go test -v ./cmd/microgen/...
	@echo ">>> Running microgen integration tests..."
	@go -C ./tools test -v ./... -run "Test(Microgen|ReadmeQuickStartSmoke)"

test-contracts:
	@echo ">>> Generating and validating OpenAPI, JSON Schema, and TypeScript contracts..."
	@go -C ./tools/contractcheck test ./...
	@go -C ./tools test -v . -run "TestMicrogen(IDLContractIntegration|ProtoIntegration|FromDBIntegration)" -count=1
	@echo ">>> Comparing generated Go and TypeScript SDK HTTP behavior..."
	@go -C ./tools test -v . -run TestGeneratedSDKBehaviorContract -count=1

test-docs:
	@echo ">>> Running documentation and UTF-8 verification..."
	@go -C ./tools test -v . -run "Test(RepositoryTextFilesAreUTF8|DocumentationLinksResolve|DocumentationDoesNotRecommendRemovedAPIs|DocumentationReferencesExistingAPIs)" -count=1

test-examples:
	@echo ">>> Running example package tests..."
	@go -C ./examples test -v ./...
	@echo ">>> Running example smoke tests..."
	@go -C ./tools test -v ./... -run TestAllExamples

test-observability:
	@echo ">>> Running slog adapter tests..."
	@go test -v ./observability/slog
	@echo ">>> Running Zap adapter tests..."
	@go test -v ./integrations/zap
	@echo ">>> Running OpenTelemetry adapter tests..."
	@go test -v ./observability/otel

test-security:
	@echo ">>> Running optional HTTP security middleware tests..."
	@go test -v ./security/http

test-api:
	@echo ">>> Verifying the reviewed public API surface..."
	@go -C ./tools test -v . -run TestPublicAPISurfaceSnapshot -count=1

test-boundaries:
	@echo ">>> Verifying architecture and module dependency boundaries..."
	@go -C ./tools test -v . -run "Test(ArchitectureDependencyGates|KitHTTPAssemblyDoesNotResolveOptionalDependencies|MicrogenImplementationPackagesAreInternal|OnlyOneModuleIsPublishable|PublishableModulesDoNotUseLocalReplacements|ReleaseManifestMatchesRepository)" -count=1

test-race:
	@echo ">>> Running race tests..."
	@go -C ./tools run ./releaseverify -root .. -suites race

test-snapshots:
	@echo ">>> Verifying the reviewed snapshots..."
	@go -C ./tools test . -run "Test(PublicAPISurfaceSnapshot|Microgen(IDLContract|Proto|FromDB)Integration)" -count=1

update-snapshots:
	@echo ">>> Refreshing the reviewed public API snapshot..."
	@go -C ./tools test . -run TestPublicAPISurfaceSnapshot -count=1 -args -update-api-snapshot
	@echo ">>> Refreshing the reviewed generated contract snapshots..."
	@go -C ./tools test . -run TestMicrogen -count=1 -args -update-contract-snapshots
	@echo ">>> Review the diff before committing: git diff -- v2/tools/testdata"

test-fmt:
	@echo ">>> Verifying gofmt formatting..."
	@go run ./tools/makeguard gofmt .

test-vet:
	@echo ">>> Running go vet across maintained modules..."
	@go vet ./...
	@go -C ./examples vet ./...
	@go -C ./tools vet ./...
	@go -C ./tools/contractcheck vet ./...

test-modules:
	@echo ">>> Verifying module files are tidy..."
	@go mod tidy
	@go -C ./examples mod tidy
	@go -C ./tools mod tidy
	@go -C ./tools/contractcheck mod tidy
	@git diff --exit-code -- go.mod go.sum go.work go.work.sum \
		./examples/go.mod ./examples/go.sum \
		./tools/go.mod ./tools/go.sum \
		./tools/contractcheck/go.mod ./tools/contractcheck/go.sum

release-check-clean:
	@echo ">>> Verifying the v2 release scope is clean..."
	@go -C ./tools run ./releasecheck -scope .. -manifest ../RELEASE_MANIFEST.json -check-tags

## verify: run exactly what CI runs, so local green means CI green
##
## The tidy suite diffs the worktree, so run this from a committed tree. Use
## `make test` and the focused test-* targets while iterating.
verify: test-fmt
	@echo ">>> Running the release suites CI runs..."
	@go -C ./tools run ./releaseverify -root .. -suites test,standalone,vet,tidy,race

verify-published:
	@echo ">>> Verifying the published module..."
	@go -C ./tools run ./releasecheck -scope .. -manifest ../RELEASE_MANIFEST.json -check-published

## install-microgen: build and install microgen into GOPATH/bin
install-microgen:
	@echo ">>> Installing microgen..."
	@go install ./cmd/microgen
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
	@go -C ./examples build ./...
	@go -C ./tools build ./...
	@go -C ./tools/contractcheck build ./...

## test: run the focused runtime and generator tests
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
	@echo "  make test-snapshots Verify every reviewed snapshot (API and generated contracts)"
	@echo "  make update-snapshots Refresh those snapshots after an intended change"
	@echo "  make test-boundaries Verify architecture and module dependency boundaries"
	@echo "  make test-race     Run the race detector over every core package"
	@echo "  make test-fmt      Verify every Go file is gofmt-formatted"
	@echo "  make test-vet      Run go vet across maintained modules"
	@echo "  make test-modules  Verify all maintained go.mod files are tidy"
	@echo "  make verify        Run exactly what CI runs (needs a committed tree)"
	@echo "  make release-check-clean Verify the committed v2 scope and release tag phase"
	@echo "  make verify-published Verify the published module from the manifest"
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
