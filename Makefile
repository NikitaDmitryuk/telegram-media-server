
.PHONY: build
build:
	makepkg -f

.PHONY: run
run:
	docker compose up --build

.PHONY: format
format:
	go fmt .
