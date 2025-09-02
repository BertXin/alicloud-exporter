# Makefile for Alicloud Exporter

# 变量定义
APP_NAME := alicloud-exporter
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go 相关变量
GO := go
GOFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildDate=$(BUILD_TIME) -X main.commitSHA=$(GIT_COMMIT)"
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

# 目录定义
BIN_DIR := bin
CMD_DIR := cmd/alicloud-exporter
COVER_DIR := coverage

# 默认目标
.PHONY: all
all: clean deps lint test build

# 帮助信息
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build      - Build the binary"
	@echo "  test       - Run tests"
	@echo "  lint       - Run linter"
	@echo "  clean      - Clean build artifacts"
	@echo "  deps       - Download dependencies"
	@echo "  fmt        - Format code"
	@echo "  vet        - Run go vet"
	@echo "  coverage   - Generate test coverage report"
	@echo "  docker     - Build docker image"
	@echo "  install    - Install binary to GOPATH/bin"
	@echo "  run        - Run the application"
	@echo "  dev        - Run in development mode"

# 创建必要的目录
$(BIN_DIR):
	mkdir -p $(BIN_DIR)

$(COVER_DIR):
	mkdir -p $(COVER_DIR)

# 下载依赖
.PHONY: deps
deps:
	$(GO) mod download
	$(GO) mod tidy

# 代码格式化
.PHONY: fmt
fmt:
	$(GO) fmt ./...

# 代码检查
.PHONY: vet
vet:
	$(GO) vet ./...

# 代码规范检查 (需要安装 golangci-lint)
.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, skipping lint check"; \
		echo "Install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# 运行测试
.PHONY: test
test:
	$(GO) test -v -race ./...

# 生成测试覆盖率报告
.PHONY: coverage
coverage: $(COVER_DIR)
	$(GO) test -v -race -coverprofile=$(COVER_DIR)/coverage.out ./...
	$(GO) tool cover -html=$(COVER_DIR)/coverage.out -o $(COVER_DIR)/coverage.html
	@echo "Coverage report generated: $(COVER_DIR)/coverage.html"

# 构建二进制文件
.PHONY: build
build: $(BIN_DIR) deps
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(APP_NAME) $(CMD_DIR)/main.go
	@echo "Binary built: $(BIN_DIR)/$(APP_NAME)"

# 构建多平台二进制文件
.PHONY: build-all
build-all: $(BIN_DIR) deps
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 $(CMD_DIR)/main.go
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(APP_NAME)-linux-arm64 $(CMD_DIR)/main.go
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(APP_NAME)-darwin-amd64 $(CMD_DIR)/main.go
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(APP_NAME)-darwin-arm64 $(CMD_DIR)/main.go
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(APP_NAME)-windows-amd64.exe $(CMD_DIR)/main.go
	@echo "Multi-platform binaries built in $(BIN_DIR)/"

# 安装到 GOPATH/bin
.PHONY: install
install: deps
	$(GO) install $(GOFLAGS) $(CMD_DIR)/main.go

# 运行应用程序
.PHONY: run
run: build
	./$(BIN_DIR)/$(APP_NAME) --config config/config.yaml

# 开发模式运行 (直接运行源码)
.PHONY: dev
dev:
	$(GO) run $(CMD_DIR)/main.go --config config/config.yaml --log-level debug

# 验证配置文件
.PHONY: validate-config
validate-config: build
	./$(BIN_DIR)/$(APP_NAME) validate --config config/config.yaml

# 显示可用指标
.PHONY: show-metrics
show-metrics: build
	./$(BIN_DIR)/$(APP_NAME) metrics

# Docker 相关
.PHONY: docker
docker:
	docker build -t $(APP_NAME):$(VERSION) .
	docker tag $(APP_NAME):$(VERSION) $(APP_NAME):latest
	@echo "Docker image built: $(APP_NAME):$(VERSION)"

# 推送 Docker 镜像 (需要先设置 DOCKER_REGISTRY)
.PHONY: docker-push
docker-push: docker
	@if [ -z "$(DOCKER_REGISTRY)" ]; then \
		echo "Error: DOCKER_REGISTRY not set"; \
		exit 1; \
	fi
	docker tag $(APP_NAME):$(VERSION) $(DOCKER_REGISTRY)/$(APP_NAME):$(VERSION)
	docker tag $(APP_NAME):$(VERSION) $(DOCKER_REGISTRY)/$(APP_NAME):latest
	docker push $(DOCKER_REGISTRY)/$(APP_NAME):$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(APP_NAME):latest

# 清理构建产物
.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
	rm -rf $(COVER_DIR)
	$(GO) clean

# 深度清理 (包括模块缓存)
.PHONY: clean-all
clean-all: clean
	$(GO) clean -modcache

# 检查代码质量
.PHONY: check
check: fmt vet lint test

# 发布准备 (构建所有平台 + 运行所有检查)
.PHONY: release
release: clean check build-all
	@echo "Release artifacts ready in $(BIN_DIR)/"

# 本地开发环境设置
.PHONY: setup-dev
setup-dev:
	@echo "Setting up development environment..."
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GO) install github.com/air-verse/air@latest
	@echo "Development tools installed"

# 使用 air 进行热重载开发
.PHONY: dev-watch
dev-watch:
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "air not found, install it with: make setup-dev"; \
	fi

# 生成版本信息
.PHONY: version
version:
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Go Version: $(shell $(GO) version)"
	@echo "Platform: $(GOOS)/$(GOARCH)"

# 健康检查 (需要应用程序运行)
.PHONY: health-check
health-check:
	@echo "Checking application health..."
	@curl -f http://localhost:9100/health || (echo "Health check failed" && exit 1)
	@echo "Application is healthy"

# 指标检查 (需要应用程序运行)
.PHONY: metrics-check
metrics-check:
	@echo "Checking metrics endpoint..."
	@curl -f http://localhost:9100/metrics > /dev/null || (echo "Metrics check failed" && exit 1)
	@echo "Metrics endpoint is working"

# 完整的 CI/CD 检查
.PHONY: ci
ci: deps fmt vet lint test build
	@echo "CI checks completed successfully"