.PHONY: help dev dev-deps dev-stop run test build docker-build clean

# Default target
help:
	@echo "Available commands:"
	@echo "  make dev         - Start development environment with docker-compose"
	@echo "  make stop        - Stop development environment"
	@echo "  make logs        - View logs of the development environment"
	@echo "  make status      - Show status of services"
	@echo "  make test        - Run tests"
	@echo "  make run         - Run the application locally"
	@echo "  make build       - Build the application binary"
	@echo ""
	@echo "Access URLs:"
	@echo "  API:          http://localhost:8080"
	@echo "  PostgreSQL:   localhost:5433"
	@echo "  Redis:        localhost:6379"

# Start full development environment
dev:
	@echo "Starting development environment..."
	@cp -n .env.dev .env 2>/dev/null || true
	docker-compose -f docker-compose.dev.yml up -d
	@echo "Waiting for services to be ready..."
	@sleep 5
	@echo "Starting API..."
	go run cmd/api/main.go

# Start only dependencies (for local Go development)
dev-deps:
	@echo "Starting development dependencies..."
	@cp -n .env.dev .env 2>/dev/null || true
	docker-compose -f docker-compose.dev.yml up -d postgres redis
	@echo "Services started:"
	@echo "  PostgreSQL: localhost:5432"
	@echo "  Redis:      localhost:6379"
	@echo "  Adminer:    http://localhost:8081"

# Stop development environment
dev-stop:
	@echo "Stopping development environment..."
	docker-compose -f docker-compose.dev.yml down

# Run the API locally
run:
	@cp -n .env.dev .env 2>/dev/null || true
	go run cmd/api/main.go

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Build binary
build:
	@echo "Building binary..."
	go build -o bin/synesthesie-api cmd/api/main.go

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t synesthesie-api:latest .

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	go clean