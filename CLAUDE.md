# Comic Server - Claude Code Guide

This guide helps Claude Code instances work effectively in the comic-server repository.

## Project Overview

**comic-server** is a headless standalone wireless sync server for ComicRack Android/iOS clients, written in Go. It implements the ComicRack wireless synchronization protocol to allow devices to sync comic books without requiring the full ComicRack desktop application.

This is a standalone project with its own git repository, located alongside the [ComicRackCE](https://github.com/maforget/ComicRackCE) project (`../ComicRackCE`).

## Important Rules

⚠️ **CRITICAL**: This is a separate project from ComicRackCE.

- **DO NOT** modify any files in the ComicRackCE directory (`../ComicRackCE`)
- **DO NOT** commit changes to the ComicRackCE repository
- All work should be confined to the `comic-server/` directory
- This project **MUST** remain fully compatible with ComicRack libraries created by ComicRackCE
- The ComicRackCE source code in `../ComicRackCE` is the ultimate reference for library format and protocol behavior

## Quick Start

### Prerequisites

- [mise](https://mise.jdx.dev) for tool version management
- Go 1.25.1 (managed by mise)
- just task runner (managed by mise)

### Setup

```bash
# Install tools
mise install

# Build and test
just dev

# Run the server
just run-dev  # Runs with production tablet ignored
```

### Common Commands

```bash
just --list          # Show all available tasks
just build           # Build the binary
just test            # Run all tests
just test-coverage   # Generate coverage report
just run-dev         # Run with production device ignored
just lint            # Format and lint code
just ci              # Run all checks (lint + test + build)
```

## Project Structure

```
comic-server/
├── cmd/                    # CLI commands (Cobra framework)
│   ├── root.go            # Root command
│   ├── server.go          # Server command (device discovery & registry)
│   ├── discover.go        # Discovery command
│   ├── version.go         # Version command
│   ├── config.go          # Config command group
│   ├── config_*.go        # Config subcommands (add-list, remove-list, etc.)
│   └── ...
├── internal/              # Internal packages (not importable)
│   ├── config/           # Configuration management
│   │   ├── xdg.go        # XDG directory resolution
│   │   ├── config.go     # Config structures
│   │   ├── loader.go     # YAML/TOML loading/saving
│   │   ├── helpers.go    # Device/list resolution
│   │   └── device.go     # Device-specific operations
│   ├── device/           # Device discovery and management
│   │   ├── discovery.go  # UDP multicast listener
│   │   ├── info.go       # Device info parsing (INI format)
│   │   └── registry.go   # Device tracking and validation
│   ├── library/          # ComicRack library management
│   │   ├── library.go    # Library XML parsing
│   │   ├── time.go       # Custom time format handling
│   │   └── library_test.go  # Comprehensive tests
│   ├── protocol/         # Binary protocol implementation
│   │   ├── protocol.go   # Encoding/decoding (big-endian)
│   │   └── client.go     # TCP client for device communication
│   └── sync/             # Synchronization logic
│       ├── sync.go       # Sync orchestration
│       ├── settings.go   # Sync settings (OnlyUnread, Limit, Sort, etc.)
│       └── ...
├── main.go               # Application entry point
├── go.mod                # Go module definition
├── go.sum                # Dependency checksums
├── .mise.toml            # Tool version management
├── justfile              # Task definitions
├── .gitignore            # Git ignore patterns
└── WIRELESS_SYNC_PROTOCOL.md  # Protocol specification
```

## Architecture

### Communication Protocol

- **Discovery**: UDP multicast on 224.34.123.90:7615
  - Devices broadcast: `ComicRack[Variant]:{device_id}[:Sync]`
  - Variants: `ComicRack`, `ComicRackAndroid`, `ComicRackiOS`

- **Device Communication**: TCP on port 7614
  - Binary protocol with big-endian encoding
  - Commands: ReadFile, WriteFile, FileExists, DeleteFile, etc.

- **Server Control**: TCP on port 7620+

### Binary Protocol Types

- `INT`: 4-byte signed integer (big-endian)
- `LONG`: 8-byte signed integer (big-endian)
- `STRING`: INT(length) + UTF-8 bytes
- `BOOL`: 1-byte (0 or 1)
- `DATA`: INT(length) + raw bytes

### Device Validation

- Devices send `comicrack.ini` with metadata
- Server validates device hash: SHA1(Model + Manufacturer + Serial + Edition + Version)
- Registry tracks discovered devices with last-seen timestamps

## Development Workflow

### Making Changes

1. Create/modify code
2. Run tests: `just test`
3. Format and lint: `just lint`
4. Build: `just build`
5. Test manually: `just run-dev`

### Running Tests

```bash
just test              # All tests
just test-verbose      # Verbose output
just test-coverage     # Generate HTML coverage report
just test-match PATTERN  # Run specific tests
```

### Device Ignore Functionality

To protect production devices during development:

```bash
# In justfile: run-dev task ignores 192.168.0.24
just run-dev

# Manual ignore by IP, ID, or name:
./comic-server server --library /path \
  --ignore-device 192.168.0.24 \
  --ignore-device "SM-T970"
```

## Key Implementation Details

### Device Discovery (internal/device/discovery.go)

- Uses `golang.org/x/net/ipv4` for proper multicast support
- Joins multicast group on default network interface
- Parses broadcasts using `strings.HasPrefix("ComicRack")` to match any variant
- Sends discovered devices to channel for processing

### Device Info Parsing (internal/device/info.go)

- Parses INI format from `comicrack.ini`
- Required fields: Name, Model, Manufacturer, Serial, ID, Hash, Version, Edition
- Editions: "Android Free" (100 book limit), "Android Full" (unlimited), "iOS" (unlimited)
- Validates SHA1 hash for authentication

### TCP Client (internal/protocol/client.go)

- Implements connection pooling with configurable timeout
- Thread-safe command execution
- Commands: ReadFile, WriteFile, FileExists, DeleteFile, GetDeviceInfo, etc.

### Library Management (internal/library/library.go)

- Reads ComicRackCE library XML format (`ComicDb.xml`)
- Parses comic books with full metadata (series, title, publisher, credits, etc.)
- Supports reading lists and smart lists
- Custom time format handling for .NET DateTime compatibility
- Library path typically: `~/.local/share/ComicRack/ComicDb.xml` (Linux) or `%APPDATA%/ComicRack/ComicDb.xml` (Windows)

### Configuration Management (internal/config/)

**XDG-Compliant Configuration**:

- Config location: `$XDG_CONFIG_HOME/comic-server/` or `~/.config/comic-server/`
- Supports both YAML and TOML formats (auto-detected by extension)
- Default file: `config.yaml` or `config.toml`

**Per-Device Sync Configuration**:

```go
type Config struct {
    Devices map[string]*DeviceConfig  // Keyed by device ID
}

type DeviceConfig struct {
    DeviceID        string
    FriendlyName    string
    LastSeen        time.Time
    Lists           []SharedListConfig       // Smart lists to sync
    DefaultSettings *sync.SharedListSettings // Device-wide defaults
}

type SharedListConfig struct {
    ListID   string                      // Smart list GUID
    ListName string                      // Cached name for display
    Enabled  bool                        // Allow disable without deleting
    Settings *sync.SharedListSettings    // Per-list overrides
}
```

**Device Resolution**:

- Devices can be identified by ID or friendly name
- Fuzzy matching: exact ID → exact name → partial name (case-insensitive)
- Disambiguates when multiple devices match

**Smart List Resolution**:

- Lists identified by GUID (ComicRackCE compatible)
- CLI accepts list names, stores GUIDs internally
- Ensures stability when list names change

**CLI Commands**:

```bash
# List available smart lists in library
comic-server config list-smartlists --library /path/to/ComicDb.xml

# List configured devices
comic-server config list-devices

# Show device configuration
comic-server config show-device "My Tablet"

# Add smart list to device
comic-server config add-list "My Tablet" "Currently Reading" \
  --library /path/to/ComicDb.xml \
  --only-unread \
  --limit 50 \
  --limit-type books

# Update sync options for a list
comic-server config set-options "My Tablet" "Currently Reading" \
  --limit 100 \
  --sort series

# Remove list from device
comic-server config remove-list "My Tablet" "Currently Reading"
```

**v0.2 Limitations**:

- **Single-device sync**: Only one device can sync at a time (enforced by mutex)
- **Single list per device**: Only first enabled list is synced (v0.3 will support multiple)
- Concurrent sync support planned for v0.3

## Testing

All tests must pass before committing:

```bash
just ci  # Runs lint + test + build
```

### Test Organization

**Unit Tests** - Component-level testing:

- `internal/protocol/protocol_test.go`: Binary encoding/decoding (21 tests)
- `internal/device/discovery_test.go`: Message parsing (11 tests)
- `internal/device/info_test.go`: INI parsing and validation (multiple test suites)
- `internal/library/library_test.go`: Library XML parsing (11 test suites, 24 test cases)
- `internal/library/smartlist_test.go`: Smart list matcher evaluation (100+ test cases)
- `internal/sync/sync_test.go`: Sync operation planning and execution
- `internal/sync/smartlist_test.go`: Smart list filtering integration with sync
- `internal/config/config_test.go`: Configuration structures and operations (10 tests)
- `internal/config/loader_test.go`: YAML/TOML loading and saving (9 tests)
- `internal/config/helpers_test.go`: Device and smart list resolution (9 tests)
- `internal/config/xdg_test.go`: XDG directory resolution (2 tests)

**Integration Tests** - End-to-end workflow testing:

- `internal/protocol/integration_test.go`: Client-server communication with mock device server
  - Tests file transfer operations (ReadFile, FileExists)
  - Tests device info retrieval (CommandInfo)
  - Tests connection handling, retries, and large file transfers
- `internal/device/integration_test.go`: Device discovery and registration workflows
  - UDP multicast discovery testing with mock broadcasters
  - Full device validation and registry management
  - End-to-end discovery → connect → validate → register workflow
- `internal/library/integration_test.go`: Real library parsing with testdata
  - Uses actual ComicDB.xml file from `testdata/` directory
  - Tests loading, parsing, and querying large comic libraries
  - Benchmarks library loading performance
- `internal/sync/integration_test.go`: Full sync session testing
  - Tests complete sync workflow with mock device
  - Tests error handling and progress reporting
  - Tests sync with smart list filtering

### Test Infrastructure

**Mock Device Server** (`internal/protocol/testserver.go`):

- Simulates a ComicRack device for testing
- Implements protocol commands (ReadFile, FileExists, CommandInfo)
- Supports in-memory file system for test data
- Used by integration tests to verify client-server communication

**Test Data**:

- `testdata/ComicDB.xml`: Real ComicRack library XML (226MB) with actual comic metadata
- Used for integration testing library parsing and querying

## Common Issues

### Multicast Not Working

- Check firewall: Ports 7615/udp, 7614/tcp, 7620/tcp, IGMP protocol
- Verify network interface supports multicast
- Ensure not running in WSL2 (use real Linux or Windows)

### Device Not Discovered

- Check device is broadcasting on network
- Use tcpdump to verify packets: `sudo tcpdump -i any -n udp port 7615`
- Ensure device broadcasts with "ComicRack" prefix

### Tests Failing

- Run `go mod tidy` to ensure dependencies are correct
- Check Go version: `go version` (should be 1.25.1)
- Run `mise install` to ensure tools are up to date

## Protocol Reference

See `WIRELESS_SYNC_PROTOCOL.md` for complete protocol specification including:

- All command codes and formats
- Device synchronization flow
- Error handling
- Extended protocol documentation

## Contributing

1. Keep changes focused and well-tested
2. Update tests when adding features
3. Maintain test coverage above 80%
4. Follow existing code style (use `just lint`)
5. Update this guide when adding major features

## Smart List Filtering

**Status**: Phase 1 complete (basic filtering), Phases 2-5 in roadmap

The server implements ComicRack-compatible smart list filtering for syncing subsets of large libraries to devices with limited storage.

### Implemented (Phase 1)

**Smart List Matcher Evaluation** (`internal/library/smartlist.go`):

- 20+ matcher types (Series, Publisher, Year, Genre, PageCount, Rating, etc.)
- String operators: Equals, Contains, ContainsAny, ContainsAll, StartsWith, EndsWith, Regex
- Numeric operators: Equals, Greater, Lesser, InRange
- Date operators: Equals, IsAfter, IsBefore, IsInLastDays, IsInDateRange
- Boolean operators: EqualsYes, EqualsNo, EqualsUnknown
- AND/OR logic with negation support
- Case-insensitive string matching

**Sync Integration** (`internal/sync/`):

```go
syncer := sync.NewSyncer(client, library)
syncer.SetFilterList(smartList)  // Filter by smart list
syncer.PerformSync()              // Only matching books sync
```

Books matching the filter are synced to device. Books on device but not in filter are automatically removed.

### Roadmap (Phases 2-5)

See issues #15, #16, #17 for:

- Per-list sync options (OnlyUnread, Limit, Sort, KeepLastRead, etc.)
- Per-device configuration storage (multiple lists per device)
- CLI/API for configuration management
- Advanced matchers (nested groups, file path matching, etc.)

## Status

✅ Completed:

- Binary protocol implementation
- UDP multicast device discovery
- Device registry and validation
- TCP command handlers
- Device ignore/filter functionality
- Library XML parsing (ComicRackCE ComicDb.xml format)
- Basic sync logic (metadata only, file transfers stubbed)
- Smart list filtering (Phase 1: evaluation engine + basic sync integration)
- Per-list sync options (OnlyUnread, Limit, Sort, etc.) - Issue #15 ✓
- Per-device sync configuration storage - Issue #17 ✓
- CLI commands for sync configuration management - Issue #17 ✓
- Development tooling (mise + just)

🚧 In Progress (v0.2 Milestone):

- Complete sync implementation (file transfers, reading lists) - Issue #4

📋 Backlog (v0.3+):

- Concurrent sync support (multi-device) - Issue #18
- SQLite storage investigation - Issue #19
- Multiple smart lists per device - Issue #16
- Daemon/service mode - Issue #11
- Web UI for monitoring - Issue #12
