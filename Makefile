.PHONY: all build test lint clean help

# 默认目标
all: lint test build

# 构建项目
build:
	@echo "Building go-kit..."
	@go build ./...

# 运行测试
test:
	@echo "Running tests..."
	@go test -v ./...

# 代码质量检查
lint:
	@echo "Running linter..."
	@golangci-lint run

# 清理构建文件
clean:
	@echo "Cleaning build artifacts..."
	@go clean
	@rm -f coverage.out

# 生成测试覆盖率报告
coverage:
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html

# 安装依赖
deps:
	@echo "Installing dependencies..."
	@go mod tidy
	@go mod download

# 安装开发工具
tools:
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 显示帮助信息
help:
	@echo "Available targets:"
	@echo "  build     - Build the project"
	@echo "  test      - Run tests"
	@echo "  lint      - Run code quality checks"
	@echo "  coverage  - Generate test coverage report"
	@echo "  clean     - Clean build artifacts"
	@echo "  deps      - Install dependencies"
	@echo "  tools     - Install development tools"
