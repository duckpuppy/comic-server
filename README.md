# comic-server

A headless, standalone wireless sync server for ComicRack Android/iOS clients.

## Overview

`comic-server` implements the ComicRack wireless synchronization protocol, allowing Android and iOS devices to sync comic books without requiring the full ComicRack desktop application.

This project is currently under development and located within the [ComicRackCE](https://github.com/maforget/ComicRackCE) repository for reference purposes. It will eventually be split into its own repository.

## Status

🚧 **Work in Progress** - Core protocol implementation is underway.

See [WIRELESS_SYNC_PROTOCOL.md](./WIRELESS_SYNC_PROTOCOL.md) for the complete protocol specification.

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

## Usage

```bash
# Start the sync server
comic-server server --library /path/to/comics

# Start with specific devices ignored (useful for protecting production devices)
comic-server server --library /path/to/comics \
  --ignore-device 192.168.0.24 \
  --ignore-device "Galaxy Tab"

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
│   ├── server.go          # Server subcommand
│   ├── discover.go        # Device discovery
│   ├── version.go         # Version info
│   └── testclient/        # Test client (simulates device)
│       ├── main.go
│       └── README.md
├── internal/              # Private application code
│   ├── protocol/          # Binary protocol encoding/decoding
│   ├── device/            # Device management and registry
│   ├── library/           # ComicRack library management
│   ├── config/            # Configuration system
│   └── sync/              # Sync logic
├── testdata/              # Test data
│   └── ComicDB.xml        # Sample library file
├── main.go                # Entry point
└── WIRELESS_SYNC_PROTOCOL.md  # Protocol specification
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

### v0.3 - Planned

- [ ] Multiple smart lists per device
- [ ] Concurrent device sync support
- [ ] SQLite storage investigation
- [ ] Viper configuration migration (optional)

## License

To be determined (currently part of ComicRackCE repository).
