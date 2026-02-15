.PHONY: build run dev clean test lint

APP_NAME := wkr
CLI_NAME := wkr
BUILD_DIR := ./bin

build:
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server

build-cli:
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(CLI_NAME) ./cmd/cli

build-all: build build-cli

install-cli: build-cli
	cp $(BUILD_DIR)/$(CLI_NAME) $(GOPATH)/bin/$(CLI_NAME) 2>/dev/null || cp $(BUILD_DIR)/$(CLI_NAME) /usr/local/bin/$(CLI_NAME)

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
