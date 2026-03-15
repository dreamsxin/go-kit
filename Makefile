.PHONY: all build test lint clean help \
        install-microgen \
        gen gen-http gen-grpc gen-full \
        run run-demo \
        proto \
        swag \
        deps tools

# ──────────────────────────────────────────────────────────────────────────────
# 变量（可通过命令行覆盖）
# ──────────────────────────────────────────────────────────────────────────────

# 示例 IDL 文件
IDL         ?= ./examples/usersvc/idl.go

# 生成输出目录
OUT         ?= ./generated

# 生成项目的 Go import path
IMPORT      ?= github.com/example/myapp

# 数据库驱动 (sqlite | mysql | postgres | sqlserver | clickhouse)
DB_DRIVER   ?= sqlite

# HTTP 监听端口（run-demo 使用）
HTTP_PORT   ?= :8080

# microgen 可执行文件路径
MICROGEN    := $(shell go env GOPATH)/bin/microgen

# ──────────────────────────────────────────────────────────────────────────────
# 默认目标
# ──────────────────────────────────────────────────────────────────────────────

all: build

# ──────────────────────────────────────────────────────────────────────────────
# 安装 microgen 代码生成工具
# ──────────────────────────────────────────────────────────────────────────────

## install-microgen: 编译并安装 microgen 到 $GOPATH/bin
install-microgen:
	@echo ">>> Installing microgen..."
	@go install ./cmd/microgen
	@echo "    Done: $(MICROGEN)"

# ──────────────────────────────────────────────────────────────────────────────
# 代码生成
# ──────────────────────────────────────────────────────────────────────────────

## gen: 生成 HTTP 服务（SQLite，带 model/repository/swag）
gen: install-microgen
	@echo ">>> Generating HTTP service from $(IDL) → $(OUT)"
	@$(MICROGEN) \
		-idl    $(IDL) \
		-out    $(OUT) \
		-import $(IMPORT) \
		-protocols http \
		-model \
		-db \
		-db.driver $(DB_DRIVER) \
		-swag
	@echo ">>> Done. Next steps:"
	@echo "    cd $(OUT) && go mod init $(IMPORT) && go mod tidy"
	@echo "    swag init -g cmd/main.go"
	@echo "    go run ./cmd/main.go"

## gen-http: 仅生成 HTTP 服务（不含 model/swag）
gen-http: install-microgen
	@echo ">>> Generating HTTP-only service from $(IDL) → $(OUT)"
	@$(MICROGEN) \
		-idl    $(IDL) \
		-out    $(OUT) \
		-import $(IMPORT) \
		-protocols http

## gen-grpc: 生成 HTTP + gRPC 双协议服务（含 model/swag）
gen-grpc: install-microgen
	@echo ">>> Generating HTTP+gRPC service from $(IDL) → $(OUT)"
	@$(MICROGEN) \
		-idl    $(IDL) \
		-out    $(OUT) \
		-import $(IMPORT) \
		-protocols http,grpc \
		-model \
		-db \
		-db.driver $(DB_DRIVER) \
		-swag

## gen-full: 完整生成（HTTP+gRPC + model + swag + test）
gen-full: install-microgen
	@echo ">>> Generating FULL service (HTTP+gRPC+model+swag+tests) → $(OUT)"
	@$(MICROGEN) \
		-idl      $(IDL) \
		-out      $(OUT) \
		-import   $(IMPORT) \
		-protocols http,grpc \
		-model \
		-db \
		-db.driver $(DB_DRIVER) \
		-swag \
		-tests

# ──────────────────────────────────────────────────────────────────────────────
# 运行示例（generated-usersvc）
# ──────────────────────────────────────────────────────────────────────────────

## run-demo: 直接运行 generated-usersvc 示例服务
run-demo:
	@echo ">>> Starting demo service on $(HTTP_PORT)..."
	@cd generated-usersvc && go run ./cmd/main.go -http.addr $(HTTP_PORT)

## swag-demo: 为 generated-usersvc 重新生成 Swagger 文档
swag-demo:
	@echo ">>> Running swag init for generated-usersvc..."
	@cd generated-usersvc && swag init -g cmd/main.go -o docs
	@echo ">>> Done. Start with: make run-demo"

