# NodelistDB and Testing Daemon Makefile

# Variables
BINARY_NAME_PARSER=parser
BINARY_NAME_SERVER=server
BINARY_NAME_DAEMON=testdaemon
BUILD_DIR=bin
CMD_PARSER=cmd/parser
CMD_SERVER=cmd/server
CMD_DAEMON=cmd/testdaemon
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# Go commands
GO=go
GOFMT=gofmt
GOVET=$(GO) vet
GOTEST=$(GO) test
GOBUILD=$(GO) build
GOCLEAN=$(GO) clean
GOMOD=$(GO) mod

# Cross-compilation toolchains (can be overridden)
ARM64_CC ?= aarch64-linux-gnu-gcc
ARM64_CXX ?= aarch64-linux-gnu-g++

.PHONY: help build clean test run-parser deps build-daemon run-daemon

# Default target
help: ## Show this help message
	@echo 'NodelistDB - Clean DuckDB-only FidoNet Nodelist System'
	@echo ''
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Dependencies
deps: ## Download and tidy Go dependencies
	go mod download
	go mod tidy

# Version information
VERSION_RAW := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
VERSION := $(shell echo "$(VERSION_RAW)" | sed 's/^v//')
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%d %H:%M:%S UTC")

# Build flags
LDFLAGS := -X 'github.com/nodelistdb/internal/version.Version=$(VERSION)' \
           -X 'github.com/nodelistdb/internal/version.GitCommit=$(COMMIT)' \
           -X 'github.com/nodelistdb/internal/version.BuildTime=$(BUILD_TIME)'

# Build targets
build: build-parser build-server build-daemon ## Build all binaries

build-parser: ## Build parser binary
	@echo "Building parser..."
	go build -ldflags "$(LDFLAGS)" -o bin/parser ./cmd/parser
	@echo "✓ Parser built successfully"

build-server: ## Build server binary
	@echo "Building server..."
	go build -ldflags "$(LDFLAGS)" -o bin/server ./cmd/server
	@echo "✓ Server built successfully"

build-daemon: ## Build testing daemon binary
	@echo "Building testing daemon..."
	go build -ldflags "$(LDFLAGS)" -o bin/testdaemon ./cmd/testdaemon
	@echo "✓ Testing daemon built successfully"

# Development targets  
test: ## Run tests
	go test -v ./...

test-coverage: ## Run tests with coverage
	go test -v -cover ./...

run-parser: ## Run parser (requires NODELIST_PATH)
	@if [ -z "$(NODELIST_PATH)" ]; then \
		echo "Error: NODELIST_PATH environment variable is required"; \
		echo "Example: make run-parser NODELIST_PATH='/path/to/nodelists'"; \
		exit 1; \
	fi
	./bin/parser -path "$(NODELIST_PATH)" -db ./nodelist.duckdb -verbose $(ARGS)

run-server: build-server ## Run web server
	./bin/server -db ./nodelist.duckdb -host localhost -port 8080

run-daemon: build-daemon ## Run testing daemon
	./bin/testdaemon -config config.yaml

run-daemon-debug: build-daemon ## Run testing daemon in debug mode
	./bin/testdaemon -config config.yaml -debug

run-daemon-once: build-daemon ## Run testing daemon single cycle
	./bin/testdaemon -config config.yaml -once

# Cross-compilation for deployment
build-linux: build-linux-amd64 build-linux-arm64 ## Build for Linux (both x64 and ARM64)

build-linux-amd64: ## Build for Linux x64/AMD64
	@echo "Building for Linux x64/AMD64..."
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/parser-linux-amd64 ./cmd/parser
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/server-linux-amd64 ./cmd/server
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/testdaemon-linux-amd64 ./cmd/testdaemon
	@echo "✓ Linux AMD64 build complete"

build-linux-arm64: ## Build for Linux ARM64
	@echo "Building for Linux ARM64..."
	@if ! command -v $(ARM64_CC) >/dev/null 2>&1; then \
		echo "Error: $(ARM64_CC) not found. Install gcc-aarch64-linux-gnu or set ARM64_CC to your ARM64 compiler."; \
		exit 1; \
	fi
	@if ! command -v $(ARM64_CXX) >/dev/null 2>&1; then \
		echo "Error: $(ARM64_CXX) not found. Install g++-aarch64-linux-gnu or set ARM64_CXX to your ARM64 C++ compiler."; \
		exit 1; \
	fi
	CC=$(ARM64_CC) CXX=$(ARM64_CXX) CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/parser-linux-arm64 ./cmd/parser
	CC=$(ARM64_CC) CXX=$(ARM64_CXX) CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/server-linux-arm64 ./cmd/server
	CC=$(ARM64_CC) CXX=$(ARM64_CXX) CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/testdaemon-linux-arm64 ./cmd/testdaemon
	@echo "✓ Linux ARM64 build complete"

# Individual cross-compilation targets
build-parser-linux-amd64: ## Build parser for Linux AMD64
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/parser-linux-amd64 ./cmd/parser

build-parser-linux-arm64: ## Build parser for Linux ARM64
	CC=$(ARM64_CC) CXX=$(ARM64_CXX) CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/parser-linux-arm64 ./cmd/parser

build-server-linux-amd64: ## Build server for Linux AMD64
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/server-linux-amd64 ./cmd/server

build-server-linux-arm64: ## Build server for Linux ARM64
	CC=$(ARM64_CC) CXX=$(ARM64_CXX) CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/server-linux-arm64 ./cmd/server

build-daemon-linux-amd64: ## Build daemon for Linux AMD64
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/testdaemon-linux-amd64 ./cmd/testdaemon

build-daemon-linux-arm64: ## Build daemon for Linux ARM64
	CC=$(ARM64_CC) CXX=$(ARM64_CXX) CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/testdaemon-linux-arm64 ./cmd/testdaemon

# Clean targets
clean: ## Clean build artifacts
	rm -rf bin/
	rm -f *.duckdb
	rm -f *.db

# Development database
init-db: build-parser ## Initialize development database
	./bin/parser -path ./test_data -db ./dev.duckdb -verbose || true
	@echo "✓ Development database initialized"

# Docker targets (future)
docker-build: ## Build Docker image
	@echo "Docker build not implemented yet"

# Format code
fmt: ## Format Go code
	go fmt ./...

# Lint code  
lint: ## Run linter
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Check Go modules
mod-check: ## Check Go modules
	go mod verify
	go mod tidy -v
