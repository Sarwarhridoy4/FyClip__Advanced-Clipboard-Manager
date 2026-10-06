# FyClip Makefile
# Build and release automation

# Variables
APP_NAME := FyClip
BINARY_NAME := fyclip
VERSION := $(shell git describe --tags --always --dirty | sed 's/^v//')
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
VERSION_PACKAGE := github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/internal/version
LDFLAGS := -ldflags "-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X ${VERSION_PACKAGE}.Version=${VERSION} -X ${VERSION_PACKAGE}.BuildTime=${BUILD_TIME}"
LDFLAGS_DEBUG := -ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X ${VERSION_PACKAGE}.Version=${VERSION} -X ${VERSION_PACKAGE}.BuildTime=${BUILD_TIME}"

# Go parameters
GO := go
GOCMD := go
GOBUILD := $(GOCMD) build $(LDFLAGS)
GOBUILD_DEBUG := $(GOCMD) build $(LDFLAGS_DEBUG)
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := $(GOCMD) fmt

# Directories
OUTPUT_DIR := bin
SOURCE_DIRS := .

# Platform-specific settings
ifeq ($(OS),Windows_NT)
	EXT := .exe
else
	EXT :=
endif

# Default target
.PHONY: all
all: lint test build

# Build the application
.PHONY: build
build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) -o $(OUTPUT_DIR)/$(BINARY_NAME)$(EXT) .

# Build for all platforms
.PHONY: build-all
build-all: build-linux build-darwin build-windows

.PHONY: build-linux
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(OUTPUT_DIR)/linux
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(OUTPUT_DIR)/linux/$(BINARY_NAME) .
	GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(OUTPUT_DIR)/linux/$(BINARY_NAME)-arm64 .

.PHONY: build-darwin
build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(OUTPUT_DIR)/darwin
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(OUTPUT_DIR)/darwin/$(BINARY_NAME) .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(OUTPUT_DIR)/darwin/$(BINARY_NAME)-arm64 .

.PHONY: build-windows
build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(OUTPUT_DIR)/windows
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(OUTPUT_DIR)/windows/$(BINARY_NAME).exe .

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

# Run benchmarks
.PHONY: benchmark
benchmark:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Run linter
.PHONY: lint
lint:
	@echo "Running linter..."
	$(GOVET) ./...
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, skipping..."; \
	fi

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	$(GOFMT) -w $(SOURCE_DIRS)

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	rm -rf $(OUTPUT_DIR)
	rm -f coverage.out

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	$(GOCMD) mod download
	$(GOCMD) mod tidy

# Run the application
.PHONY: run
run:
	$(GOCMD) run .

# Generate coverage report
.PHONY: coverage
coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Build with Fyne packaging
.PHONY: package
package: build
	@echo "Packaging application..."
	./build.sh

# Build Debian package
.PHONY: deb
deb: build
	@echo "Building Debian package..."
	./build-deb.sh

# Build Debian package with specific version
.PHONY: deb-version
deb-version: build
	@echo "Building Debian package version $(VERSION)..."
	./build-deb.sh $(VERSION)

# Build PPA source package
.PHONY: ppa
ppa: build
	@echo "Building PPA source package..."
	./build-ppa.sh

# Build PPA source package with specific version
.PHONY: ppa-version
ppa-version: build
	@echo "Building PPA source package version $(VERSION)..."
	./build-ppa.sh $(VERSION)

# Create release
.PHONY: release
release: clean test lint build-all package
	@echo "Creating release..."
	@mkdir -p release
	cd $(OUTPUT_DIR) && zip -r ../release/$(APP_NAME)-$(VERSION)-all.zip ./

# Development shortcuts
.PHONY: dev
dev: deps run

.PHONY: check
check: lint test

# Run verbose tests
.PHONY: test-verbose
test-verbose:
	@echo "Running tests with verbose output..."
	$(GOTEST) -v ./...

# Run tests with race detector
.PHONY: test-race
test-race:
	@echo "Running tests with race detector..."
	$(GOTEST) -race ./...

# Run specific test by name
.PHONY: test-name
test-name:
	@echo "Running tests matching '$(NAME)'..."
	$(GOTEST) -v -run $(NAME) ./...

# Run tests for a specific package
.PHONY: test-package
test-package:
	@echo "Running tests for package '$(PKG)'..."
	$(GOTEST) -v $(PKG)

# Run benchmarks for clipboard package
.PHONY: bench
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./internal/clipboard/...