# ──────────────────────────────────────────────────────────────────────────────
# gRPC protobuf 代码生成
# ──────────────────────────────────────────────────────────────────────────────

## proto: 从 pb/ 目录下所有 .proto 文件生成 pb.go（需安装 protoc）
## 用法: make proto OUT=./your-service
## 注意: 在 Linux/macOS 上使用 find；Windows 请直接运行 protoc 命令
proto:
	@echo ">>> Generating protobuf Go files from $(OUT)/pb/..."
	@protoc \
		--go_out=$(OUT) \
		--go-grpc_out=$(OUT) \
		--go_opt=paths=source_relative \
		--go-grpc_opt=paths=source_relative \
		--proto_path=$(OUT) \
		$(OUT)/pb/*/*.proto
	@echo ">>> Done. pb.go files generated."

# ──────────────────────────────────────────────────────────────────────────────
# 构建 & 测试
# ──────────────────────────────────────────────────────────────────────────────

## build: 构建整个项目
build:
	@echo ">>> Building go-kit..."
	@go build ./...

## test: 运行所有测试
test:
	@echo ">>> Running tests..."
	@go test -v -race ./...

## coverage: 生成测试覆盖率报告（coverage.html）
coverage:
	@echo ">>> Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo ">>> Opened: coverage.html"

## lint: 静态检查
lint:
	@echo ">>> Running linter..."
	@golangci-lint run

# ──────────────────────────────────────────────────────────────────────────────
# 依赖管理
# ──────────────────────────────────────────────────────────────────────────────

## deps: go mod tidy + download
deps:
	@echo ">>> Tidying dependencies..."
	@go mod tidy
	@go mod download

## tools: 安装所有开发工具
tools:
	@echo ">>> Installing tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/swaggo/swag/cmd/swag@latest
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo ">>> All tools installed"

# ──────────────────────────────────────────────────────────────────────────────
# 清理
# ──────────────────────────────────────────────────────────────────────────────

## clean: 清理构建产物
clean:
	@echo ">>> Cleaning..."
	@go clean
	@rm -f coverage.out coverage.html

# ──────────────────────────────────────────────────────────────────────────────
# 帮助
# ──────────────────────────────────────────────────────────────────────────────

## help: 显示所有可用目标
help:
	@echo ""
	@echo "  microgen — Go 微服务代码生成器"
	@echo ""
	@echo "  ┌─ 代码生成 ──────────────────────────────────────────────┐"
	@echo "  │  make gen           HTTP服务 + SQLite + model + swag     │"
	@echo "  │  make gen-http      仅 HTTP 服务（最小化）                │"
	@echo "  │  make gen-grpc      HTTP + gRPC + model + swag           │"
	@echo "  │  make gen-full      HTTP + gRPC + model + swag + tests   │"
	@echo "  │                                                           │"
	@echo "  │  自定义参数示例：                                          │"
	@echo "  │    make gen IDL=./myapp/idl.go OUT=./myapp IMPORT=github.com/me/myapp │"
	@echo "  │    make gen DB_DRIVER=mysql                               │"
	@echo "  └───────────────────────────────────────────────────────────┘"
	@echo ""
	@echo "  ┌─ 运行示例 ──────────────────────────────────────────────┐"
	@echo "  │  make run-demo      启动 generated-usersvc（:8080）       │"
	@echo "  │  make swag-demo     重新生成 Swagger 文档                 │"
	@echo "  └───────────────────────────────────────────────────────────┘"
	@echo ""
	@echo "  ┌─ 工具 ──────────────────────────────────────────────────┐"
	@echo "  │  make install-microgen  编译安装 microgen 工具             │"
	@echo "  │  make proto         生成 protobuf Go 代码                 │"
	@echo "  │  make tools         安装 swag / golangci-lint / protoc 插件│"
	@echo "  │  make deps          go mod tidy + download               │"
	@echo "  │  make build         构建项目                               │"
	@echo "  │  make test          运行测试                               │"
	@echo "  │  make coverage      生成覆盖率报告                         │"
	@echo "  │  make lint          静态检查                               │"
	@echo "  │  make clean         清理构建产物                           │"
	@echo "  └───────────────────────────────────────────────────────────┘"
	@echo ""
