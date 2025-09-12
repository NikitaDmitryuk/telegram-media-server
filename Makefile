BINARY_NAME=telegram-media-server
BUILD_DIR=build
VERSION=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

# Core build commands
.PHONY: build
build:
	@echo "Building $(BINARY_NAME) version $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/telegram-media-server

.PHONY: build-simple
build-simple:
	@echo "Building $(BINARY_NAME) for CI..."
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/telegram-media-server

.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) $(BINARY_NAME)
	go clean -cache -testcache

# Code quality
.PHONY: format
format:
	@echo "Running code formatter..."
	golines --max-len=140 -w .
	gofmt -s -w .
	go mod tidy

.PHONY: lint
lint:
	@echo "Running linter..."
	golangci-lint run

.PHONY: vet
vet:
	@echo "Running go vet..."
	go vet ./...

.PHONY: security-check
security-check:
	@echo "Running security checks..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec -fmt=json -out=gosec-report.json ./...; \
	else \
		echo "gosec not found, installing..."; \
		go install github.com/cosmos/gosec/v2/cmd/gosec@latest && \
		$(shell go env GOPATH)/bin/gosec -fmt=json -out=gosec-report.json ./...; \
	fi

# Testing
.PHONY: test
test:
	@echo "Running tests..."
	go test -v ./...

.PHONY: test-unit
test-unit:
	@echo "Running unit tests..."
	go test -v -short ./...

.PHONY: test-integration
test-integration:
	@echo "Running integration tests..."
	go test -v -run Integration ./...

.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out

# Specific module tests (used by CI)
.PHONY: test-config
test-config:
	@echo "Running config tests..."
	go test -v ./internal/config/...

.PHONY: test-torrent
test-torrent:
	@echo "Running torrent tests..."
	go test -v ./internal/downloader/torrent/...

.PHONY: test-video
test-video:
	@echo "Running video tests..."
	go test -v ./internal/downloader/video/...

# Docker commands
.PHONY: run
run:
	@echo "Running with Docker Compose..."
	docker-compose up --build

.PHONY: stop
stop:
	@echo "Stopping Docker Compose services..."
	docker-compose down

# Docker test commands
.PHONY: docker-test-build
docker-test-build:
	@echo "Building Docker test image..."
	docker build -f Dockerfile.test -t telegram-media-server:test .

.PHONY: test-integration-docker
test-integration-docker: docker-test-build
	@echo "Running integration tests in Docker..."
	docker run --rm \
		-v $(PWD):/workspace \
		-w /workspace \
		telegram-media-server:test \
		go test -v -tags=integration ./internal/downloader/...

# Utility commands
.PHONY: check
check: lint vet test-unit
	@echo "All checks passed!"

.PHONY: pre-commit
pre-commit: format check
	@echo "Pre-commit checks completed successfully!"

.PHONY: help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build:"
	@echo "  build          - Build the application"
	@echo "  build-simple   - Build for CI (no dependencies check)"
	@echo "  clean          - Clean build artifacts"
	@echo ""
	@echo "Code Quality:"
	@echo "  format         - Format code and fix issues"
	@echo "  lint           - Run linter"
	@echo "  vet            - Run go vet"
	@echo ""
	@echo "Testing:"
	@echo "  test           - Run all tests"
	@echo "  test-integration-docker - Run integration tests in Docker"
	@echo "  test-unit      - Run unit tests only (fast)"
	@echo "  test-integration - Run integration tests (slow)"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  test-config    - Run config tests"
	@echo "  test-torrent   - Run torrent tests"
	@echo "  test-video     - Run video tests"
	@echo ""
	@echo "Docker:"
	@echo "  run            - Run with Docker Compose"
	@echo "  stop           - Stop Docker Compose services"
	@echo ""
	@echo "Utility:"
	@echo "  check          - Run all checks (lint + vet + test-unit)"
	@echo "  pre-commit     - Run pre-commit checks (format + check)"
	@echo "  help           - Show this help"