# Load environment variables from .env file if it exists
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# Load local overrides if they exist
ifneq (,$(wildcard ./.env.local))
    include .env.local
    export
endif

# Build information
BUILD_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "local")
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Application Configuration
APP_NAME := ad-processing-system
GOVERSION := $(shell go version | awk '{print $$3}')
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

# Directories
DIST_DIR := ./dist
BIN_DIR := ./bin
MIGRATIONS_DIR := ./migrations
SCRIPTS_DIR := ./scripts
DOCKER_DIR := ./deployments/docker
K8S_DIR := ./k8s

# Go build flags
LDFLAGS := -X 'main.Version=$(BUILD_VERSION)' \
          -X 'main.BuildCommit=$(BUILD_COMMIT)' \
          -X 'main.BuildTime=$(BUILD_TIME)' \
          -X 'main.GoVersion=$(GOVERSION)'

BUILD_FLAGS := -ldflags "$(LDFLAGS)"

# Docker configuration
DOCKER_REGISTRY ?= localhost:5000
DOCKER_NAMESPACE ?= ad-processing
DOCKER_TAG ?= $(BUILD_VERSION)

# Default target
.DEFAULT_GOAL := help

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
BOLD := \033[1m
NC := \033[0m # No Color

##@ Help
.PHONY: help
help: ## Display available commands
	@echo -e "$(BOLD)$(APP_NAME) Development Commands$(NC)\n"
	@echo -e "$(BLUE)Environment:$(NC)"
	@echo -e "  Go Version: $(GOVERSION)"
	@echo -e "  OS/Arch: $(GOOS)/$(GOARCH)"
	@echo -e "  Build Version: $(BUILD_VERSION)"
	@echo -e "  Build Commit: $(BUILD_COMMIT)\n"
	@awk 'BEGIN {FS = ":.*##"; printf ""} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2 } /^##@/ { printf "\n$(BOLD)%s$(NC)\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Environment Setup
.PHONY: setup
setup: ## Initial project setup
	@echo -e "$(BLUE)Setting up development environment...$(NC)"
	@go mod download
	@go mod tidy
	@$(MAKE) install-tools
	@$(MAKE) create-dirs
	@$(MAKE) db-create
	@echo -e "$(GREEN)✓ Development environment setup complete$(NC)"

.PHONY: install-tools
install-tools: ## Install development tools
	@echo -e "$(BLUE)Installing development tools...$(NC)"
	@go install github.com/pressly/goose/v3/cmd/goose@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/swaggo/swag/cmd/swag@latest
	@go install github.com/golang/mock/mockgen@latest
	@echo -e "$(GREEN)✓ Development tools installed$(NC)"

.PHONY: create-dirs
create-dirs: ## Create necessary directories
	@mkdir -p $(BIN_DIR) $(DIST_DIR) logs test-results

.PHONY: env-check
env-check: ## Check required environment variables
	@echo -e "$(BLUE)Checking environment variables...$(NC)"
	@if [ -z "$(DATABASE_URL)" ]; then echo -e "$(RED)ERROR: DATABASE_URL is not set$(NC)"; exit 1; fi
	@echo -e "$(GREEN)✓ Environment variables OK$(NC)"

##@ Build
.PHONY: build
build: clean ## Build all binaries
	@echo -e "$(BLUE)Building binaries...$(NC)"
	@$(MAKE) build-api
	@$(MAKE) build-processor
	@$(MAKE) build-migrate
	@echo -e "$(GREEN)✓ All binaries built successfully$(NC)"

.PHONY: build-api
build-api: ## Build ad-api binary
	@echo -e "$(BLUE)Building ad-api...$(NC)"
	@go build $(BUILD_FLAGS) -o $(BIN_DIR)/ad-api ./cmd/ad-api

.PHONY: build-processor
build-processor: ## Build ad-processor binary
	@echo -e "$(BLUE)Building ad-processor...$(NC)"
	@go build $(BUILD_FLAGS) -o $(BIN_DIR)/ad-processor ./cmd/ad-processor

.PHONY: build-migrate
build-migrate: ## Build migrate binary
	@echo -e "$(BLUE)Building migrate binary...$(NC)"
	@go build $(BUILD_FLAGS) -o $(BIN_DIR)/migrate ./cmd/migrate

.PHONY: build-race
build-race: ## Build with race detection
	@echo -e "$(BLUE)Building with race detection...$(NC)"
	@go build -race $(BUILD_FLAGS) -o $(BIN_DIR)/ad-api-race ./cmd/ad-api
	@go build -race $(BUILD_FLAGS) -o $(BIN_DIR)/ad-processor-race ./cmd/ad-processor

.PHONY: clean
clean: ## Clean build artifacts
	@echo -e "$(BLUE)Cleaning build artifacts...$(NC)"
	@rm -rf $(BIN_DIR)/* $(DIST_DIR)/*
	@go clean -cache -testcache
	@echo -e "$(GREEN)✓ Clean complete$(NC)"

##@ Database Operations
.PHONY: db-create
db-create: env-check ## Create database if it doesn't exist
	@echo -e "$(BLUE)Creating database if it doesn't exist...$(NC)"
	@docker exec -i $$(docker ps -q -f name=postgres) createdb -U postgres $(APP_DATABASE_NAME) 2>/dev/null || echo "Database already exists or container not found"

.PHONY: db-drop
db-drop: env-check ## Drop database (DANGEROUS - for development only)
	@echo -e "$(YELLOW)WARNING: This will drop the entire database!$(NC)"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		echo; \
		docker exec -i $$(docker ps -q -f name=postgres) dropdb -U postgres $(APP_DATABASE_NAME) --if-exists; \
		echo -e "$(GREEN)✓ Database dropped$(NC)"; \
	else \
		echo; \
		echo -e "$(BLUE)Operation cancelled$(NC)"; \
	fi

.PHONY: db-reset
db-reset: db-drop db-create migrate-up ## Reset database and run migrations

##@ Migration Operations (Goose)
.PHONY: migrate-status
migrate-status: env-check ## Show migration status
	@echo -e "$(BLUE)Migration Status:$(NC)"
	@goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" status

.PHONY: migrate-up
migrate-up: env-check ## Run all pending migrations
	@echo -e "$(BLUE)Running migrations up...$(NC)"
	@goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" up
	@echo -e "$(GREEN)✓ Migrations completed$(NC)"

.PHONY: migrate-up-one
migrate-up-one: env-check ## Run one pending migration
	@echo -e "$(BLUE)Running one migration up...$(NC)"
	@goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" up-one

.PHONY: migrate-down
migrate-down: env-check ## Rollback last migration
	@echo -e "$(YELLOW)Rolling back last migration...$(NC)"
	@goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" down

.PHONY: migrate-down-to
migrate-down-to: env-check ## Rollback to specific version (usage: make migrate-down-to VERSION=001)
	@if [ -z "$(VERSION)" ]; then echo -e "$(RED)ERROR: VERSION is required. Usage: make migrate-down-to VERSION=001$(NC)"; exit 1; fi
	@echo -e "$(YELLOW)Rolling back to version $(VERSION)...$(NC)"
	@goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" down-to $(VERSION)

.PHONY: migrate-redo
migrate-redo: env-check ## Redo last migration (down then up)
	@echo -e "$(BLUE)Redoing last migration...$(NC)"
	@goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" redo

.PHONY: migrate-reset
migrate-reset: env-check ## Reset all migrations (DANGEROUS)
	@echo -e "$(YELLOW)WARNING: This will reset ALL migrations!$(NC)"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		echo; \
		goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" reset; \
		echo -e "$(GREEN)✓ Migrations reset$(NC)"; \
	else \
		echo; \
		echo -e "$(BLUE)Operation cancelled$(NC)"; \
	fi

.PHONY: migrate-version
migrate-version: env-check ## Show current migration version
	@goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" version

.PHONY: migrate-create
migrate-create: ## Create new migration (usage: make migrate-create NAME=add_users_table)
	@if [ -z "$(NAME)" ]; then echo -e "$(RED)ERROR: NAME is required. Usage: make migrate-create NAME=add_users_table$(NC)"; exit 1; fi
	@echo -e "$(BLUE)Creating migration: $(NAME)$(NC)"
	@goose -dir $(MIGRATIONS_DIR) create $(NAME) sql
	@echo -e "$(GREEN)✓ Migration created$(NC)"

.PHONY: migrate-create-go
migrate-create-go: ## Create new Go migration (usage: make migrate-create-go NAME=complex_migration)
	@if [ -z "$(NAME)" ]; then echo -e "$(RED)ERROR: NAME is required. Usage: make migrate-create-go NAME=complex_migration$(NC)"; exit 1; fi
	@echo -e "$(BLUE)Creating Go migration: $(NAME)$(NC)"
	@goose -dir $(MIGRATIONS_DIR) create $(NAME) go
	@echo -e "$(GREEN)✓ Go migration created$(NC)"

.PHONY: migrate-fix
migrate-fix: env-check ## Fix migration sequence numbers
	@echo -e "$(BLUE)Fixing migration sequence...$(NC)"
	@goose -dir $(MIGRATIONS_DIR) fix

##@ Testing
.PHONY: test
test: ## Run all tests
	@echo -e "$(BLUE)Running tests...$(NC)"
	@go test -v -race ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	@echo -e "$(BLUE)Running tests with coverage...$(NC)"
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo -e "$(GREEN)✓ Coverage report generated: coverage.html$(NC)"

.PHONY: test-integration
test-integration: ## Run integration tests
	@echo -e "$(BLUE)Running integration tests...$(NC)"
	@go test -v -tags=integration ./tests/integration/...

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests
	@echo -e "$(BLUE)Running e2e tests...$(NC)"
	@go test -v -tags=e2e ./tests/e2e/...

.PHONY: test-benchmark
test-benchmark: ## Run benchmark tests
	@echo -e "$(BLUE)Running benchmark tests...$(NC)"
	@go test -bench=. -benchmem ./...

##@ Code Quality
.PHONY: lint
lint: ## Run linter
	@echo -e "$(BLUE)Running linter...$(NC)"
	@golangci-lint run

.PHONY: lint-fix
lint-fix: ## Run linter with auto-fix
	@echo -e "$(BLUE)Running linter with auto-fix...$(NC)"
	@golangci-lint run --fix

.PHONY: fmt
fmt: ## Format code
	@echo -e "$(BLUE)Formatting code...$(NC)"
	@go fmt ./...
	@go mod tidy

.PHONY: vet
vet: ## Run go vet
	@echo -e "$(BLUE)Running go vet...$(NC)"
	@go vet ./...

.PHONY: check
check: fmt vet lint test ## Run all code quality checks

##@ Development
.PHONY: dev
dev: ## Start development environment
	@echo -e "$(BLUE)Starting development environment...$(NC)"
	@docker-compose -f docker-compose.dev.yml up -d
	@echo -e "$(GREEN)✓ Development environment started$(NC)"

.PHONY: dev-stop
dev-stop: ## Stop development environment
	@echo -e "$(BLUE)Stopping development environment...$(NC)"
	@docker-compose -f docker-compose.dev.yml down
	@echo -e "$(GREEN)✓ Development environment stopped$(NC)"

.PHONY: dev-logs
dev-logs: ## Show development environment logs
	@docker-compose -f docker-compose.dev.yml logs -f

.PHONY: dev-restart
dev-restart: dev-stop dev ## Restart development environment

.PHONY: run-api
run-api: build-api ## Run ad-api locally
	@echo -e "$(BLUE)Starting ad-api...$(NC)"
	@$(BIN_DIR)/ad-api

.PHONY: run-processor
run-processor: build-processor ## Run ad-processor locally
	@echo -e "$(BLUE)Starting ad-processor...$(NC)"
	@$(BIN_DIR)/ad-processor

.PHONY: run-migrate
run-migrate: build-migrate ## Run custom migrate tool
	@echo -e "$(BLUE)Running custom migrate tool...$(NC)"
	@$(BIN_DIR)/migrate -status

##@ Docker Operations
.PHONY: docker-build
docker-build: ## Build all Docker images
	@echo -e "$(BLUE)Building Docker images...$(NC)"
	@$(MAKE) docker-build-api
	@$(MAKE) docker-build-processor
	@$(MAKE) docker-build-migrate
	@echo -e "$(GREEN)✓ All Docker images built$(NC)"

.PHONY: docker-build-api
docker-build-api: ## Build ad-api Docker image
	@echo -e "$(BLUE)Building ad-api Docker image...$(NC)"
	@docker build -f Dockerfile.ad-api -t $(DOCKER_REGISTRY)/$(DOCKER_NAMESPACE)/ad-api:$(DOCKER_TAG) .

.PHONY: docker-build-processor
docker-build-processor: ## Build ad-processor Docker image
	@echo -e "$(BLUE)Building ad-processor Docker image...$(NC)"
	@docker build -f Dockerfile.ad-processor -t $(DOCKER_REGISTRY)/$(DOCKER_NAMESPACE)/ad-processor:$(DOCKER_TAG) .

.PHONY: docker-build-migrate
docker-build-migrate: ## Build migrate Docker image
	@echo -e "$(BLUE)Building migrate Docker image...$(NC)"
	@docker build -f Dockerfile.migrate -t $(DOCKER_REGISTRY)/$(DOCKER_NAMESPACE)/migrate:$(DOCKER_TAG) .

.PHONY: docker-push
docker-push: docker-build ## Push Docker images to registry
	@echo -e "$(BLUE)Pushing Docker images...$(NC)"
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_NAMESPACE)/ad-api:$(DOCKER_TAG)
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_NAMESPACE)/ad-processor:$(DOCKER_TAG)
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_NAMESPACE)/migrate:$(DOCKER_TAG)
	@echo -e "$(GREEN)✓ Docker images pushed$(NC)"

.PHONY: docker-run
docker-run: ## Run application with Docker Compose
	@echo -e "$(BLUE)Starting application with Docker Compose...$(NC)"
	@docker-compose -f $(DOCKER_DIR)/docker-compose.yaml up -d
	@echo -e "$(GREEN)✓ Application started$(NC)"

.PHONY: docker-stop
docker-stop: ## Stop Docker Compose application
	@echo -e "$(BLUE)Stopping Docker Compose application...$(NC)"
	@docker-compose -f $(DOCKER_DIR)/docker-compose.yaml down
	@echo -e "$(GREEN)✓ Application stopped$(NC)"

##@ Kubernetes Operations
.PHONY: k8s-deploy
k8s-deploy: ## Deploy to Kubernetes
	@echo -e "$(BLUE)Deploying to Kubernetes...$(NC)"
	@kubectl apply -f $(K8S_DIR)/namespace.yaml
	@kubectl apply -f $(K8S_DIR)/configmaps.yaml
	@kubectl apply -f $(K8S_DIR)/secrets.yaml
	@kubectl apply -f $(K8S_DIR)/postgres.yaml
	@kubectl apply -f $(K8S_DIR)/redis.yaml
	@kubectl apply -f $(K8S_DIR)/migration-job.yaml
	@kubectl wait --for=condition=complete job/migration-job --timeout=300s
	@kubectl apply -f $(K8S_DIR)/ad-api.yaml
	@kubectl apply -f $(K8S_DIR)/ad-processor.yaml
	@kubectl apply -f $(K8S_DIR)/ingress.yaml
	@echo -e "$(GREEN)✓ Deployed to Kubernetes$(NC)"

.PHONY: k8s-status
k8s-status: ## Show Kubernetes deployment status
	@echo -e "$(BLUE)Kubernetes Status:$(NC)"
	@kubectl get pods,services,ingress -n ad-processing

.PHONY: k8s-logs
k8s-logs: ## Show application logs from Kubernetes
	@echo -e "$(BLUE)Application logs:$(NC)"
	@kubectl logs -f deployment/ad-api -n ad-processing

.PHONY: k8s-delete
k8s-delete: ## Delete Kubernetes deployment
	@echo -e "$(YELLOW)Deleting Kubernetes deployment...$(NC)"
	@kubectl delete namespace ad-processing
	@echo -e "$(GREEN)✓ Kubernetes deployment deleted$(NC)"

##@ Monitoring and Debugging
.PHONY: logs
logs: ## Show application logs
	@echo -e "$(BLUE)Application logs:$(NC)"
	@tail -f logs/*.log

.PHONY: metrics
metrics: ## Show application metrics
	@echo -e "$(BLUE)Application metrics:$(NC)"
	@curl -s http://localhost:8080/metrics | grep -E "^(ad_|http_|go_)"

.PHONY: health
health: ## Check application health
	@echo -e "$(BLUE)Health check:$(NC)"
	@curl -s http://localhost:8080/health | jq .

.PHONY: ps
ps: ## Show running processes
	@echo -e "$(BLUE)Running processes:$(NC)"
	@pgrep -fl "ad-api\|ad-processor" || echo "No processes found"

##@ Database Tools
.PHONY: db-shell
db-shell: env-check ## Connect to database shell
	@echo -e "$(BLUE)Connecting to database...$(NC)"
	@psql "$(DATABASE_URL)"

.PHONY: db-dump
db-dump: env-check ## Create database dump
	@echo -e "$(BLUE)Creating database dump...$(NC)"
	@pg_dump "$(DATABASE_URL)" > dump_$(shell date +%Y%m%d_%H%M%S).sql
	@echo -e "$(GREEN)✓ Database dump created$(NC)"

.PHONY: db-restore
db-restore: env-check ## Restore database from dump (usage: make db-restore DUMP=dump_file.sql)
	@if [ -z "$(DUMP)" ]; then echo -e "$(RED)ERROR: DUMP is required. Usage: make db-restore DUMP=dump_file.sql$(NC)"; exit 1; fi
	@echo -e "$(BLUE)Restoring database from $(DUMP)...$(NC)"
	@psql "$(DATABASE_URL)" < $(DUMP)
	@echo -e "$(GREEN)✓ Database restored$(NC)"

##@ Utility Commands
.PHONY: deps
deps: ## Download and verify dependencies
	@echo -e "$(BLUE)Downloading dependencies...$(NC)"
	@go mod download
	@go mod verify
	@go mod tidy
	@echo -e "$(GREEN)✓ Dependencies updated$(NC)"

.PHONY: update-deps
update-deps: ## Update all dependencies to latest
	@echo -e "$(BLUE)Updating dependencies...$(NC)"
	@go get -u ./...
	@go mod tidy
	@echo -e "$(GREEN)✓ Dependencies updated$(NC)"

.PHONY: vendor
vendor: ## Create vendor directory
	@echo -e "$(BLUE)Creating vendor directory...$(NC)"
	@go mod vendor
	@echo -e "$(GREEN)✓ Vendor directory created$(NC)"

.PHONY: mock-gen
mock-gen: ## Generate mocks for interfaces
	@echo -e "$(BLUE)Generating mocks...$(NC)"
	@find . -name "*.go" -exec grep -l "//go:generate mockgen" {} \; | xargs -r go generate
	@echo -e "$(GREEN)✓ Mocks generated$(NC)"

.PHONY: swagger
swagger: ## Generate Swagger documentation
	@echo -e "$(BLUE)Generating Swagger docs...$(NC)"
	@swag init -g cmd/ad-api/main.go -o ./docs
	@echo -e "$(GREEN)✓ Swagger docs generated$(NC)"

.PHONY: version
version: ## Show version information
	@echo -e "$(BOLD)Version Information:$(NC)"
	@echo -e "  Version: $(BUILD_VERSION)"
	@echo -e "  Commit: $(BUILD_COMMIT)"
	@echo -e "  Build Time: $(BUILD_TIME)"
	@echo -e "  Go Version: $(GOVERSION)"
	@echo -e "  OS/Arch: $(GOOS)/$(GOARCH)"

# Include additional makefiles if they exist
-include Makefile.local
