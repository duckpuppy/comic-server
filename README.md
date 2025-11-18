# comic-server

A headless, standalone wireless sync server for ComicRack Android/iOS clients.

## Overview

`comic-server` implements the ComicRack wireless synchronization protocol, allowing Android and iOS devices to sync comic books without requiring the full ComicRack desktop application.

**Key Features:**
- **Bidirectional Sync**: Syncs comics to devices AND syncs reading progress back to library
- **Smart List Support**: Filter which comics sync to each device
- **Per-Device Configuration**: Different smart lists and settings per device
- **Web UI**: Real-time monitoring dashboard with WebSocket updates
- **REST API & Prometheus Metrics**: For monitoring and integration

This is a standalone project with its own git repository, located alongside the [ComicRackCE](https://github.com/maforget/ComicRackCE) project.

## Status

✅ **Production Ready** - Core functionality is complete and tested. The server successfully syncs comic libraries with Android/iOS devices.

See [WIRELESS_SYNC_PROTOCOL.md](./docs/WIRELESS_SYNC_PROTOCOL.md) for the complete protocol specification.

## Development Setup

This project uses [mise](https://mise.jdx.dev) for tool version management and [just](https://github.com/casey/just) for task running.

### Prerequisites

1. Install mise:
```bash
curl https://mise.jdx.dev/install.sh | sh
```

2. Activate mise in your shell:
```bash
mise activate bash >> ~/.bashrc  # or ~/.zshrc for zsh
```

3. Install project tools (Go and just):
```bash
mise install
```

### Available Tasks

Run `just` or `just --list` to see all available tasks:

```bash
just --list
```

Common tasks:
- `just build` - Build the server
- `just build-testclient` - Build the test client
- `just test` - Run all tests
- `just test-coverage` - Run tests with coverage report
- `just run` - Run the server with test library
- `just run-dev` - Run with production tablet ignored (safe for development)
- `just run-testclient` - Run test client that simulates a ComicRack device
- `just lint` - Format code and run linters
- `just dev` - Full development workflow (clean, build, test)
- `just build-all` - Build for all platforms (Linux, macOS, Windows)

## Building

Using just (recommended):
```bash
# Build the binary
just build

# Build for all platforms (Linux, macOS Intel/ARM, Windows)
just build-all

# Clean build artifacts
just clean
```

Manual building:
```bash
# Build the binary
go build -o comic-server

# Build with version information
go build -ldflags "-X github.com/duckpuppy/comic-server/cmd.Version=1.0.0 \
                    -X github.com/duckpuppy/comic-server/cmd.GitCommit=$(git rev-parse HEAD) \
                    -X github.com/duckpuppy/comic-server/cmd.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
         -o comic-server

# Build static binary (for deployment)
CGO_ENABLED=0 go build -o comic-server
```

## Docker Deployment

The easiest way to run comic-server is using Docker:

```bash
# Pull the latest image
docker pull ghcr.io/duckpuppy/comic-server:latest

# Run with docker run
docker run -d \
  --name comic-server \
  --network host \
  -v ~/.local/share/ComicRack/ComicDb.xml:/data/ComicDb.xml:ro \
  -v ~/Comics:/comics:ro \
  -e COMIC_SERVER_LOG_LEVEL=info \
  -e COMIC_SERVER_AUTO_SYNC=true \
  ghcr.io/duckpuppy/comic-server:latest

# Or use docker-compose
curl -O https://raw.githubusercontent.com/duckpuppy/comic-server/master/docker-compose.yml
docker-compose up -d
```

**Available images:**
- `latest` - Latest build from master/main
- `vX.Y.Z` - Specific release versions
- `X.Y` - Major.minor versions

**Platforms:**
- `linux/amd64` - x86_64
- `linux/arm64` - ARM64 (Raspberry Pi 4+, Apple Silicon)

See [docs/DOCKER.md](docs/DOCKER.md) for detailed Docker deployment guide.

## Usage

### Basic Usage

```bash
# Start the sync server
comic-server server --library /path/to/ComicDb.xml

# Start with automatic sync enabled
comic-server server --library /path/to/ComicDb.xml --auto-sync

# Start with specific devices ignored (useful for protecting production devices)
comic-server server --library /path/to/ComicDb.xml \
  --ignore-device 192.168.0.24 \
  --ignore-device "Galaxy Tab"

# Configure logging
comic-server server --library /path/to/ComicDb.xml \
  --log-level debug \
  --log-format json

# Using just (recommended for development)
just run              # Run with test library
just run-dev          # Run with production tablet ignored

# Discover devices on the network
comic-server discover

# Show version information
comic-server version

# Show help
comic-server --help
```

### Configuration File

Create a configuration file at `~/.config/comic-server/config.yaml`:

```yaml
server:
  library_path: "/path/to/ComicDb.xml"
  auto_sync: true
  log_level: "info"
  log_format: "text"
  ignore_devices:
    - "192.168.0.24"
    - "Galaxy Tab"

# Per-device sync configuration
devices:
  tablet-abc123:
    device_id: "tablet-abc123"
    device_name: "Samsung Galaxy Tab"
    lists:
      - list_id: "6300352f-35f2-4f98-8953-6ef29162122a"
        list_name: "Recent Comics"
        enabled: true
        settings:
          only_unread: true
          limit: true
          limit_value: 50
```

### Running as a Service

See [scripts/README.md](scripts/README.md) for detailed installation instructions.

**Linux (systemd):**
```bash
# Install and start
sudo cp comic-server /usr/local/bin/
sudo cp scripts/comic-server.service /etc/systemd/system/
sudo systemctl enable --now comic-server

# Reload configuration (SIGHUP)
sudo systemctl reload comic-server
```

**macOS (launchd):**
```bash
# Install and start
sudo cp comic-server /usr/local/bin/
cp scripts/com.comic-server.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.comic-server.plist
```

## Testing Without Physical Devices

A test client is included to simulate ComicRack devices:

```bash
# Build and run test client
just run-testclient

# Or manually
./testclient --sync --storage ./test-comics
```

The test client:
- Broadcasts device discovery messages every 5 seconds
- Listens for server commands on port 7614
- Saves received comic files to local storage
- Displays all protocol commands and metadata

See [cmd/testclient/README.md](cmd/testclient/README.md) for detailed usage.

### End-to-End Testing

**Terminal 1 - Start test client:**
```bash
just run-testclient
```

**Terminal 2 - Start server with auto-sync:**
```bash
./comic-server server --library ~/.local/share/ComicRack/ComicDb.xml --auto-sync
```

The test client will show all received commands and save synced files to `./test-storage/`.

## Project Structure

```
comic-server/
├── cmd/                    # CLI commands
│   ├── root.go            # Root command
│   ├── server.go          # Server subcommand (discovery, registry, signal handling)
│   ├── discover.go        # Device discovery
│   ├── version.go         # Version info
│   └── testclient/        # Test client (simulates device)
│       ├── main.go
│       └── README.md
├── internal/              # Private application code
│   ├── protocol/          # Binary protocol encoding/decoding
│   ├── device/            # Device management and registry
│   ├── library/           # ComicRack library management
│   ├── config/            # Configuration system (YAML/TOML)
│   ├── sync/              # Sync logic and file transfers
│   └── log/               # Structured logging (zerolog)
├── scripts/               # Utility scripts
│   ├── README.md          # Service installation guide
│   ├── comic-server.service  # systemd service file
│   ├── com.comic-server.plist  # launchd plist
│   └── reduce-comics.go   # Script to reduce CBZ file sizes
├── testdata/              # Test data
│   ├── ComicDB.xml        # Sample library file (226MB)
│   └── real-comics/       # Test CBZ files (reduced to 3 pages each)
├── main.go                # Entry point
└── WIRELESS_SYNC_PROTOCOL.md  # Protocol specification
```

## Monitoring and Management

### REST API

The server provides a REST API on port 7620 for monitoring and management:

**Health Check:**
```bash
curl http://localhost:7620/api/health
```

**Version Information:**
```bash
curl http://localhost:7620/api/version
```

**Active Sync Operations:**
```bash
curl http://localhost:7620/api/sync/status
```

**Sync History (with pagination):**
```bash
# Get first 20 entries
curl http://localhost:7620/api/sync/history?limit=20&offset=0

# Get next page
curl http://localhost:7620/api/sync/history?limit=20&offset=20
```

**Device List (with filtering):**
```bash
# All devices
curl http://localhost:7620/api/devices

# Filter by edition
curl http://localhost:7620/api/devices?edition=Android+Full

# Filter by sync status
curl http://localhost:7620/api/devices?syncing=true

# Filter by last seen (RFC3339 format)
curl "http://localhost:7620/api/devices?last_seen_after=2025-01-01T00:00:00Z"

# Combine filters
curl "http://localhost:7620/api/devices?edition=iOS&syncing=false&last_seen_after=2025-01-01T00:00:00Z"
```

**Server Statistics:**
```bash
curl http://localhost:7620/api/stats
```

### Prometheus Metrics

The server exposes Prometheus metrics at `/metrics`:

```bash
curl http://localhost:7620/metrics
```

Available metrics:
- `comic_server_syncs_total` - Total sync operations (by status)
- `comic_server_active_syncs` - Current active syncs
- `comic_server_books_processed_total` - Books processed (by operation)
- `comic_server_sync_duration_seconds` - Sync duration histogram

Example Prometheus scrape configuration:
```yaml
scrape_configs:
  - job_name: 'comic-server'
    static_configs:
      - targets: ['localhost:7620']
```

## Development Roadmap

### v0.2 - Complete! ✅

- [x] Project initialization
- [x] CLI framework setup (Cobra)
- [x] Tool management (mise + just)
- [x] Binary protocol implementation
- [x] Device discovery (UDP multicast)
- [x] Device registry and validation
- [x] TCP command handlers (ReadFile, WriteFile, DeleteFile, etc.)
- [x] Device ignore/filter functionality
- [x] Library management (ComicDB.xml parsing)
- [x] Configuration system (YAML/TOML, env vars, CLI flags)
- [x] Smart list filtering
- [x] Per-device configuration
- [x] Sync logic (file transfers, metadata, reading lists)
- [x] Test client for development

### v0.3 - Complete! ✅

- [x] Structured logging with zerolog (debug, info, warn, error levels)
- [x] JSON and text log formats
- [x] Per-device sync configuration storage (YAML/TOML)
- [x] Daemon/service mode support
- [x] SIGHUP signal handling for config reload
- [x] systemd service file (Linux)
- [x] launchd plist (macOS)
- [x] Service installation documentation

### v0.4 - Complete! ✅

- [x] Multiple smart lists per device - Full implementation
- [x] Security hardening
  - Device authentication validation
  - Rate limiting and connection limits
  - Input validation and sanitization
  - Security audit and testing

### v0.5 - Complete! ✅

- [x] Prometheus metrics for monitoring
  - Active syncs gauge
  - Sync operations counter (by status)
  - Books processed counter (by operation)
  - Sync duration histogram
- [x] REST API enhancements
  - `/api/health` - Health check with version info
  - `/api/version` - Version information
  - `/api/sync/status` - Active sync operations
  - `/api/sync/history` - Completed sync history
  - `/api/devices` - Connected devices
  - `/api/stats` - Server statistics
  - `/metrics` - Prometheus metrics endpoint

### v0.6 - Complete! ✅

- [x] Graceful device disconnect handling
  - Network error detection and classification
  - Proper error logging for disconnections vs. application errors
  - Timeout and connection reset detection
- [x] Pagination support for sync history
  - Offset-based pagination with metadata
  - Configurable page size (up to 100 entries)
  - Backward-compatible API
- [x] Device list filtering
  - Filter by edition (Android Full, Android Free, iOS)
  - Filter by sync status (currently syncing)
  - Filter by last seen timestamp
  - Combined filters with AND logic

### v0.7 - Complete! ✅

- [x] Web UI for server monitoring
  - Real-time dashboard with WebSocket updates
  - Device management (registration/unregistration)
  - Sync progress monitoring with progress bars
  - Sync history with file statistics
  - Responsive design with vanilla JavaScript

### v0.8 - Complete! ✅

- [x] Reverse sync (device-to-server metadata sync)
  - Reading progress sync (current page, open count, etc.)
  - User metadata sync (ratings, notes, reviews, tags)
  - Page bookmarks and metadata sync
  - Automatic library updates after each sync
  - Performance optimized (skips save when no changes)

### v1.0 - Planned (Enhanced Features)

- [ ] Performance optimization for large libraries
- [ ] Release automation (GoReleaser + Release Please)
- [ ] Enhanced monitoring and alerting

## License

To be determined (currently part of ComicRackCE repository).
