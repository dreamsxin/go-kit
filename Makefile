.PHONY: all build test lint clean help \
        install-microgen \
        gen gen-http gen-grpc gen-full gen-from-db \
        run-demo swag-demo \
        proto \
        swag \
        coverage \
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

# 数据库 DSN（gen-from-db 使用）
DB_DSN      ?= app.db

# 数据库名（MySQL/SQLServer 的 gen-from-db 使用）
DB_NAME     ?=

# 服务名（gen-from-db 使用）
SERVICE     ?= AppService

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
		-driver $(DB_DRIVER) \
		-swag
	@echo ">>> Done. Next steps:"
	@echo "    cd $(OUT) && go mod tidy"
	@echo "    go run ./cmd/main.go -http.addr :8080"

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
		-driver $(DB_DRIVER) \
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
		-driver $(DB_DRIVER) \
		-swag \
		-tests

## gen-from-db: 从数据库生成 RESTful 服务（需设置 DB_DRIVER / DB_DSN / DB_NAME / SERVICE）
##   示例（MySQL）:
##     make gen-from-db DB_DRIVER=mysql DB_DSN="root:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True" DB_NAME=mydb SERVICE=MyApp IMPORT=github.com/myorg/myapp OUT=./gen
##   示例（SQLite）:
##     make gen-from-db DB_DRIVER=sqlite DB_DSN=app.db SERVICE=MyApp IMPORT=github.com/myorg/myapp OUT=./gen
gen-from-db: install-microgen
	@echo ">>> Generating service from database [driver=$(DB_DRIVER)] → $(OUT)"
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

# ──────────────────────────────────────────────────────────────────────────────
# 运行示例（generated-usersvc）
# ──────────────────────────────────────────────────────────────────────────────

## run-demo: 运行生成的示例服务（默认使用 OUT 目录）
run-demo:
	@if [ ! -d "$(OUT)" ]; then \
		echo "Error: output directory '$(OUT)' not found. Run 'make gen' first."; \
		exit 1; \
	fi
	@echo ">>> Starting demo service on $(HTTP_PORT)..."
	@cd $(OUT) && go run ./cmd/main.go -http.addr $(HTTP_PORT)

## swag-demo: 为生成的服务重新生成 Swagger 文档
swag-demo:
	@if [ ! -d "$(OUT)" ]; then \
		echo "Error: output directory '$(OUT)' not found. Run 'make gen' first."; \
		exit 1; \
	fi
	@echo ">>> Running swag init for $(OUT)..."
	@cd $(OUT) && swag init -g cmd/main.go -o docs
	@echo ">>> Done. Start with: make run-demo"

# ──────────────────────────────────────────────────────────────────────────────
# gRPC protobuf 代码生成
# ──────────────────────────────────────────────────────────────────────────────

## proto: 从指定 .proto 文件生成 pb.go（需安装 protoc）
## 用法:
##   make proto PROTO_DIR=./generated/pb/userservice
##   或手动执行生成时打印的 protoc 提示命令
## 注意: Windows 下建议直接复制 microgen 输出的 protoc 命令手动执行
PROTO_DIR ?= $(OUT)/pb

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
	@echo "  │  make run-demo      启动生成的服务（$(OUT)，端口 $(HTTP_PORT)） │"
	@echo "  │  make swag-demo     重新生成 Swagger 文档（$(OUT)）       │"
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
