.PHONY: build run dev clean test lint

APP_NAME := wkr
BUILD_DIR := ./bin

build:
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server

run: build
	$(BUILD_DIR)/$(APP_NAME)

dev:
	go run ./cmd/server

clean:
	rm -rf $(BUILD_DIR)

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

migrate:
	go run ./cmd/server --migrate-only

docker-build:
	docker build -t $(APP_NAME):latest .

docker-run:
	docker compose up -d
