# Enterprise Risk Engine Makefile

.PHONY: all build run test clean docker-build docker-up docker-down migrate help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_API=api-server
BINARY_WORKER=worker

# Docker parameters
DOCKER_COMPOSE=docker-compose

all: build

## build: Build both API server and worker binaries
build:
	@echo "Building API server..."
	$(GOBUILD) -o bin/$(BINARY_API) ./cmd/api-server
	@echo "Building worker..."
	$(GOBUILD) -o bin/$(BINARY_WORKER) ./cmd/worker
	@echo "Build complete!"

## build-api: Build only the API server
build-api:
	@echo "Building API server..."
	$(GOBUILD) -o bin/$(BINARY_API) ./cmd/api-server

## build-worker: Build only the worker
build-worker:
	@echo "Building worker..."
	$(GOBUILD) -o bin/$(BINARY_WORKER) ./cmd/worker

## run-api: Run the API server locally
run-api:
	@echo "Starting API server..."
	$(GOCMD) run ./cmd/api-server

## run-worker: Run the worker locally
run-worker:
	@echo "Starting worker..."
	$(GOCMD) run ./cmd/worker

## test: Run all tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

## docker-build: Build Docker images
docker-build:
	@echo "Building Docker images..."
	docker build -t risk-engine-api -f Dockerfile.api .
	docker build -t risk-engine-worker -f Dockerfile.worker .

## docker-up: Start all services with Docker Compose
docker-up:
	@echo "Starting services..."
	$(DOCKER_COMPOSE) up -d

## docker-up-scale: Start all services including scaled workers
docker-up-scale:
	@echo "Starting services with scaled workers..."
	$(DOCKER_COMPOSE) --profile scale up -d

## docker-down: Stop all services
docker-down:
	@echo "Stopping services..."
	$(DOCKER_COMPOSE) down

## docker-logs: View logs from all services
docker-logs:
	$(DOCKER_COMPOSE) logs -f

## docker-logs-api: View logs from API server
docker-logs-api:
	$(DOCKER_COMPOSE) logs -f api-server

## docker-logs-worker: View logs from worker
docker-logs-worker:
	$(DOCKER_COMPOSE) logs -f worker

## docker-clean: Remove all containers, volumes, and images
docker-clean:
	$(DOCKER_COMPOSE) down -v --rmi all

## migrate: Run database migrations
migrate:
	@echo "Running migrations..."
	psql $(DATABASE_URL) -f db/migrations/001_initial_schema.sql
	psql $(DATABASE_URL) -f db/migrations/002_create_partitions.sql
	psql $(DATABASE_URL) -f db/migrations/003_seed_rules.sql
	@echo "Migrations complete!"

## migrate-docker: Run migrations against Docker PostgreSQL
migrate-docker:
	@echo "Running migrations against Docker PostgreSQL..."
	docker exec -i risk-engine-postgres psql -U postgres -d risk_engine < db/migrations/001_initial_schema.sql
	docker exec -i risk-engine-postgres psql -U postgres -d risk_engine < db/migrations/002_create_partitions.sql
	docker exec -i risk-engine-postgres psql -U postgres -d risk_engine < db/migrations/003_seed_rules.sql
	@echo "Migrations complete!"

## lint: Run linter
lint:
	@echo "Running linter..."
	golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

## dev: Start development environment (postgres + redis only)
dev:
	@echo "Starting development dependencies..."
	$(DOCKER_COMPOSE) up -d postgres redis
	@echo "Waiting for services to be ready..."
	sleep 5
	@echo "Development environment ready!"

## dev-down: Stop development environment
dev-down:
	$(DOCKER_COMPOSE) stop postgres redis

## seed: Seed test data
seed:
	@echo "Seeding test data..."
	$(GOCMD) run ./scripts/seed.go

## benchmark: Run benchmarks
benchmark:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

## dashboard: Serve dashboard locally (requires Python 3)
dashboard:
	@echo "Starting dashboard at http://localhost:3000..."
	@cd dashboard && python3 -m http.server 3000

## dashboard-open: Open dashboard in browser
dashboard-open:
	@echo "Opening dashboard..."
	@xdg-open http://localhost:3000 2>/dev/null || open http://localhost:3000 2>/dev/null || echo "Open http://localhost:3000 in your browser"

## load-test: Run k6 load test (smoke)
load-test:
	@echo "Running k6 load test (smoke)..."
	k6 run scripts/load_test.js

## load-test-full: Run full k6 load test suite
load-test-full:
	@echo "Running full k6 load test suite..."
	k6 run --out json=scripts/load_test_results.json scripts/load_test.js

## load-test-stress: Run k6 stress test
load-test-stress:
	@echo "Running k6 stress test..."
	k6 run --vus 100 --duration 5m scripts/load_test.js

## test-api: Run API test script
test-api:
	@echo "Running API tests..."
	./scripts/test_api.sh

# ============================================
# Demo & Kafka Commands
# ============================================

## demo: Run the interactive demo script
demo:
	@echo "Starting interactive demo..."
	./scripts/demo.sh

## kafka-up: Start all services including Kafka ecosystem
kafka-up:
	@echo "Starting all services with Kafka..."
	docker compose --profile kafka up -d
	@echo "Waiting for services to be ready..."
	@sleep 30
	@echo "Setting up Debezium CDC connector..."
	./scripts/setup-debezium.sh

## kafka-down: Stop Kafka services
kafka-down:
	@echo "Stopping Kafka services..."
	docker compose --profile kafka down

## kafka-logs: View Kafka worker logs
kafka-logs:
	docker compose --profile kafka logs -f kafka-worker

## kafka-ui: Open Kafka UI in browser
kafka-ui:
	@echo "Opening Kafka UI..."
	@xdg-open http://localhost:8090 2>/dev/null || open http://localhost:8090 2>/dev/null || echo "Open http://localhost:8090 in your browser"

## debezium-status: Check Debezium connector status
debezium-status:
	@curl -s http://localhost:8083/connectors/risk-engine-connector/status | jq '.'

## debezium-setup: Setup Debezium CDC connector
debezium-setup:
	./scripts/setup-debezium.sh

## dashboard: Open dashboard in browser
dashboard:
	@echo "Opening dashboard..."
	@xdg-open http://localhost:3000 2>/dev/null || open http://localhost:3000 2>/dev/null || echo "Open http://localhost:3000 in your browser"

## full-demo: Complete demo setup (Kafka + Dashboard + Demo script)
full-demo: kafka-up
	@echo ""
	@echo "============================================"
	@echo "  Full Demo Environment Ready!"
	@echo "============================================"
	@echo ""
	@echo "  Dashboard:  http://localhost:3000"
	@echo "  Kafka UI:   http://localhost:8090"
	@echo "  API:        http://localhost:8080"
	@echo ""
	@echo "  Login: admin@example.com / admin123"
	@echo ""
	@echo "Run './scripts/demo.sh' to start the interactive demo"
	@echo ""

## help: Show this help message
help:
	@echo "Enterprise Risk Engine - Available Commands:"
	@echo ""
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

# Default target
.DEFAULT_GOAL := help
