.PHONY: run build test tidy docker-up docker-down migrate migrate-down lint env-check

## run: Start the API locally (requires running DB + Redis)
run:
	go run ./cmd/api

## build: Compile the binary
build:
	go build -ldflags="-s -w" -o bin/server ./cmd/api

## test: Run all tests with race detector
test:
	go test -race -cover ./...

## tidy: Sync go.mod and go.sum
tidy:
	go mod tidy

## docker-up: Spin up all infrastructure containers
docker-up:
	docker compose up -d --build

## docker-down: Stop and remove containers
docker-down:
	docker compose down

## migrate: Apply all pending SQL migrations (requires migrate CLI)
migrate:
	migrate -path ./migrations -database "$(POSTGRES_URL)" up

## migrate-down: Roll back the last migration
migrate-down:
	migrate -path ./migrations -database "$(POSTGRES_URL)" down 1

## migrate-drop: Wipe the entire schema — DANGER, dev only
migrate-drop:
	migrate -path ./migrations -database "$(POSTGRES_URL)" drop -f

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## env-check: Verify all required env vars are set before running
env-check:
	@test -f .env || (echo "ERROR: .env file not found. Copy .env.example to .env and fill in values." && exit 1)
	@grep -v '^#' .env | grep -v '^$$' | grep '=$$' && echo "WARNING: some env vars are empty" || true

## help: List available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'

# ── Config ────────────────────────────────────────────────────────────────────
# Override via: make migrate POSTGRES_URL="postgres://user:pass@host/db?sslmode=disable"
POSTGRES_URL ?= postgres://flipbills:supersecret@localhost:5432/flipbills_db?sslmode=disable