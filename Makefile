BINARY_NAME=telegram-media-server
BUILD_DIR=build
INSTALL_DIR=/usr/local/bin
CONFIG_DIR=/etc/telegram-media-server
SERVICE_DIR=/usr/lib/systemd/system
LOCALES_SRC=locales
LOCALES_DEST=/usr/local/share/telegram-media-server/locales

DEPENDENCIES=yt-dlp aria2 ffmpeg
DEPENDENCY_BINARIES=yt-dlp aria2c ffmpeg
BUILD_DEPENDENCIES=go
OPTIONAL_DEPENDENCIES=minidlna prowlarr
OPTIONAL_DEPENDENCY_CHECK="minidlna.service prowlarr.service"

# Version information
VERSION=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

.PHONY: check-deps
check-deps:
	@echo "Checking build dependencies..."
	@for dep in $(BUILD_DEPENDENCIES); do \
		if ! command -v $$dep >/dev/null 2>&1; then \
			echo "Error: Build dependency $$dep is not installed. Please install it."; \
			exit 1; \
		fi; \
	done
	@echo "Checking runtime dependencies..."
	@i=0; \
	for dep in $(DEPENDENCIES); do \
		binary=$$(echo $(DEPENDENCY_BINARIES) | cut -d' ' -f$$((i+1))); \
		if ! command -v $$binary >/dev/null 2>&1; then \
			echo "Error: $$dep is not installed. Please install it."; \
			exit 1; \
		fi; \
		i=$$((i+1)); \
	done
	@echo "Checking optional dependencies..."
	@for dep in $(OPTIONAL_DEPENDENCY_CHECK); do \
		if ! systemctl list-units --type=service --all | grep -q $$dep; then \
			echo "Warning: Optional dependency $$dep is not installed or not enabled."; \
		else \
			echo "Optional dependency $$dep is installed and enabled."; \
		fi; \
	done
	@echo "All required dependencies are installed."

.PHONY: build
build: check-deps
	@echo "Building $(BINARY_NAME) version $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/telegram-media-server

.PHONY: build-debug
build-debug: check-deps
	@echo "Building $(BINARY_NAME) with debug symbols..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -gcflags="all=-N -l" -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-debug ./cmd/telegram-media-server

.PHONY: build-release
build-release: check-deps
	@echo "Building $(BINARY_NAME) release version $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux go build $(LDFLAGS) -a -installsuffix cgo -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-release ./cmd/telegram-media-server

.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME)..."
	install -Dm755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	install -Dm644 .env.example $(CONFIG_DIR)/.env.example
	install -Dm644 telegram-media-server.service $(SERVICE_DIR)/telegram-media-server.service
	install -d $(LOCALES_DEST)
	install -Dm644 $(LOCALES_SRC)/* $(LOCALES_DEST)/
	systemctl daemon-reload
	systemctl enable --now telegram-media-server
	systemctl restart telegram-media-server
	@echo "Installation complete"
	@echo "Please configure the service by creating a .env file in $(CONFIG_DIR) based on the provided $(CONFIG_DIR)/.env.example and then restarting the service."

.PHONY: uninstall
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	systemctl stop telegram-media-server
	systemctl disable telegram-media-server
	rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	rm -f $(CONFIG_DIR)/.env.example
	rm -f $(SERVICE_DIR)/telegram-media-server.service
	rm -rf $(LOCALES_DEST)
	@echo "Uninstallation complete."

.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	go clean -cache -testcache

.PHONY: format
format:
	@echo "Formatting code..."
	go fmt ./...
	go mod tidy
	golines --max-len=140 -w .
	golangci-lint run --fix
	gocritic check .

.PHONY: lint
lint:
	@echo "Running linter..."
	golines --max-len=140 -w .
	golangci-lint run
	gocritic check .

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

.PHONY: test-torrent
test-torrent:
	@echo "Running torrent tests..."
	go test -v ./internal/downloader/torrent/...

.PHONY: test-video
test-video:
	@echo "Running video tests..."
	go test -v ./internal/downloader/video/...

.PHONY: test-config
test-config:
	@echo "Running config tests..."
	go test -v ./internal/config/...

.PHONY: test-database
test-database:
	@echo "Running database tests..."
	go test -v ./internal/database/...

.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out
	@echo "Coverage report generated: coverage.html"

.PHONY: test-coverage-detailed
test-coverage-detailed:
	@echo "Running tests with detailed coverage..."
	go test -v -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | grep -E "(total|TOTAL)"
	@echo "Detailed coverage report generated: coverage.html"

.PHONY: test-benchmark
test-benchmark:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

.PHONY: vet
vet:
	@echo "Running go vet..."
	go vet ./...

.PHONY: security-check
security-check:
	@echo "Running security checks..."
	gosec ./...

.PHONY: run
run:
	@echo "Running with Docker Compose..."
	docker-compose up --build

.PHONY: run-detached
run-detached:
	@echo "Running with Docker Compose in detached mode..."
	docker-compose up --build -d

.PHONY: stop
stop:
	@echo "Stopping Docker Compose services..."
	docker-compose down

.PHONY: logs
logs:
	@echo "Showing Docker Compose logs..."
	docker-compose logs -f

.PHONY: status
status:
	@echo "Checking service status..."
	@if systemctl is-active --quiet telegram-media-server; then \
		echo "✓ telegram-media-server is running"; \
	else \
		echo "✗ telegram-media-server is not running"; \
	fi

.PHONY: restart
restart:
	@echo "Restarting telegram-media-server..."
	systemctl restart telegram-media-server

.PHONY: check
check: lint vet test
	@echo "All checks passed!"

.PHONY: pre-commit
pre-commit: format lint vet test
	@echo "Pre-commit checks completed successfully!"

.PHONY: release
release: clean build-release
	@echo "Release build completed: $(BUILD_DIR)/$(BINARY_NAME)-release"

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build          - Build the application"
	@echo "  build-debug    - Build with debug symbols"
	@echo "  build-release  - Build release version"
	@echo "  install        - Install as system service"
	@echo "  uninstall      - Uninstall system service"
	@echo "  clean          - Clean build artifacts"
	@echo "  format         - Format code"
	@echo "  lint           - Run linter"
	@echo "  test           - Run all tests"
	@echo "  test-unit      - Run unit tests only"
	@echo "  test-integration - Run integration tests only"
	@echo "  test-torrent   - Run torrent-specific tests"
	@echo "  test-video     - Run video-specific tests"
	@echo "  test-config    - Run config tests"
	@echo "  test-database  - Run database tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  test-coverage-detailed - Run tests with detailed coverage"
	@echo "  test-benchmark - Run benchmarks"
	@echo "  vet            - Run go vet"
	@echo "  security-check - Run security checks"
	@echo "  run            - Run with Docker Compose"
	@echo "  run-detached   - Run with Docker Compose (detached)"
	@echo "  stop           - Stop Docker Compose services"
	@echo "  logs           - Show Docker Compose logs"
	@echo "  status         - Check service status"
	@echo "  restart        - Restart service"
	@echo "  check          - Run all checks (lint, vet, test)"
	@echo "  pre-commit     - Run pre-commit checks"
	@echo "  release        - Build release version"
	@echo "  help           - Show this help"
	@echo "  devtools       - Установка golines и gocritic для автоформатирования"
