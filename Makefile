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
