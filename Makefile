.PHONY: help build test lint clean run docker-build docker-run migrate-up migrate-down

# Variables
BINARY_NAME=responses-gateway-server
EXTPROC_BINARY_NAME=responses-gateway-extproc
VERSION?=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Go parameters
GOCMD=go
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
	@echo "$(GREEN)OpenAI Responses API Gateway - Makefile Commands$(NC)"
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
	docker build -t responses-gateway:$(VERSION) -f deployments/docker/Dockerfile .
	docker tag responses-gateway:$(VERSION) responses-gateway:latest
	@echo "$(GREEN)✓ Docker image built$(NC)"

docker-run: ## Run Docker container
	@echo "$(GREEN)Running Docker container...$(NC)"
	docker run -p 8080:8080 --env-file .env responses-gateway:latest

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
	migrate -path pkg/storage/postgres/migrations -database "postgresql://postgres:postgres@localhost:5432/responses_gateway?sslmode=disable" up
	@echo "$(GREEN)✓ Migrations applied$(NC)"

migrate-down: ## Run database migrations down
	@echo "$(YELLOW)Rolling back migrations...$(NC)"
	migrate -path pkg/storage/postgres/migrations -database "postgresql://postgres:postgres@localhost:5432/responses_gateway?sslmode=disable" down 1
	@echo "$(GREEN)✓ Migration rolled back$(NC)"

deps: ## Download dependencies
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	$(GOGET) -v ./...
	$(GOMOD) tidy
	@echo "$(GREEN)✓ Dependencies downloaded$(NC)"

tools: ## Install development tools
	@echo "$(GREEN)Installing development tools...$(NC)"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/air-verse/air@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@echo "$(GREEN)✓ Tools installed$(NC)"

.DEFAULT_GOAL := help
