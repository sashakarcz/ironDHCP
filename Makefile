.PHONY: help build build-web build-all run test clean dev-db dev-db-down migrate deps lint hashpw

# Default target
help:
	@echo "ironDHCP Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build       - Build the irondhcp binary"
	@echo "  build-web   - Build the React frontend"
	@echo "  build-all   - Build both backend and frontend"
	@echo "  run         - Run irondhcp with example config"
	@echo "  test        - Run tests"
	@echo "  clean       - Clean build artifacts"
	@echo "  dev-db      - Start local PostgreSQL with docker-compose"
	@echo "  dev-db-down - Stop local PostgreSQL"
	@echo "  migrate     - Run database migrations"
	@echo "  deps        - Download Go dependencies"
	@echo "  lint        - Run linters"
	@echo "  hashpw      - Generate password hash for web auth"

# Build the frontend
build-web:
	@echo "Building frontend..."
	@if [ ! -d "web/node_modules" ]; then \
		echo "Installing npm dependencies..."; \
		cd web && npm install; \
	fi
	cd web && npm run build
	@echo "Copying frontend build to API dist..."
	rm -rf internal/api/dist
	cp -r web/dist internal/api/dist
	@echo "Frontend built and embedded!"

# Build the binary (includes frontend if not already built)
build: deps
	@echo "Checking if frontend is built..."
	@if [ ! -d "internal/api/dist" ] || [ ! -f "internal/api/dist/index.html" ]; then \
		echo "Frontend not built, building now..."; \
		$(MAKE) build-web; \
	fi
	@echo "Building irondhcp with embedded frontend..."
	go build -o bin/irondhcp ./cmd/godhcp
	@echo ""
	@echo "✓ Build complete!"
	@echo "✓ Frontend embedded in binary"
	@echo "✓ Run with: ./bin/irondhcp"

# Build everything (force rebuild of frontend)
build-all: build-web build

# Run the server
run: build
	@echo "Starting irondhcp..."
	./bin/irondhcp --config example-config.yaml

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -cover ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	go clean

# Start local PostgreSQL for development
dev-db:
	@echo "Starting local PostgreSQL..."
	cd deployments/docker && docker-compose up -d
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 5
	@echo "PostgreSQL is ready at localhost:5432"
	@echo "  Database: godhcp"
	@echo "  User: dhcp"
	@echo "  Password: dhcp_dev_password"

# Stop local PostgreSQL
dev-db-down:
	@echo "Stopping local PostgreSQL..."
	cd deployments/docker && docker-compose down

# Run database migrations (manual for now)
migrate:
	@echo "Migrations are automatically applied by PostgreSQL on startup"
	@echo "Check deployments/docker/docker-compose.yml and migrations/"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

# Run linters
lint:
	@echo "Running linters..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; }
	golangci-lint run ./...

# Generate password hash for web authentication
hashpw:
	@go run ./cmd/hashpw/main.go $(filter-out $@,$(MAKECMDGOALS))

%:
	@:
