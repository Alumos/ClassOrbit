.PHONY: build dev test docker-up docker-down docker-logs

build:
	cd frontend && npm run build
	go build -trimpath -o classorbit ./backend

dev:
	PUBLIC_DIR=frontend/dist go run ./backend

test:
	cd frontend && npm test
	cd frontend && npm run build
	go test ./backend

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f classorbit
