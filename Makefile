.PHONY: build
build:
	cd build && makepkg -Acsf --config makepkg.conf

.PHONY: run
run:
	docker compose up --build

.PHONY: format
format:
	go fmt ./...
	go mod tidy

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: generate-messages
generate-messages:
	./scripts/generate_messages.sh
