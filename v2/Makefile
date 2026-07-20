.PHONY: all build test lint clean help \
        install-microgen \
        gen gen-http gen-grpc gen-full gen-from-db \
        run-demo swag-demo \
        proto \
        swag \
        coverage \
        deps tools \
        test-runtime test-microgen test-contracts test-docs test-examples \
        test-observability test-security test-api test-race test-vet test-modules \
        release-check-clean verify verify-release

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

MICROGEN    := $(shell go env GOPATH)/bin/microgen

all: build

## workflow shortcuts
test-runtime:
	@echo ">>> Running runtime package tests..."
	@go test -v ./kit ./endpoint ./transport/... ./sd/... ./log ./utils

test-microgen:
	@echo ">>> Running microgen tests..."
	@go test -v ./cmd/microgen/...
	@echo ">>> Running microgen integration tests..."
	@go test -v ./tools/... -run "Test(Microgen|ReadmeQuickStartSmoke)"

test-contracts:
	@echo ">>> Generating and validating OpenAPI, JSON Schema, and TypeScript contracts..."
	@cd ./tools/contractcheck && go test ./...
	@go test -v ./tools -run "TestMicrogen(IDLContractIntegration|ProtoIntegration|FromDBIntegration)" -count=1
	@go run ./tools/typecheck \
		./tools/testdata/gen_idl_integration/sdk/typescript/tsconfig.json \
		./tools/testdata/gen_proto_integration/sdk/typescript/tsconfig.json \
		./tools/testdata/gen_fromdb_sqlite/sdk/typescript/tsconfig.json
	@echo ">>> Comparing generated Go and TypeScript SDK HTTP behavior..."
	@go test -v ./tools -run TestGeneratedSDKBehaviorContract -count=1

test-docs:
	@echo ">>> Running documentation, UTF-8, and skill verification..."
	@go test -v ./tools -run "Test(SKILL|RepositoryTextFilesAreUTF8|DocumentationLinksResolve)" -count=1

test-examples:
	@echo ">>> Running example package tests..."
	@go test -v ./examples/...
	@echo ">>> Running example smoke tests..."
	@go test -v ./tools/... -run TestAllExamples

test-observability:
	@echo ">>> Running slog adapter tests..."
	@go test -v ./observability/slog
	@echo ">>> Running OpenTelemetry adapter tests..."
	@cd ./observability/otel && go test -v ./...

test-security:
	@echo ">>> Running optional HTTP security middleware tests..."
	@go test -v ./security/http

test-api:
	@echo ">>> Verifying the reviewed public API surface..."
	@go test -v ./tools -run TestPublicAPISurfaceSnapshot -count=1

test-race:
	@echo ">>> Running focused race tests..."
	@go test -race ./kit ./interaction/... ./transport/... ./sd/... ./cmd/microgen/generator ./security/http ./observability/slog
	@cd ./observability/otel && go test -race ./...

test-vet:
	@echo ">>> Running go vet across maintained modules..."
	@go vet ./...
	@cd ./observability/otel && go vet ./...
	@cd ./tools/contractcheck && go vet ./...

test-modules:
	@echo ">>> Verifying module files are tidy..."
	@go mod tidy -diff
	@cd ./observability/otel && go mod tidy -diff
	@cd ./tools/contractcheck && go mod tidy -diff

release-check-clean:
	@echo ">>> Verifying the v2 release scope is clean..."
	@go run ./tools/releasecheck

verify: build test-runtime test-microgen test-docs test-examples test-observability test-security
	@echo ">>> Verification pass completed."

verify-release: verify test-contracts test-api test-race test-vet test-modules
	@echo ">>> Release verification pass completed."

## install-microgen: build and install microgen into GOPATH/bin
install-microgen:
	@echo ">>> Installing microgen..."
	@go install ./cmd/microgen
	@echo "    Done: $(MICROGEN)"

## gen: generate HTTP service with model/repository/swag
gen: install-microgen
	@echo ">>> Generating HTTP service from $(IDL) -> $(OUT)"
	@$(MICROGEN) \
		-idl    $(IDL) \
		-out    $(OUT) \
		-import $(IMPORT) \
		-protocols http \
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

## gen-grpc: generate HTTP + gRPC service with model/repository/swag
gen-grpc: install-microgen
	@echo ">>> Generating HTTP+gRPC service from $(IDL) -> $(OUT)"
	@$(MICROGEN) \
		-idl    $(IDL) \
		-out    $(OUT) \
		-import $(IMPORT) \
		-protocols http,grpc \
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
		-openapi
	@echo ">>> Done. Next steps:"
	@echo "    cd $(OUT) && go mod tidy"
	@echo "    go run ./cmd"

## run-demo: run the generated service from OUT
run-demo:
	@go run ./tools/makeguard dir-exists "$(OUT)" "output directory '$(OUT)' not found. Run 'make gen' first."
	@echo ">>> Starting demo service on $(HTTP_PORT)..."
	@cd $(OUT) && go run ./cmd -http.addr $(HTTP_PORT)

## swag-demo: regenerate Swagger docs for the generated service in OUT
swag-demo:
	@go run ./tools/makeguard dir-exists "$(OUT)" "output directory '$(OUT)' not found. Run 'make gen' first."
	@echo ">>> Running swag init for $(OUT)..."
	@cd $(OUT) && swag init -g cmd/main.go -o docs
	@echo ">>> Done. Start with: make run-demo"

## swag: alias for swag-demo
swag: swag-demo

## proto: generate protobuf Go files from PROTO_DIR
proto:
	@go run ./tools/makeguard proto "$(PROTO_DIR)"

## build: build the whole repository
build:
	@echo ">>> Building go-kit..."
	@go build ./...

## test: run all tests with the race detector
test:
	@echo ">>> Running tests..."
	@go test -v -race ./...

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
	@go mod tidy
	@go mod download

## tools: install local development tools
tools:
	@echo ">>> Installing tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/swaggo/swag/cmd/swag@latest
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
	@echo "  make gen           Generate HTTP service + model + db + swag"
	@echo "  make gen-http      Generate HTTP-only service"
	@echo "  make gen-grpc      Generate HTTP + gRPC service"
	@echo "  make gen-full      Generate HTTP + gRPC + model + db + swag + tests"
	@echo "  make gen-from-db   Generate service from an existing database"
	@echo ""
	@echo "Validation"
	@echo "  make build         Build the repository"
	@echo "  make test          Run full test suite with race detector"
	@echo "  make test-runtime  Run focused runtime/framework tests"
	@echo "  make test-microgen Run generator tests and integration coverage"
	@echo "  make test-contracts Validate generated contracts and TypeScript SDKs"
	@echo "  make test-docs     Verify docs-backed snippets and SKILL.md"
	@echo "  make test-examples Run example tests and smoke tests"
	@echo "  make test-observability Test slog and optional OpenTelemetry adapters"
	@echo "  make test-security Test optional HTTP security middleware"
	@echo "  make test-api      Verify the reviewed exported API snapshot"
	@echo "  make test-race     Run focused race tests across release-critical packages"
	@echo "  make test-vet      Run go vet across maintained modules"
	@echo "  make test-modules  Verify all maintained go.mod files are tidy"
	@echo "  make verify        Run recommended pre-merge validation pass"
	@echo "  make verify-release Run the full release validation pass"
	@echo "  make release-check-clean Verify the committed v2 scope is clean"
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
	@echo "  make swag-demo     Regenerate Swagger docs for OUT"
	@echo ""
