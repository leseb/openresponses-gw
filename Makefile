.PHONY: help build test lint clean run

# Variables
BINARY_NAME=openresponses-gw-server
EXTPROC_BINARY_NAME=openresponses-gw-extproc
VERSION?=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Directories
BIN_DIR=bin
CMD_DIR=cmd

# Colors for output
GREEN=\033[0;32m
YELLOW=\033[0;33m
RED=\033[0;31m
NC=\033[0m # No Color

help: ## Show this help message
	@echo "$(GREEN)Open Responses Gateway - Makefile Commands$(NC)"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-30s$(NC) %s\n", $$1, $$2}'

init: ## Initialize project
	@echo "$(GREEN)Initializing project...$(NC)"
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "$(GREEN)✓ Project initialized$(NC)"

build: build-server build-extproc ## Build all binaries
	@echo "$(GREEN)✓ All binaries built$(NC)"

build-server: ## Build HTTP server binary
	@echo "$(GREEN)Building HTTP server...$(NC)"
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) ./$(CMD_DIR)/server
	@echo "$(GREEN)✓ Built $(BIN_DIR)/$(BINARY_NAME)$(NC)"

build-extproc: ## Build Envoy ExtProc binary
	@echo "$(GREEN)Building Envoy ExtProc...$(NC)"
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(EXTPROC_BINARY_NAME) ./$(CMD_DIR)/envoy-extproc
	@echo "$(GREEN)✓ Built $(BIN_DIR)/$(EXTPROC_BINARY_NAME)$(NC)"

test: ## Run unit tests
	@echo "$(GREEN)Running tests...$(NC)"
	$(GOTEST) -v -race ./...

test-coverage: ## Run tests with coverage
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.txt -o coverage.html
	@echo "$(GREEN)✓ Coverage report generated: coverage.html$(NC)"

lint: ## Run linter
	@echo "$(GREEN)Running linter...$(NC)"
	@which golangci-lint > /dev/null || (echo "$(RED)golangci-lint not installed. Run: brew install golangci-lint$(NC)" && exit 1)
	golangci-lint run ./...
	@echo "$(GREEN)✓ Linting passed$(NC)"

fmt: ## Format code
	@echo "$(GREEN)Formatting code...$(NC)"
	$(GOFMT) ./...
	@echo "$(GREEN)✓ Code formatted$(NC)"

vet: ## Run go vet
	@echo "$(GREEN)Running go vet...$(NC)"
	$(GOVET) ./...
	@echo "$(GREEN)✓ Vet passed$(NC)"

clean: ## Clean build artifacts
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	rm -rf $(BIN_DIR)
	rm -f coverage.txt coverage.html
	@echo "$(GREEN)✓ Cleaned$(NC)"

run: build-server ## Build and run server
	@echo "$(GREEN)Starting server...$(NC)"
	./$(BIN_DIR)/$(BINARY_NAME)

gen-openapi: ## Generate OpenAPI spec from Go annotations
	@echo "$(GREEN)Generating OpenAPI spec...$(NC)"
	@which swag > /dev/null || (echo "$(RED)swag not installed. Run: make install-swag$(NC)" && exit 1)
	swag init --v3.1 --generalInfo cmd/server/main.go --dir ./ --output docs --outputTypes yaml --parseDependency --parseInternal
	@mv docs/swagger.yaml docs/openapi.yaml
	@echo "$(GREEN)Post-processing nullable fields...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	uv run --with pyyaml python scripts/fix-openapi-nullable.py docs/openapi.yaml
	@echo "$(GREEN)✓ Generated docs/openapi.yaml$(NC)"

install-swag: ## Install swag OpenAPI generator
	@echo "$(GREEN)Installing swag v2...$(NC)"
	go install github.com/swaggo/swag/v2/cmd/swag@latest
	@echo "$(GREEN)✓ swag installed$(NC)"

run-dev: ## Run server in development mode with live reload
	@echo "$(GREEN)Starting server in dev mode...$(NC)"
	@which air > /dev/null || (echo "$(YELLOW)air not installed. Run: go install github.com/air-verse/air@latest$(NC)")
	air

# Pre-commit

pre-commit: ## Run all pre-commit checks
	@echo "$(GREEN)Running pre-commit checks...$(NC)"
	@which pre-commit > /dev/null || (echo "$(RED)pre-commit not installed. Run: pip install pre-commit$(NC)" && exit 1)
	pre-commit run --all-files
	@echo "$(GREEN)✓ Pre-commit checks passed$(NC)"

pre-commit-install: ## Install pre-commit hooks
	@echo "$(GREEN)Installing pre-commit hooks...$(NC)"
	@which pre-commit > /dev/null || (echo "$(RED)pre-commit not installed. Run: pip install pre-commit$(NC)" && exit 1)
	pre-commit install
	@echo "$(GREEN)✓ Pre-commit hooks installed$(NC)"

# Testing

test-conformance: ## Run conformance tests (assumes server is already running)
	@echo "$(GREEN)Running conformance tests...$(NC)"
	@echo "$(YELLOW)Note: Server must be running on port 8080$(NC)"
	@echo "$(YELLOW)Use 'make test-conformance-auto' to start server automatically$(NC)"
	./tests/scripts/test-conformance.sh

test-conformance-auto: build-server ## Run conformance tests (starts server automatically)
	@echo "$(GREEN)Running conformance tests with auto-started server...$(NC)"
	./tests/scripts/test-conformance-with-server.sh

test-conformance-custom: build-server ## Run conformance tests with custom model
	@echo "$(GREEN)Running conformance tests with custom parameters...$(NC)"
	@MODEL="${MODEL:-ollama/gpt-oss:20b}"; \
	PORT="${PORT:-8080}"; \
	API_KEY="${API_KEY:-none}"; \
	./tests/scripts/test-conformance-with-server.sh "$$MODEL" "$$PORT" "$$API_KEY"

test-conformance-envoy: build-extproc ## Run conformance tests through Envoy ExtProc
	@echo "$(GREEN)Running conformance tests through Envoy...$(NC)"
	@which envoy > /dev/null || (echo "$(RED)envoy not installed. See: https://www.envoyproxy.io/docs/envoy/latest/start/install$(NC)" && exit 1)
	@MODEL="${MODEL:-ollama/gpt-oss:20b}"; \
	API_KEY="${API_KEY:-none}"; \
	./tests/scripts/test-conformance-with-envoy.sh "$$MODEL" "$$API_KEY"

test-integration-python: ## Run Python integration tests
	@echo "$(GREEN)Running Python integration tests...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	uv run --project tests/integration pytest tests/integration/ -v

test-integration-envoy: ## Run Python integration tests through Envoy ExtProc
	@echo "$(GREEN)Running Python integration tests (Envoy ExtProc)...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	OPENRESPONSES_BASE_URL=http://localhost:8081/v1 \
	OPENRESPONSES_ADAPTER=envoy \
	uv run --project tests/integration pytest tests/integration/ -v

vllm-field-tracking: ## Show and save vLLM vs gateway field tracking for /v1/responses
	@echo "$(GREEN)Running vLLM field tracking...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	uv run --with pyyaml ./scripts/vllm/vllm_field_tracking.py --update

test-openapi-conformance: ## Check OpenAPI conformance against OpenAI spec
	@echo "$(GREEN)Checking OpenAPI conformance...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	uv run --with pyyaml ./scripts/conformance/openapi_conformance.py

test-openapi-conformance-verbose: ## Check OpenAPI conformance with verbose output
	@echo "$(GREEN)Checking OpenAPI conformance (verbose)...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	uv run --with pyyaml ./scripts/conformance/openapi_conformance.py --verbose

test-openapi-conformance-json: ## Check OpenAPI conformance and save JSON report
	@echo "$(GREEN)Checking OpenAPI conformance (saving to JSON)...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	uv run --with pyyaml ./scripts/conformance/openapi_conformance.py --update

.DEFAULT_GOAL := help
