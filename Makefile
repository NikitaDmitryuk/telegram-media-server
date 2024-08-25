
.PHONY: build
build:
	CARCH=armv7h makepkg -f

.PHONY: run
run:
	docker compose up --build

.PHONY: format
format:
	go fmt .
