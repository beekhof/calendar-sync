.PHONY: build test test-verbose clean install run help

# Binary name
BINARY_NAME=calsync

# Build flags
LDFLAGS=-ldflags "-s -w"

# Default target
.DEFAULT_GOAL := help

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/calsync
	@echo "Build complete: $(BINARY_NAME)"

# Install the binary using go install
install:
	@echo "Installing $(BINARY_NAME)..."
	@go install $(LDFLAGS) ./cmd/calsync
	@echo "Install complete. Binary installed to $$(go env GOPATH)/bin/$(BINARY_NAME)"

# Run all tests
test:
	@echo "Running tests..."
	@go test ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -cover ./...

# Run tests and generate coverage report
test-coverage-html:
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@go clean
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@echo "Clean complete"


# Run the tool (requires -in and -out flags)
run:
	@echo "Usage: make run IN='<doc-url-or-id>' OUT='<output-file>'"
	@if [ -z "$(IN)" ] || [ -z "$(OUT)" ]; then \
		echo "Error: IN and OUT must be set"; \
		exit 1; \
	fi
	@./$(BINARY_NAME) -in $(IN) -out $(OUT)

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Format complete"

# Run linter (requires golangci-lint)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running linter..."; \
		golangci-lint run; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies updated"

# Show help
help:
	@echo "Available targets:"
	@echo "  build              - Build the binary"
	@echo "  test               - Run all tests"
	@echo "  test-verbose       - Run tests with verbose output"
	@echo "  test-coverage      - Run tests with coverage"
	@echo "  test-coverage-html - Generate HTML coverage report"
	@echo "  clean              - Remove build artifacts"
	@echo "  install            - Install binary to GOPATH/bin"
	@echo "  run                - Run the tool (requires IN and OUT vars)"
	@echo "  fmt                - Format code"
	@echo "  lint               - Run linter (requires golangci-lint)"
	@echo "  deps               - Download and tidy dependencies"
	@echo "  help               - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make test"
	@echo "  make run IN='https://docs.google.com/document/d/DOC_ID/edit' OUT='output.md'"

