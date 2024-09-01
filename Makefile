.PHONY: build
build:
	makepkg -f

.PHONY: build_arm
build_arm:
	CARCH=armv7h makepkg -f

.PHONY: run
run:
	docker compose up --build

.PHONY: format
format:
	go fmt .
