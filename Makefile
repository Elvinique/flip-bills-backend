.PHONY: run build test tidy docker-up docker-down migrate lint

## run: Start the API locally (requires running DB + Redis)
run:
	go run ./cmd/api

## build: Compile the binary
build:
	go build -o bin/server ./cmd/api

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

## migrate: Apply SQL migrations (requires migrate CLI)
migrate:
	migrate -path ./migrations -database "postgres://flipbills:supersecret@localhost:5432/flipbills_db?sslmode=disable" up

## lint: Run golangci-lint
lint:
	golangci-lint run ./...
