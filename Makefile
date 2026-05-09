# Cannectors Runtime Makefile
# Build, test, and lint commands for the Go CLI

# Build variables
BINARY_NAME=cannectors
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOVET=$(GOCMD) vet

# Directories
CMD_DIR=./cmd/cannectors
BIN_DIR=./bin
DIST_DIR=./dist
TEST_LAB_COMPOSE_FILE=test-lab/docker-compose.yml

# Default target
.DEFAULT_GOAL := help

##@ General

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: build
build: ## Build the binary for current platform
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Binary built: $(BIN_DIR)/$(BINARY_NAME)"

.PHONY: run
run: build ## Build and run the CLI
	@$(BIN_DIR)/$(BINARY_NAME) $(ARGS)

.PHONY: install
install: ## Install the binary to GOPATH/bin
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_NAME) $(CMD_DIR)

.PHONY: clean
clean: ## Remove build artifacts
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BIN_DIR) $(DIST_DIR)
	@rm -f coverage.out coverage.html

##@ Testing

.PHONY: test
test: ## Run all tests
	@echo "Running tests..."
	$(GOTEST) -v ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: test-race
test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	$(GOTEST) -v -race ./...

##@ Local Test Lab

.PHONY: test-lab-up
test-lab-up: ## Start local PostgreSQL and WireMock test lab
	docker compose -f $(TEST_LAB_COMPOSE_FILE) up -d --wait

.PHONY: test-lab-down
test-lab-down: ## Stop local test lab without deleting persisted volumes
	docker compose -f $(TEST_LAB_COMPOSE_FILE) down --remove-orphans

.PHONY: test-lab-reset
test-lab-reset: ## Recreate local test lab volumes, database seeds, stubs, and request journal
	docker compose -f $(TEST_LAB_COMPOSE_FILE) down --volumes --remove-orphans
	docker compose -f $(TEST_LAB_COMPOSE_FILE) up -d --wait

.PHONY: test-lab-db-reset
test-lab-db-reset: ## Truncate and reseed the local PostgreSQL test database
	cat test-lab/postgres/reset.sql test-lab/postgres/init/002_seed.sql | docker compose -f $(TEST_LAB_COMPOSE_FILE) exec -T postgres psql -U cannectors_test -d cannectors_test

.PHONY: test-lab-requests
test-lab-requests: ## List requests captured by WireMock
	curl -fsS http://localhost:18080/__admin/requests

.PHONY: test-lab-requests-reset
test-lab-requests-reset: ## Clear WireMock request journal
	curl -fsS -X DELETE http://localhost:18080/__admin/requests

.PHONY: test-lab-mappings-reload
test-lab-mappings-reload: ## Reload WireMock stub mappings from disk
	curl -fsS -X POST http://localhost:18080/__admin/mappings/reset >/dev/null

.PHONY: test-lab-verify-pagination
test-lab-verify-pagination: ## Run HTTP pagination scenarios (story 22.1)
	bash test-lab/scripts/verify-pagination.sh

.PHONY: test-lab-verify-database
test-lab-verify-database: ## Run database input/output scenarios (story 22.2)
	bash test-lab/scripts/verify-database.sh

.PHONY: test-lab-verify-sql-call
test-lab-verify-sql-call: ## Run sql_call enrichment scenarios (story 22.3)
	bash test-lab/scripts/verify-sql-call.sh

.PHONY: test-lab-verify-http-call
test-lab-verify-http-call: ## Run http_call enrichment scenarios (story 22.4)
	bash test-lab/scripts/verify-http-call.sh

.PHONY: test-lab-verify-filters
test-lab-verify-filters: ## Run transformation filter scenarios (story 22.5)
	bash test-lab/scripts/verify-filters.sh

.PHONY: test-lab-verify-state-persistence
test-lab-verify-state-persistence: ## Run httpPolling state persistence scenarios (story 22.6)
	bash test-lab/scripts/verify-state-persistence.sh

.PHONY: test-lab-verify-webhook
test-lab-verify-webhook: ## Run webhook input scenarios (story 22.7)
	bash test-lab/scripts/verify-webhook.sh

.PHONY: test-lab-verify-auth
test-lab-verify-auth: ## Run HTTP authentication scenarios (story 23.1)
	bash test-lab/scripts/verify-auth.sh

.PHONY: test-lab-verify-retry
test-lab-verify-retry: ## Run reliability (retry/timeout/error) scenarios (story 23.2)
	bash test-lab/scripts/verify-retry.sh

.PHONY: test-lab-run
test-lab-run: ## Run the full test-lab E2E suite, including webhook scenarios. Filter declarative scenarios with SCENARIO=name1,name2.
	@if [ -n "$(SCENARIO)" ]; then \
		SCENARIO="$(SCENARIO)" python3 test-lab/run.py; \
	else \
		python3 test-lab/run.py && bash test-lab/scripts/verify-webhook.sh; \
	fi

##@ Code Quality

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting code..."
	$(GOFMT) -s -w .

.PHONY: fmt-check
fmt-check: ## Check if code is formatted
	@echo "Checking formatting..."
	@test -z "$$($(GOFMT) -l .)" || (echo "Code is not formatted. Run 'make fmt'" && exit 1)

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	$(GOVET) ./...

.PHONY: lint
lint: ## Run golangci-lint (requires golangci-lint installed)
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

.PHONY: check
check: fmt-check vet ## Run all code quality checks (fmt, vet)
	@echo "All checks passed!"

##@ Dependencies

.PHONY: deps
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download

.PHONY: deps-tidy
deps-tidy: ## Tidy dependencies
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

.PHONY: deps-update
deps-update: ## Update all dependencies
	@echo "Updating dependencies..."
	$(GOGET) -u ./...
	$(GOMOD) tidy

##@ Cross-Platform Build

.PHONY: build-all
build-all: build-linux build-darwin build-windows ## Build for all platforms

.PHONY: build-linux
build-linux: ## Build for Linux (amd64)
	@echo "Building for Linux..."
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)

.PHONY: build-darwin
build-darwin: ## Build for macOS (amd64 and arm64)
	@echo "Building for macOS..."
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)

.PHONY: build-windows
build-windows: ## Build for Windows (amd64)
	@echo "Building for Windows..."
	@mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)

##@ Release

.PHONY: release
release: clean build-all ## Create release builds for all platforms
	@echo "Release builds created in $(DIST_DIR)/"
	@ls -la $(DIST_DIR)/
