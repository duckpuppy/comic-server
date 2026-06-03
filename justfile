# Default recipe to display help information
default:
    @just --list

# Build the comic-server binary
build:
    mise exec -- go build -o comic-server

# Build the test client
build-testclient:
    mise exec -- go build -o testclient ./cmd/testclient

# Build for Windows (useful when developing in WSL2)
build-windows:
    mise exec -- env GOOS=windows GOARCH=amd64 go build -o comic-server.exe
    @echo "Windows binary created: comic-server.exe"
    @echo "Run on Windows: .\\comic-server.exe server --library path\\to\\ComicDb.xml"

# Build for multiple platforms
build-all:
    mise exec -- env GOOS=linux GOARCH=amd64 go build -o comic-server-linux-amd64
    mise exec -- env GOOS=darwin GOARCH=amd64 go build -o comic-server-darwin-amd64
    mise exec -- env GOOS=darwin GOARCH=arm64 go build -o comic-server-darwin-arm64
    mise exec -- env GOOS=windows GOARCH=amd64 go build -o comic-server-windows-amd64.exe

# Clean build artifacts
clean:
    rm -f comic-server comic-server-* comic-server.exe testclient testclient-*

# Run all tests
test:
    mise exec -- go test ./...

# Run tests with verbose output
test-verbose:
    mise exec -- go test -v ./...

# Run tests with coverage
test-coverage:
    mise exec -- go test -cover ./...
    mise exec -- go test -coverprofile=coverage.out ./...
    mise exec -- go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# Run a specific test by pattern
test-match PATTERN:
    mise exec -- go test -v -run {{PATTERN}} ./...

# Format Go code
fmt:
    mise exec -- go fmt ./...

# Run go vet
vet:
    mise exec -- go vet ./...

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
    mise exec -- go mod download
    mise exec -- go mod verify

# Update dependencies
deps-update:
    mise exec -- go get -u ./...
    mise exec -- go mod tidy

# Tidy go.mod
tidy:
    mise exec -- go mod tidy

# Build and run tests
ci: lint test build
    @echo "All CI checks passed!"

# Development workflow: clean, build, test
dev: clean build test
    @echo "Development build complete!"

# Run test client (simulates a ComicRack device)
run-testclient: build-testclient
    ./testclient --sync

# Run test client with custom storage
run-testclient-storage STORAGE: build-testclient
    ./testclient --sync --storage {{STORAGE}}
