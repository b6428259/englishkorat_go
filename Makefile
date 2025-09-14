# English Korat Go Backend Makefile

.PHONY: build run dev test clean docker-build docker-run help

# Build the application
build:
	@echo "🔨 Building English Korat API..."
	@go build -o englishkorat-api main.go
	@echo "✅ Build completed successfully!"

# Run the application
run: build
	@echo "🚀 Starting English Korat API..."
	@./englishkorat-api

# Run in development mode with live reload (requires air)
dev:
	@echo "🔄 Starting development server with live reload..."
	@air -c .air.toml 2>/dev/null || go run main.go

# Run tests
test:
	@echo "🧪 Running tests..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "🧪 Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "📊 Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -f englishkorat-api
	@rm -f coverage.out coverage.html
	@rm -rf dist/
	@echo "✅ Clean completed!"

# Format code
format:
	@echo "🎨 Formatting code..."
	@go fmt ./...
	@echo "✅ Code formatted!"

# Lint code
lint:
	@echo "🔍 Linting code..."
	@golangci-lint run 2>/dev/null || echo "⚠️  golangci-lint not installed, skipping..."

# Tidy dependencies
tidy:
	@echo "📦 Tidying dependencies..."
	@go mod tidy
	@echo "✅ Dependencies tidied!"

# Download dependencies
deps:
	@echo "📥 Downloading dependencies..."
	@go mod download
	@echo "✅ Dependencies downloaded!"

# Build for production
build-prod:
	@echo "🏭 Building for production..."
	@CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o englishkorat-api main.go
	@echo "✅ Production build completed!"

# Docker build
docker-build:
	@echo "🐳 Building Docker image..."
	@docker build -t englishkorat-api:latest .
	@echo "✅ Docker image built!"

# Docker run
docker-run: docker-build
	@echo "🐳 Running Docker container..."
	@docker run -p 3000:3000 --env-file .env englishkorat-api:latest

# Database migration
migrate:
	@echo "🗄️  Running database migrations..."
	@go run main.go migrate
	@echo "✅ Migrations completed!"

# Seed database (DEPRECATED)
# NOTE: Seeding is intentionally only available via the start-seed.ps1 script
# to avoid accidental data loss. The Makefile target is kept for discoverability
# but will not run seeding automatically.
seed:
	@echo "⚠️  The Makefile 'seed' target is deprecated. Use start-seed.ps1 to run seeds."
	@echo "    PowerShell: ./start-seed.ps1"
	@echo "    Alternatively, run the start-seed.ps1 script manually to execute seeding."

# Setup development environment
setup-dev: deps build
	@echo "🛠️  Setting up development environment..."
	@cp .env.example .env
	@echo "📝 Please configure your .env file"
	@echo "✅ Development environment setup completed!"

# Install development tools
install-tools:
	@echo "🔧 Installing development tools..."
	@go install github.com/cosmtrek/air@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "✅ Development tools installed!"

# Health check
health:
	@echo "🏥 Checking API health..."
	@curl -s http://localhost:3000/health | jq . 2>/dev/null || echo "⚠️  API not running or jq not installed"

# Help
help:
	@echo "🆘 Available commands:"
	@echo "  build         - Build the application"
	@echo "  run           - Build and run the application"
	@echo "  dev           - Run in development mode with live reload"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  clean         - Clean build artifacts"
	@echo "  format        - Format code"
	@echo "  lint          - Lint code"
	@echo "  tidy          - Tidy dependencies"
	@echo "  deps          - Download dependencies"
	@echo "  build-prod    - Build for production"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  migrate       - Run database migrations"
	@echo "  seed          - Seed database"
	@echo "  setup-dev     - Setup development environment"
	@echo "  install-tools - Install development tools"
	@echo "  health        - Check API health"
	@echo "  help          - Show this help message"

# Default target
all: build