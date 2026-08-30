APP_VERSION := $(shell tr -d '\r\n' < VERSION)
BUILD_COMMIT := $(shell git rev-parse --short=12 HEAD 2>/dev/null || printf unknown)

.PHONY: build dev test version release-check docker-up docker-down docker-logs

build:
	cd frontend && npm run build
	go build -trimpath -ldflags="-X main.appVersion=$(APP_VERSION) -X main.buildCommit=$(BUILD_COMMIT)" -o classorbit ./backend

dev:
	PUBLIC_DIR=frontend/dist go run ./backend

test:
	cd frontend && npm test
	cd frontend && npm run build
	go test ./backend

version:
	@printf 'ClassOrbit %s (%s)\n' '$(APP_VERSION)' '$(BUILD_COMMIT)'

release-check:
	@sh scripts/release-check.sh

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f classorbit