# Run specific benchmark
.PHONY: bench-name
bench-name:
	@echo "Running benchmark '$(NAME)'..."
	$(GOTEST) -bench=$(NAME) -benchmem ./...

# Save benchmark results
.PHONY: bench-save
bench-save:
	@echo "Running and saving benchmarks..."
	$(GOTEST) -bench=. -benchmem -run=^$ ./... > benchmark.txt

# Build debug version
.PHONY: build-debug
build-debug:
	@echo "Building debug version..."
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD_DEBUG) -o $(OUTPUT_DIR)/$(BINARY_NAME)-debug$(EXT) .

# Build release version
.PHONY: build-release
build-release:
	@echo "Building release version..."
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) -o $(OUTPUT_DIR)/$(BINARY_NAME)$(EXT) .

# Install linter
.PHONY: install-lint
install-lint:
	@echo "Installing golangci-lint..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run static analysis
.PHONY: static
static:
	@echo "Running static analysis..."
	$(GOVET) ./...
	@if command -v staticcheck > /dev/null; then \
		staticcheck ./...; \
	else \
		echo "staticcheck not found, skipping..."; \
	fi

# Show test coverage
.PHONY: cover
cover:
	@echo "Showing test coverage..."
	$(GOTEST) -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Watch tests
.PHONY: watch
watch:
	@echo "Running tests in watch mode..."
	@if command -v gotestsum > /dev/null; then \
		gotestsum -watch; \
	else \
		echo "gotestsum not found, install with: go install gotest.tools/gotestsum@latest"; \
	fi

# Update dependencies
.PHONY: update-deps
update-deps:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

# Verify dependencies
.PHONY: verify-deps
verify-deps:
	@echo "Verifying dependencies..."
	go mod verify
	go mod tidy

# Show version info
.PHONY: version
version:
	@echo "$(APP_NAME) version: $(VERSION)"
	@echo "Build time: $(BUILD_TIME)"

# Development with hot reload (requires fyne)
.PHONY: dev-gui
dev-gui:
	@echo "Running in development mode..."
	fyne package --release --name $(BINARY_NAME) --icon icon.png
	./$(BINARY_NAME)

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo ""
	@echo "=== Building ==="
	@echo "  build          - Build the application"
	@echo "  build-debug    - Build debug version"
	@echo "  build-release  - Build release version"
	@echo "  build-all      - Build for Linux, Darwin, and Windows"
	@echo ""
	@echo "=== Testing ==="
	@echo "  test           - Run tests"
	@echo "  test-verbose   - Run tests with verbose output"
	@echo "  test-race      - Run tests with race detector"
	@echo "  test-name      - Run tests matching pattern (NAME=...)"
	@echo "  test-package   - Run tests for specific package (PKG=...)"
	@echo ""
	@echo "=== Benchmarking ==="
	@echo "  bench          - Run benchmarks"
	@echo "  bench-name     - Run specific benchmark (NAME=...)"
	@echo "  bench-save     - Run and save benchmarks to file"
	@echo ""
	@echo "=== Code Quality ==="
	@echo "  lint           - Run linter"
	@echo "  fmt            - Format code"
	@echo "  static         - Run static analysis"
	@echo "  cover          - Show test coverage"
	@echo "  install-lint   - Install golangci-lint"
	@echo ""
	@echo "=== Dependencies ==="
	@echo "  deps           - Install dependencies"
	@echo "  update-deps    - Update dependencies"
	@echo "  verify-deps    - Verify dependencies"
	@echo ""
	@echo "=== Development ==="
	@echo "  dev            - Development mode (deps + run)"
	@echo "  run            - Run the application"
	@echo "  dev-gui        - Run in GUI development mode"
	@echo "  watch          - Run tests in watch mode"
	@echo ""
	@echo "=== Release ==="
	@echo "  package        - Package the application (Fyne/AppImage/Tarball)"
	@echo "  deb            - Build Debian package"
	@echo "  deb-version    - Build Debian package with specific version (VERSION=...)"
	@echo "  ppa            - Build PPA source package"
	@echo "  ppa-version    - Build PPA source package with specific version (VERSION=...)"
	@echo "  release        - Create release"
	@echo ""
	@echo "=== Utilities ==="
	@echo "  clean          - Clean build artifacts"
	@echo "  coverage       - Generate coverage report"
	@echo "  version        - Show version info"
	@echo "  check          - Run lint and tests"
	@echo "  help           - Show this help message"
