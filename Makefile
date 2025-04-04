.PHONY: build
build:
	makepkg -Acsf --config build/makepkg.conf

.PHONY: run
run:
	docker compose up --build

.PHONY: format
format:
	go fmt ./...
	go mod tidy

.PHONY: lint
lint:
	golangci-lint run

.PHONY: gm
gm:
	./scripts/generate_messages.sh
