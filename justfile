# Default recipe to display help information
default:
    @just --list

# Build the comic-server binary
build:
    go build -o comic-server

# Build for multiple platforms
build-all:
    GOOS=linux GOARCH=amd64 go build -o comic-server-linux-amd64
    GOOS=darwin GOARCH=amd64 go build -o comic-server-darwin-amd64
    GOOS=darwin GOARCH=arm64 go build -o comic-server-darwin-arm64
    GOOS=windows GOARCH=amd64 go build -o comic-server-windows-amd64.exe

# Clean build artifacts
clean:
    rm -f comic-server comic-server-*

# Run all tests
test:
    go test ./...

# Run tests with verbose output
test-verbose:
    go test -v ./...

# Run tests with coverage
test-coverage:
    go test -cover ./...
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# Run a specific test by pattern
test-match PATTERN:
    go test -v -run {{PATTERN}} ./...

# Format Go code
fmt:
    go fmt ./...

# Run go vet
vet:
    go vet ./...

# Run all linters (fmt, vet)
lint: fmt vet

# Run the server with test library (no ignore)
run: build
    ./comic-server server --library /tmp/test-library

# Run the server with production tablet ignored
run-dev: build
    ./comic-server server --library /tmp/test-library --ignore-device 192.168.0.24

# Run the server with custom library path
run-with-library LIBRARY: build
    ./comic-server server --library {{LIBRARY}}

# Run the discover command
discover:
    ./comic-server discover

# Show version
version: build
    ./comic-server version

# Install dependencies
deps:
    go mod download
    go mod verify

# Update dependencies
deps-update:
    go get -u ./...
    go mod tidy

# Tidy go.mod
tidy:
    go mod tidy

# Build and run tests
ci: lint test build
    @echo "All CI checks passed!"

# Development workflow: clean, build, test
dev: clean build test
    @echo "Development build complete!"
