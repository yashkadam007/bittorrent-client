# BitTorrent Client Makefile

# Variables
BINARY_NAME=bittorrent-client
BUILD_DIR=build
MAIN_PACKAGE=.
GO_FILES=$(shell find . -name "*.go" -type f -not -path "./vendor/*")

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Build flags
LDFLAGS=-ldflags "-X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"

.PHONY: all build clean test coverage lint fmt vet deps run help install

# Default target
all: clean deps fmt vet lint test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for multiple platforms
build-all: build-linux build-windows build-darwin

build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)

build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PACKAGE)

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PACKAGE)

# Install the binary to GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME)..."
	$(GOCMD) install $(LDFLAGS) $(MAIN_PACKAGE)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Lint the code
lint:
	@echo "Running linter..."
	@if command -v $(GOLINT) >/dev/null 2>&1; then \
		$(GOLINT) run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Format the code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w $(GO_FILES)

# Vet the code
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Run the application with the sample torrent
run: build
	@if [ -f "archlinux-2025.08.01-x86_64.iso.torrent" ]; then \
		echo "Running with archlinux torrent..."; \
		./$(BUILD_DIR)/$(BINARY_NAME) archlinux-2025.08.01-x86_64.iso.torrent -verbose; \
	else \
		echo "No .torrent file found. Usage: make run TORRENT=file.torrent"; \
		echo "Or: ./$(BUILD_DIR)/$(BINARY_NAME) <torrent-file> [options]"; \
	fi

# Run with custom torrent file
run-with:
	@if [ -z "$(TORRENT)" ]; then \
		echo "Usage: make run-with TORRENT=file.torrent"; \
		exit 1; \
	fi
	@if [ ! -f "$(TORRENT)" ]; then \
		echo "Torrent file not found: $(TORRENT)"; \
		exit 1; \
	fi
	./$(BUILD_DIR)/$(BINARY_NAME) $(TORRENT) $(ARGS)

# Quick build and run for development
dev: build
	./$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

# Check if required tools are installed
check-tools:
	@echo "Checking required tools..."
	@command -v $(GOCMD) >/dev/null 2>&1 || { echo "Go is not installed. Please install Go first."; exit 1; }
	@echo "✓ Go is installed: $$(go version)"
	@if command -v $(GOLINT) >/dev/null 2>&1; then \
		echo "✓ golangci-lint is installed"; \
	else \
		echo "⚠ golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Show available torrents in current directory
list-torrents:
	@echo "Available .torrent files:"
	@find . -name "*.torrent" -type f | sed 's|^\./||' || echo "No .torrent files found"

# Show help
help:
	@echo "BitTorrent Client Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  build        Build the binary"
	@echo "  build-all    Build for all platforms (Linux, Windows, macOS)"
	@echo "  install      Install the binary to GOPATH/bin"
	@echo "  clean        Clean build artifacts"
	@echo "  test         Run tests"
	@echo "  coverage     Run tests with coverage report"
	@echo "  lint         Run linter (requires golangci-lint)"
	@echo "  fmt          Format code"
	@echo "  vet          Run go vet"
	@echo "  deps         Download and tidy dependencies"
	@echo "  run          Build and run with sample torrent"
	@echo "  run-with     Run with custom torrent: make run-with TORRENT=file.torrent"
	@echo "  dev          Quick build and run for development"
	@echo "  check-tools  Check if required tools are installed"
	@echo "  list-torrents List available .torrent files"
	@echo "  all          Run clean, deps, fmt, vet, lint, test, build"
	@echo "  help         Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make run"
	@echo "  make run-with TORRENT=ubuntu.torrent"
	@echo "  make dev ARGS='ubuntu.torrent -output /tmp -verbose'"
	@echo "  make test"
	@echo "  make coverage"
