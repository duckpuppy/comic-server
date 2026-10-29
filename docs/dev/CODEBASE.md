# Codebase Tour

Package layout, the wire protocol basics, and the core sync-side packages (device discovery, protocol client, library loading).

Moved from `CLAUDE.md`. See the index there for the other developer reference files.

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
│   ├── comicvine/        # ComicVine API integration and enrichment
│   │   ├── client.go     # API client with burst-and-pause rate limiting
│   │   ├── circuit.go    # Circuit breaker with exponential backoff
│   │   ├── pacer.go      # Burst-and-pause rate limiter
│   │   ├── cache.go      # SQLite cache for volumes, issues, sync state
│   │   ├── sync.go       # Background sync orchestrator
│   │   ├── completeness.go # Series completeness computation
│   │   └── types.go      # API response and cache types
│   ├── device/           # Device discovery and management
│   │   ├── discovery.go  # UDP multicast listener
│   │   ├── info.go       # Device info parsing (INI format)
│   │   └── registry.go   # Device tracking and validation
│   ├── library/          # ComicRack library management
│   │   ├── library.go    # Library XML parsing
│   │   ├── time.go       # Custom time format handling
│   │   └── library_test.go  # Comprehensive tests
│   ├── log/              # Structured logging (zerolog)
│   │   ├── log.go        # Logger initialization and convenience functions
│   │   └── log_test.go   # Logging tests
│   ├── protocol/         # Binary protocol implementation
│   │   ├── protocol.go   # Encoding/decoding (big-endian)
│   │   └── client.go     # TCP client for device communication
│   ├── ratelimit/        # Rate limiting infrastructure
│   │   ├── ip_limiter.go      # IP-based rate limiting (sliding window)
│   │   ├── device_limiter.go  # Device-based rate limiting (token bucket)
│   │   └── *_test.go          # Rate limiter tests (22 tests)
│   ├── sync/             # Synchronization logic
│   │   ├── sync.go       # Sync orchestration
│   │   ├── settings.go   # Sync settings (OnlyUnread, Limit, Sort, etc.)
│   │   └── ...
│   ├── syncstate/        # Sync state tracking
│   │   ├── manager.go    # Thread-safe sync state manager
│   │   └── manager_test.go  # State manager tests (12 tests)
│   ├── storage/          # SQLite database storage
│   │   ├── db.go         # Database connection management
│   │   ├── schema.go     # Table definitions and migrations
│   │   ├── import.go     # Idempotent XML import logic
│   │   └── import_test.go  # Import tests (10 tests)
│   └── api/              # REST API for monitoring
│       ├── api.go        # HTTP handlers and server
│       └── api_test.go   # API endpoint tests (9 tests)
├── scripts/               # Service/daemon files
│   ├── README.md          # Service installation guide
│   ├── comic-server.service  # systemd service file
│   └── com.comic-server.plist  # launchd plist
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

## Device Ignore Functionality

To protect production devices during development:

```bash
# In justfile: run-dev task ignores 192.168.0.24
just run-dev

# Manual ignore by IP, ID, or name:
./comic-server server --library /path \
  --ignore-device 192.168.0.24 \
  --ignore-device "SM-T970"
```

## Direct IP Ping (Bypass Multicast Discovery)

In environments where multicast discovery is unreliable (WSL2, VPNs, firewalls, complex network setups), you can send a direct discovery ping to a device:

```bash
# Ping device at specific IP (default port 7614)
./comic-server server --library /path \
  --ping-device 192.168.0.24

# Custom port
./comic-server server --library /path \
  --ping-device 192.168.0.24:7614
```

The `--ping-device` flag:
- Sends `CommandClientPong` directly to device IP:PORT
- Makes the sync button appear on the device without multicast
- Runs in background goroutine (doesn't block server startup)
- Defaults to port 7614 if not specified
- Useful for WSL2, VPNs, firewalls blocking multicast, or complex network topologies

## Syncing a device that is not on the local network

Multicast discovery does not cross Tailscale or other VPNs. Give the device a manual address (Device settings, Address) and use Sync Now: the server connects to that address directly. See "Manual device address" in [CONFIG_AND_LOGGING.md](CONFIG_AND_LOGGING.md). The device must listen on the VPN address too (the new ComicRack CE app does), and a full sync can be tens of GB.

### Surviving a lost link mid-sync (tracker #57)

`PerformSync` no longer stops at the first failed abort check. `waitForDevice` (`internal/sync/session.go`) pauses 2, 4, 8, 16, then 30 s and probes with `CheckAbort`, for up to `defaultReconnectWindow` (10 min, `Syncer.SetReconnectWindow`). If an operation fails while the device is unreachable, the same operation is redone after the device is back; `addBook` deletes a leftover partial file first, so a half-written book cannot stay. The same wait covers `SendCompleted`. Client-level retries stay at 1 on purpose: a retried `WriteFile` resends the whole file to a device that may append. A device reached through its manual address also gets `Client.UseSlowLink` (30 s I/O, 10 s connect, 128 KB/s minimum transfer speed for sizing a book's deadline). There is no web Abort route: only the device's own abort answers `CheckAbort`.

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

## Protocol Reference

See `WIRELESS_SYNC_PROTOCOL.md` for complete protocol specification including:

- All command codes and formats
- Device synchronization flow
- Error handling
- Extended protocol documentation
