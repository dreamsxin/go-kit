.PHONY: all build test lint clean help \
        install-microgen \
        gen gen-http gen-grpc gen-full gen-from-db \
        run-demo swag-demo \
        proto \
        swag \
        coverage \
        deps tools \
        test-runtime test-microgen test-docs test-examples verify

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
	@go test -v ./tools/... -run TestMicrogenIntegration

test-docs:
	@echo ">>> Running docs and skill verification..."
	@go test -v ./tools/... -run TestSKILL

test-examples:
	@echo ">>> Running example package tests..."
	@go test -v ./examples/...
	@echo ">>> Running example smoke tests..."
	@go test -v ./tools/... -run TestAllExamples

verify: build test-runtime test-microgen test-docs test-examples
	@echo ">>> Verification pass completed."

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
		-swag
	@echo ">>> Done. Next steps:"
	@echo "    cd $(OUT) && go mod tidy"
	@echo "    go run ./cmd/main.go -http.addr :8080"

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
		-swag

## gen-full: generate HTTP + gRPC + model + swag + tests
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
		-swag \
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
		-swag
	@echo ">>> Done. Next steps:"
	@echo "    cd $(OUT) && go mod tidy"
	@echo "    go run ./cmd/main.go"

## run-demo: run the generated service from OUT
run-demo:
	@if [ ! -d "$(OUT)" ]; then \
		echo "Error: output directory '$(OUT)' not found. Run 'make gen' first."; \
		exit 1; \
	fi
	@echo ">>> Starting demo service on $(HTTP_PORT)..."
	@cd $(OUT) && go run ./cmd/main.go -http.addr $(HTTP_PORT)

## swag-demo: regenerate Swagger docs for the generated service in OUT
swag-demo:
	@if [ ! -d "$(OUT)" ]; then \
		echo "Error: output directory '$(OUT)' not found. Run 'make gen' first."; \
		exit 1; \
	fi
	@echo ">>> Running swag init for $(OUT)..."
	@cd $(OUT) && swag init -g cmd/main.go -o docs
	@echo ">>> Done. Start with: make run-demo"

## proto: generate protobuf Go files from PROTO_DIR
proto:
	@echo ">>> Generating protobuf Go files..."
	@for dir in $(shell find $(PROTO_DIR) -name "*.proto" -exec dirname {} \; | sort -u); do \
		echo "  protoc: $$dir"; \
		protoc \
			--proto_path=$$dir \
			--go_out=$$dir --go_opt=paths=source_relative \
			--go-grpc_out=$$dir --go-grpc_opt=paths=source_relative \
			$$dir/*.proto; \
	done
	@echo ">>> Done. pb.go files generated."

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
	@rm -f coverage.out coverage.html

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
	@echo "  make test-docs     Verify docs-backed snippets and SKILL.md"
	@echo "  make test-examples Run example tests and smoke tests"
	@echo "  make verify        Run recommended pre-merge validation pass"
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
