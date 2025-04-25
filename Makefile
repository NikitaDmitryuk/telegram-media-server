BINARY_NAME=telegram-media-server
BUILD_DIR=build
INSTALL_DIR=/usr/local/bin
CONFIG_DIR=/etc/telegram-media-server
SERVICE_DIR=/usr/lib/systemd/system

DEPENDENCIES=yt-dlp aria2
DEPENDENCY_BINARIES=yt-dlp aria2c
BUILD_DEPENDENCIES=go
OPTIONAL_DEPENDENCIES=minidlna
OPTIONAL_DEPENDENCY_CHECK=minidlna.service

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
	@if ! systemctl list-units --type=service --all | grep -q $(OPTIONAL_DEPENDENCY_CHECK); then \
		echo "Warning: Optional dependency minidlna is not installed or not enabled."; \
	else \
		echo "Optional dependency minidlna is installed and enabled."; \
	fi
	@echo "All required dependencies are installed."

.PHONY: build
build: check-deps
	@echo "Building $(BINARY_NAME)..."
	go build -trimpath -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/telegram-media-server

.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME)..."
	install -Dm755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	install -Dm644 .env.example $(CONFIG_DIR)/.env.example
	install -Dm644 telegram-media-server.service $(SERVICE_DIR)/telegram-media-server.service
	@echo "Installation complete. To start the service, run:"
	@echo "  sudo systemctl enable --now telegram-media-server"

.PHONY: uninstall
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	rm -f $(CONFIG_DIR)/.env.example
	rm -f $(SERVICE_DIR)/telegram-media-server.service
	@echo "Uninstallation complete."

.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)

.PHONY: format
format:
	go fmt ./...
	go mod tidy

.PHONY: lint
lint:
	golangci-lint run

.PHONY: run
run:
	docker compose up --build
