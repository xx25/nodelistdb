# NodelistDB Makefile
# Clean DuckDB-only FidoNet Nodelist System

.PHONY: help build clean test run-parser deps

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
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%d %H:%M:%S UTC")

# Build flags
LDFLAGS := -X 'nodelistdb/internal/version.Version=$(VERSION)' \
           -X 'nodelistdb/internal/version.GitCommit=$(COMMIT)' \
           -X 'nodelistdb/internal/version.BuildTime=$(BUILD_TIME)'

# Build targets
build: build-parser build-server ## Build all binaries

build-parser: ## Build parser binary
	@echo "Building parser..."
	go build -ldflags "$(LDFLAGS)" -o bin/parser ./cmd/parser
	@echo "✓ Parser built successfully"

build-server: ## Build server binary
	@echo "Building server..."
	go build -ldflags "$(LDFLAGS)" -o bin/server ./cmd/server
	@echo "✓ Server built successfully"

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

# Cross-compilation for deployment
build-linux: ## Build for Linux x64
	@echo "Building for Linux x64..."
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o bin/parser-linux ./cmd/parser
	@echo "✓ Linux build complete"

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