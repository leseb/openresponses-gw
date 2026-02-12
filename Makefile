.PHONY: help build test lint clean run docker-build docker-run migrate-up migrate-down

# Variables
BINARY_NAME=openresponses-gw-server
EXTPROC_BINARY_NAME=openresponses-gw-extproc
VERSION?=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Go parameters
GOCMD=go
GOBIN=$(shell go env GOPATH)/bin
SWAG=$(GOBIN)/swag
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Directories
BIN_DIR=bin
CMD_DIR=cmd
PKG_DIR=pkg

# Colors for output
GREEN=\033[0;32m
YELLOW=\033[0;33m
RED=\033[0;31m
NC=\033[0m # No Color

help: ## Show this help message
	@echo "$(GREEN)Open Responses Gateway - Makefile Commands$(NC)"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-20s$(NC) %s\n", $$1, $$2}'

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
	@if [ -d ./$(CMD_DIR)/envoy-extproc ]; then \
		$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(EXTPROC_BINARY_NAME) ./$(CMD_DIR)/envoy-extproc; \
		echo "$(GREEN)✓ Built $(BIN_DIR)/$(EXTPROC_BINARY_NAME)$(NC)"; \
	else \
		echo "$(YELLOW)⚠ Skipping envoy-extproc (will be added in Phase 6)$(NC)"; \
	fi

test: ## Run unit tests
	@echo "$(GREEN)Running tests...$(NC)"
	$(GOTEST) -v -race ./...

test-coverage: ## Run tests with coverage
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.txt -o coverage.html
	@echo "$(GREEN)✓ Coverage report generated: coverage.html$(NC)"

test-integration: ## Run integration tests
	@echo "$(GREEN)Running integration tests...$(NC)"
	$(GOTEST) -v -race -tags=integration ./tests/integration/...

test-e2e: ## Run end-to-end tests
	@echo "$(GREEN)Running E2E tests...$(NC)"
	$(GOTEST) -v -tags=e2e ./tests/e2e/...

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

run-dev: ## Run server in development mode with live reload
	@echo "$(GREEN)Starting server in dev mode...$(NC)"
	@which air > /dev/null || (echo "$(YELLOW)air not installed. Run: go install github.com/air-verse/air@latest$(NC)")
	air

docker-build: ## Build Docker image
	@echo "$(GREEN)Building Docker image...$(NC)"
	docker build -t openresponses-gw:$(VERSION) -f deployments/docker/Dockerfile .
	docker tag openresponses-gw:$(VERSION) openresponses-gw:latest
	@echo "$(GREEN)✓ Docker image built$(NC)"

docker-run: ## Run Docker container
	@echo "$(GREEN)Running Docker container...$(NC)"
	docker run -p 8080:8080 --env-file .env openresponses-gw:latest

docker-compose-up: ## Start docker-compose stack
	@echo "$(GREEN)Starting docker-compose stack...$(NC)"
	docker-compose up -d
	@echo "$(GREEN)✓ Stack started$(NC)"

docker-compose-down: ## Stop docker-compose stack
	@echo "$(YELLOW)Stopping docker-compose stack...$(NC)"
	docker-compose down
	@echo "$(GREEN)✓ Stack stopped$(NC)"

docker-compose-logs: ## Show docker-compose logs
	docker-compose logs -f

migrate-create: ## Create a new migration (usage: make migrate-create NAME=add_users)
	@if [ -z "$(NAME)" ]; then echo "$(RED)Error: NAME is required. Usage: make migrate-create NAME=add_users$(NC)"; exit 1; fi
	@echo "$(GREEN)Creating migration: $(NAME)$(NC)"
	@mkdir -p pkg/storage/postgres/migrations
	@timestamp=$$(date +%s); \
	touch pkg/storage/postgres/migrations/$${timestamp}_$(NAME).up.sql; \
	touch pkg/storage/postgres/migrations/$${timestamp}_$(NAME).down.sql; \
	echo "$(GREEN)✓ Created migration files:$(NC)"; \
	echo "  pkg/storage/postgres/migrations/$${timestamp}_$(NAME).up.sql"; \
	echo "  pkg/storage/postgres/migrations/$${timestamp}_$(NAME).down.sql"

migrate-up: ## Run database migrations up
	@echo "$(GREEN)Running migrations up...$(NC)"
	@which migrate > /dev/null || (echo "$(RED)golang-migrate not installed. Run: brew install golang-migrate$(NC)" && exit 1)
	migrate -path pkg/storage/postgres/migrations -database "postgresql://postgres:postgres@localhost:5432/openresponses_gw?sslmode=disable" up
	@echo "$(GREEN)✓ Migrations applied$(NC)"

migrate-down: ## Run database migrations down
	@echo "$(YELLOW)Rolling back migrations...$(NC)"
	migrate -path pkg/storage/postgres/migrations -database "postgresql://postgres:postgres@localhost:5432/openresponses_gw?sslmode=disable" down 1
	@echo "$(GREEN)✓ Migration rolled back$(NC)"

deps: ## Download dependencies
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	$(GOGET) -v ./...
	$(GOMOD) tidy
	@echo "$(GREEN)✓ Dependencies downloaded$(NC)"

tools: install-swag ## Install development tools
	@echo "$(GREEN)Installing development tools...$(NC)"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/air-verse/air@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@echo "$(GREEN)✓ Tools installed$(NC)"

install-swag: ## Install swag CLI for OpenAPI generation
	@echo "$(GREEN)Installing swag v2...$(NC)"
	go install github.com/swaggo/swag/v2/cmd/swag@latest
	@echo "$(GREEN)✓ swag installed$(NC)"

gen-openapi: ## Generate OpenAPI spec from Go annotations
	@test -x $(SWAG) || $(MAKE) install-swag
	$(SWAG) init --v3.1 --generalInfo cmd/server/main.go --dir ./ --output docs --outputTypes yaml --parseDependency --parseInternal
	@cp docs/swagger.yaml openapi.yaml
	@echo "$(GREEN)✓ OpenAPI spec generated$(NC)"

check-openapi: ## Check generated OpenAPI spec is up to date
	@echo "$(GREEN)Checking OpenAPI spec is up to date...$(NC)"
	@test -x $(SWAG) || $(MAKE) install-swag
	$(SWAG) init --v3.1 --generalInfo cmd/server/main.go --dir ./ --output docs --outputTypes yaml --parseDependency --parseInternal
	@cp docs/swagger.yaml openapi.yaml
	@git diff --exit-code openapi.yaml docs/swagger.yaml || (echo "$(RED)OpenAPI spec is out of date. Run 'make gen-openapi' and commit.$(NC)" && exit 1)
	@echo "$(GREEN)✓ OpenAPI spec is up to date$(NC)"

# Pre-commit and Conformance Testing

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

test-conformance: ## Run conformance tests (assumes server is already running)
	@echo "$(GREEN)Running conformance tests...$(NC)"
	@echo "$(YELLOW)Note: Server must be running on port 8080$(NC)"
	@echo "$(YELLOW)Use 'make test-conformance-auto' to start server automatically$(NC)"
	./tests/scripts/test-conformance.sh

test-conformance-auto: build-server ## Run conformance tests (starts server automatically)
	@echo "$(GREEN)Running conformance tests with auto-started server...$(NC)"
	./tests/scripts/test-conformance-with-server.sh

test-conformance-custom: build-server ## Run conformance tests with custom model (MODEL=... make test-conformance-custom)
	@echo "$(GREEN)Running conformance tests with custom parameters...$(NC)"
	@MODEL="${MODEL:-ollama/gpt-oss:20b}"; \
	PORT="${PORT:-8080}"; \
	API_KEY="${API_KEY:-none}"; \
	./tests/scripts/test-conformance-with-server.sh "$$MODEL" "$$PORT" "$$API_KEY"

test-integration-python: ## Run Python integration tests
	@echo "$(GREEN)Running Python integration tests...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	uv run pytest tests/integration/ -v

test-openapi-conformance: ## Check OpenAPI conformance against OpenAI spec
	@echo "$(GREEN)Checking OpenAPI conformance...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	uv run --with pyyaml ./scripts/openapi_conformance.py

test-openapi-conformance-verbose: ## Check OpenAPI conformance with verbose output
	@echo "$(GREEN)Checking OpenAPI conformance (verbose)...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	uv run --with pyyaml ./scripts/openapi_conformance.py --verbose

test-openapi-conformance-json: ## Check OpenAPI conformance and save JSON report
	@echo "$(GREEN)Checking OpenAPI conformance (saving to JSON)...$(NC)"
	@which uv > /dev/null || (echo "$(RED)uv not installed. Run: brew install uv$(NC)" && exit 1)
	uv run --with pyyaml ./scripts/openapi_conformance.py --output conformance-results.json

validate-openapi: check-openapi ## Validate OpenAPI spec consistency

.DEFAULT_GOAL := help
