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

### Direct IP Ping (Bypass Multicast Discovery)

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

### SQLite Storage (internal/storage/)

**Experimental feature** for importing ComicRack libraries into SQLite for improved query performance.

**Import Command**:

```bash
# Import library into SQLite database
comic-server import --xml /path/to/ComicDb.xml --db /path/to/library.db

# Preview changes without modifying database
comic-server import --xml /path/to/ComicDb.xml --db /path/to/library.db --dry-run

# Verbose output
comic-server import --xml /path/to/ComicDb.xml --db /path/to/library.db --verbose
```

**Database Info Command**:

```bash
# Show database statistics
comic-server db info --db /path/to/library.db
```

**Key Features**:

- **Idempotent import**: Importing the same XML twice results in no changes
- **Change detection**: Uses SHA256 hashing to detect modified records
- **Automatic updates**: Adds new books, updates changed books, deletes removed books
- **Normalized storage**: Custom values and tags stored in separate tables for fast queries
- **Pure Go**: Uses `modernc.org/sqlite` (no CGO required)

**Schema Highlights**:

- `books` table with 40+ columns matching ComicBook struct
- `book_custom_values` table (book_id, key, value) with index on (key, value)
- `book_tags` table (book_id, tag) with index on tag
- `lists` table with JSON matchers for smart list definitions
- `reading_list_items` junction table for reading list references
- `library_metadata` table for library ID, name, import timestamp

**Note**: This feature is experimental. The server currently still uses the in-memory XML library for sync operations. Future versions will add the option to read directly from SQLite.

### Safe File Replace (internal/trash/)

Foundation infra (comic-server-rhe) for any feature that overwrites or deletes a comic archive file on disk. First (and currently only) caller is CBZ conversion (comic-server-43b, see below). See comic-server-1up for the full design record.

**Why this exists**: comic-server had never written to or deleted a comic file before CBZ conversion - only read them. ComicRack's own "Convert to CBZ" replaces files in place, but on Windows the deleted original goes to the Recycle Bin and is recoverable. Linux (comic-server's actual deployment target) has no equivalent, so a plain `os.Remove` would be strictly riskier than what ComicRack already does.

**Three primitives**, all on `*Trash`:
- `Replace(targetPath, write, validate)` - same-path swap: write to a temp file in `targetPath`'s own directory (guarantees a same-filesystem atomic rename), validate it, quarantine whatever's currently at `targetPath` (not deleted), then atomically rename the temp file into place. On a failed final rename, best-effort moves the quarantined original back rather than leaving `targetPath` missing.
- `WriteNew(targetPath, write, validate)` - same write/validate/atomic-rename mechanics as `Replace`, but for a path with nothing at it yet; errors if `targetPath` already exists rather than silently overwriting.
- `Quarantine(path)` - moves an existing file into quarantine on its own, no replacement written.

`Replace` alone isn't enough for a format conversion where the new file's path differs from the old one's (e.g. `.cbr` -> `.cbz` - see comic-server-43b below for why this split exists): that case pairs `WriteNew` for the new path with a separate `Quarantine` for the old one.

**Cleanup** (`Trash.Sweep` / `Trash.Run`): a background sweep (hourly by default, `trash.DefaultSweepInterval`) permanently deletes quarantined files older than `TrashRetentionDays`, then prunes any directories left empty. Started in `cmd/server.go` whenever `trash_path` is configured, independent of which specific feature is enabled.

**Configuration** (`ServerConfig`):
- `trash_path` / `COMIC_SERVER_TRASH_PATH` - quarantine directory. Empty disables any feature that needs it (checked by `CBZConvertConfig.Validate`). Should be under a mounted volume in Docker deployments, same reasoning as `cover_cache_dir`.
- `trash_retention_days` / `COMIC_SERVER_TRASH_RETENTION_DAYS` - default 30.

### Convert to CBZ (internal/cbzconvert/)

Port of ComicRack's native "Convert to CBZ" feature (comic-server-43b, researched in comic-server-pkk.2): repacks a comic archive's pages as-is into a new CBZ and embeds a `ComicInfo.xml` built from the book's current library metadata. comic-server's first feature that writes to and retires a comic archive file - built on `internal/trash` above.

**What it does** (`cbzconvert.Convert`):
1. Reads every image page from the source archive (`comicvine.ReadAllPages` - CBZ/CBR/CB7, shares the format-dispatch machinery `ExtractCover` already used for cover thumbnails).
2. Drops any page the library marks `PageTypeDeleted` (matches ComicRack's own `RemovePages`/`RemovePageFilter` defaults - confirmed from `StorageProvider.GetImages` in ComicRackCE's real source, not assumed). Remaining pages are **not** reordered - export sorts strictly by original page index (`IndexedPageResult.CompareTo` in ComicRackCE), never by page `Type`; a front cover that isn't page 0 stays wherever it is. (An earlier read of this feature assumed front covers get sorted to the top on export - checked directly against the source and that's not real; the "front cover jumps to the top" behavior only exists in ComicRack's page-editor UI, not the export pipeline.)
3. Builds `ComicInfo.xml` from the book's current metadata (`BuildComicInfoXML`) with `PageCount` reflecting the post-filter page count.
4. Writes the new CBZ (`archive/zip`, `zip.Store` for image entries since they're already-compressed formats) and retires the original via `internal/trash` - `Replace` if the new file lands at the same path (source was already `.cbz`), or `WriteNew` + `Quarantine` if the extension changed.
5. Returns the new raw `FilePath` and page count for the caller to apply to the library record - `Convert` itself never touches the backend, same division of responsibility as `internal/scaninfo.DetectTag`.

**Explicitly out of scope** (matches ComicRack's own defaults, not built): page re-encoding/resizing/rotation, `ComicBook.xml` embedding (`EmbedComicBook`, off by default in ComicRack too), combining multiple books into one archive.

**Trigger**: `POST /api/library/lists/:listId/convert-cbz` - runs over every book currently matched by one smart list, mirroring the user's real ComicRack workflow (multi-select a smart list's results, invoke Convert to CBZ) and comic-server-pkk.1's own scan-info endpoint pattern. A "Convert this list" button on the list detail page calls it, behind a `confirm()` prompt (destructive-adjacent, unlike scan-info's metadata-only write).

**Configuration** (`ServerConfig.CBZConvert`):
- `cbz_convert.enabled` - off by default, same precedent as `scan_info.enabled`. `Config.Validate` rejects `enabled: true` without `server.trash_path` also set - this feature cannot run without a quarantine directory.

### Configuration Management (internal/config/)

**XDG-Compliant Configuration**:

- Config location: `$XDG_CONFIG_HOME/comic-server/` or `~/.config/comic-server/`
- Supports both YAML and TOML formats (auto-detected by extension)
- Default file: `config.yaml` or `config.toml`

**Configuration Hierarchy**:

Priority (highest to lowest):
1. CLI flags (e.g., `--port 8080`)
2. Environment variables (e.g., `COMIC_SERVER_PORT=8080`)
3. Config file values
4. Built-in defaults

**Global Server Settings**:

```go
type Config struct {
    Server  ServerConfig              // Global server settings
    Devices map[string]*DeviceConfig  // Per-device configurations
}

type ServerConfig struct {
    // Library settings
    LibraryPath string  // Path to ComicDb.xml

    // Network settings
    ServerPort    int    // TCP control port (default: 7620)
    DiscoveryPort int    // UDP multicast port (default: 7615)
    BindAddress   string // Network interface to bind (default: all)

    // Device filters
    IgnoreDevices []string  // Device IPs/IDs/names to ignore

    // Sync settings
    AutoSync         bool  // Auto-sync when devices connect
    MaxConcurrentSync int  // Max concurrent syncs (0 = unlimited)

    // Logging settings
    LogLevel  string  // debug, info, warn, error (default: info)
    LogFormat string  // text, json (default: text)
}
```

**Per-Device Sync Configuration**:

```go
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

**Environment Variables**:

All server settings can be configured via environment variables:

- `COMIC_SERVER_LIBRARY_PATH` - Library path
- `COMIC_SERVER_PORT` - Server control port
- `COMIC_SERVER_DISCOVERY_PORT` - Discovery port
- `COMIC_SERVER_BIND_ADDRESS` - Network bind address
- `COMIC_SERVER_IGNORE_DEVICES` - Comma-separated list of devices to ignore
- `COMIC_SERVER_AUTO_SYNC` - Enable auto-sync (true/false)
- `COMIC_SERVER_MAX_CONCURRENT_SYNC` - Max concurrent syncs
- `COMIC_SERVER_LOG_LEVEL` - Log level (debug/info/warn/error)
- `COMIC_SERVER_LOG_FORMAT` - Log format (text/json)
- `COMIC_SERVER_TRASH_PATH` - Quarantine directory for internal/trash (see below)
- `COMIC_SERVER_TRASH_RETENTION_DAYS` - Days a quarantined file is kept before permanent deletion (default: 30)

**Example Configuration File**:

```yaml
# Global server settings
server:
  library_path: /home/user/.local/share/ComicRack/ComicDb.xml
  server_port: 7620
  discovery_port: 7615
  bind_address: ""  # Empty = bind to all interfaces
  ignore_devices:
    - 192.168.0.24  # Production tablet (prevent accidental sync)
  auto_sync: false
  max_concurrent_sync: 0  # 0 = unlimited
  log_level: info
  log_format: text
  trash_path: ""  # Empty = disabled; required if cbz_convert.enabled (see internal/trash)
  trash_retention_days: 30
  cbz_convert:
    enabled: false  # Requires trash_path to be set

# Per-device configurations
devices:
  device-abc123:
    device_id: device-abc123
    friendly_name: My Tablet
    last_seen: 2025-01-15T14:30:25Z
    lists:
      - list_id: "{GUID-1234-5678}"
        list_name: Currently Reading
        enabled: true
        settings:
          only_unread: true
          limit: true
          limit_value: 50
          limit_value_type: books
          sort: true
          list_sort_type: series
    default_settings:
      only_unread: false
      limit: false
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

**Current Limitations**:

- **Single-device sync**: Only one device can sync at a time (enforced by mutex)
- Concurrent sync support planned for v1.0

## Structured Logging

The server uses **zerolog** for structured, high-performance logging with zero allocations.

**Logger Initialization** (`internal/log/log.go`):

```go
import "github.com/duckpuppy/comic-server/internal/log"

// Initialize logger (called at server startup)
log.Init(log.Config{
    Level:  "info",      // debug, info, warn, error
    Format: "text",      // text (colorized) or json (machine-parseable)
    Output: "stdout",    // stdout, stderr, or file path
})

// Use convenience functions
log.Info().Msg("Server starting")
log.Debug().Str("path", libraryPath).Msg("Loading library")
log.Error().Err(err).Msg("Failed to connect")
```

**Contextual Logging**:

```go
// Create logger with device context
logger := log.With().
    Str("device_id", deviceID).
    Str("device_ip", deviceIP).
    Logger()

logger.Info().Msg("Device discovered")
logger.Debug().Int("books", len(books)).Msg("Sync started")
```

**Log Levels**:

- `debug`: Verbose information for troubleshooting
- `info`: General operational events (default)
- `warn`: Warning conditions
- `error`: Error conditions

**Log Formats**:

- `text`: Human-readable with colors and timestamps (development)
- `json`: Machine-parseable JSON (production, monitoring)

**Configuration**:

Logging can be configured via:
- CLI flags: `--log-level debug --log-format json`
- Environment: `COMIC_SERVER_LOG_LEVEL=debug`
- Config file: `server.log_level: debug`
- SIGHUP reload: Change config and send SIGHUP to reload without restart

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
- `internal/log/log_test.go`: Logging initialization and output (7 tests)

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
- `testdata/library/`: Minimal test library for rapid development (see Test Library section below)

### Test Library

**Location**: `testdata/library/`

A minimal test library for rapid development and testing without requiring a production library (65K+ books).

**Contents**:
- **3 comic books** - Small files (league-001.cbz, league-002.cbz, kids-club-002.cbz)
- **4 smart lists** - Pre-configured to match the test comics
- **ComicDb.xml** - Complete ComicRack-compatible library database
- **Documentation** - Comprehensive README with usage examples

**Comic Books** (user-provided, not in git):

1. **The League of Extraordinary Gentlemen: Century #1** (2009) - 80 pages, Top Shelf, Superhero
2. **The League of Extraordinary Gentlemen: Century #2** (2011) - 80 pages, Top Shelf, Superhero
3. **Top Shelf Kids Club #2** (2012) - 48 pages, Top Shelf, Kids

**Smart Lists**:

1. **League Series** - Matches 2 books (Series contains "League")
2. **Top Shelf Publisher** - Matches 3 books (Publisher equals "Top Shelf")
3. **2009-2011** - Matches 2 books (Year in range 2009-2011)
4. **All Comics** - Matches 3 books (PageCount > 0)

**Usage Examples**:

```bash
# Run server with test library
./comic-server server --library testdata/library/ComicDb.xml

# List smart lists
./comic-server config list-smartlists --library testdata/library/ComicDb.xml

# Add list to device
./comic-server config add-list "My Tablet" "League Series" \
  --library testdata/library/ComicDb.xml

# With direct IP ping (bypass multicast)
./comic-server server --library testdata/library/ComicDb.xml \
  --ping-device 192.168.0.24
```

**Advantages**:
- **Fast**: 3 books vs 65K+ books - sync completes in seconds
- **Safe**: No risk of corrupting production library during client-to-server sync tests
- **Portable**: ~6MB total (excluding comic files) - easy to commit to git
- **Predictable**: Known content makes debugging easier
- **Complete**: Real comic files with actual pages, not mocks

**Comic Files**:
- Comic files are **excluded from git** via `.gitignore`
- Users provide their own comics for testing (see `testdata/library/comics/README.md`)
- Recommended sources for public domain comics:
  - [Comic Book Plus](https://comicbookplus.com/) (free registration required)
  - [Digital Comic Museum](https://digitalcomicmuseum.com/)
  - [Internet Archive](https://archive.org/details/comics)

**File Structure**:

```
testdata/library/
├── README.md           # Comprehensive usage guide
├── ComicDb.xml         # Library database with books and smart lists
└── comics/             # Comic book files (user-provided)
    ├── .gitignore      # Excludes *.cbz, *.cbr, *.cb7, *.cbt
    ├── README.md       # Comic sources and setup instructions
    ├── league-001.cbz  # User provides
    ├── league-002.cbz  # User provides
    └── kids-club-002.cbz  # User provides
```

**Testing Scenarios**:

1. **Fresh Device Sync** - Configure "All Comics" smart list → sync 3 books
2. **Partial Sync** - Configure "League Series" smart list → sync 2 books
3. **Year Range Filter** - Configure "2009-2011" smart list → sync 2 books (excludes 2012)
4. **Publisher Filter** - Configure "Top Shelf Publisher" smart list → sync 3 books

**Integration with Tests**:

```go
func TestSyncWithSmartList(t *testing.T) {
    lib, err := library.LoadLibrary("testdata/library/ComicDb.xml")
    // Test sync logic with known data
}
```

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

## ComicVine Enrichment

Background sync pipeline that fetches series/issue data from the ComicVine API to enable series completeness checking and missing issue detection.

### How It Works

1. **Seeding**: On server startup, extracts `comicvine_volume` IDs from library books' `CustomValuesStore` fields (~97.5% of books are tagged)
2. **Background sync**: Fetches volume metadata and issue lists from ComicVine API, prioritized by how many issues you own per volume
3. **Completeness computation**: Compares owned issues against ComicVine's issue list to determine completeness percentage and missing issues
4. **Matcher evaluation**: CV enrichment matchers use the pre-computed data during smart list evaluation

### Configuration

```yaml
server:
  comicvine_api_key: "your-api-key-here"
```

The API key can also be set via `COMIC_SERVER_COMICVINE_API_KEY` environment variable.

### Rate Limiting

ComicVine's API is burst-hostile — sustained requests at any rate will eventually trigger timeouts. The client uses:

- **Burst-and-pause pacing**: 20 requests, then 5-minute pause. After 200 requests, 30-minute cooldown.
- **Circuit breaker**: On ANY failure (timeout, 429, 5xx), immediately stops all requests. Minimum 1-hour backoff with exponential increase up to 24-hour cap.
- **Resumable sync**: Progress tracked in SQLite. Survives server restarts, backoff pauses, and interruptions.

Initial sync of ~12K volumes takes approximately 2 weeks at conservative pacing.

### Cache Storage

Data cached in SQLite at `$XDG_DATA_HOME/comic-server/comicvine_cache.db` (default: `~/.local/share/comic-server/comicvine_cache.db`).

Tables:

- `cv_volumes` — Volume metadata (name, publisher, issue count, sync status)
- `cv_issues` — Issue list per volume (number, cover date, store date)
- `cv_sync_state` — Key-value store for sync state tracking

### CLI Commands

```bash
# Show sync status
comic-server comicvine status
```

### Smart List Matchers (comic-server only)

These matchers are comic-server extensions — they won't be recognized by ComicRack.

| XML Type | Matcher | Operators | Description |
|----------|---------|-----------|-------------|
| `ComicServerCVSeriesCompleteMatcher` | CVSeriesComplete | Yes/No/Unknown | Whether you own all known issues |
| `ComicServerCVMissingCountMatcher` | CVMissingCount | Numeric (=, >, <, range) | Number of missing issues |
| `ComicServerCVPercentOwnedMatcher` | CVPercentOwned | Numeric (=, >, <, range) | Percentage of series owned (0-100) |

Books without ComicVine tags or with volumes not yet synced evaluate as "Unknown" for CVSeriesComplete and don't match numeric CV matchers.

### Expression Matcher (`ComicBookExpressionMatcher`)

ComicRack-compatible (same XML type), but a best-effort approximation, not full parity. ComicRackCE evaluates arbitrary Python via IronPython; comic-server evaluates the same `MatchValue` expression via [Starlark](https://github.com/google/starlark-go) (`go.starlark.net`), a Python-like language with a pure-Go implementation — no CGO, no external Python dependency.

- `MatchValue` is a raw expression referencing `ComicBook` field names directly (e.g. `Rating > 3 and "X-Men" in Series`), exposed as Starlark globals via reflection (`internal/library/expression.go`).
- Operator `0` = "is true", operator `1` = "is false" (negates the result) — matches `ComicBookExpressionMatcher.MatchBook`.
- Works: field access, comparisons, boolean logic (`and`/`or`/`not`), string methods (`.lower()`, `in`), simple arithmetic.
- Does not work: Python stdlib (`datetime`, `re`, etc.), Python-3-only syntax, series-stats access (`__bookStats`) — series aggregates are ComicRackCE's own separate `SmartListSeries*Matcher` types, already implemented natively in comic-server (see Series aggregate matchers above), not something Expression needs to reach into.
- A parse or eval failure never matches (mirrors ComicRack's own error handling) rather than aborting the whole smart list.

**Known gap**: `ComicBookVirtualTagMatcher` (VirtualTag01-20) is not ported — tracked as `comic-server-65u`. Unlike other matchers, VirtualTags aren't stored per-book in the library XML; they're computed at runtime from a `CaptionFormat` template defined in a desktop-UI-configured collection that has no equivalent in comic-server's config today.

### Architecture

```
internal/comicvine/
├── client.go        # API client with burst-and-pause rate limiting
├── circuit.go       # Circuit breaker (closed→open→half-open state machine)
├── pacer.go         # Burst/window rate limiting with context cancellation
├── cache.go         # SQLite cache (cv_volumes, cv_issues, cv_sync_state)
├── sync.go          # Background sync orchestrator (seed → fetch → cache)
├── completeness.go  # Builds book ID → CVCompleteness map from cache
└── types.go         # API response types and cached data types
```

### Duplicate Matcher

ComicRack-compatible duplicate detection across the entire candidate set. Identifies comics that are duplicates by metadata (series/format/volume/number/language/year/month/day/BlackAndWhite) or by file path.

- XML type: `ComicBookDuplicateMatcher`
- Operator 0 (On): returns only duplicate books
- Operator 1 (Off): returns all books
- `Not` flag inverts the result (e.g., NOT + On = non-duplicates only)
- Series name comparison strips articles ("the", "der", "die", etc.) and common separators, matching ComicRackCE's `GroupInfo.CompressedName`

## Status

### v0.2 Milestone - Complete! ✅

- Binary protocol implementation
- UDP multicast device discovery
- Device registry and validation
- TCP command handlers
- Device ignore/filter functionality
- Library XML parsing (ComicRackCE ComicDb.xml format)
- Smart list filtering (Phase 1: evaluation engine + basic sync integration)
- Per-list sync options (OnlyUnread, Limit, Sort, etc.) - Issue #15 ✓
- Per-device sync configuration storage - Issue #17 ✓
- CLI commands for sync configuration management - Issue #17 ✓
- Global server configuration (library path, network, logging, etc.) - Issue #5 ✓
- Environment variable support - Issue #5 ✓
- Configuration validation and defaults - Issue #5 ✓
- Complete sync implementation - Issue #4 ✓
  - Comic book file transfers (.cbp files)
  - Sidecar XML metadata (.cbp.xml files)
  - Reading list sync (sync_information.xml)
  - Free space validation
  - Progress tracking and abort handling
  - Automatic sync on device discovery (when enabled)
- Development tooling (mise + just)

### v0.3 Milestone - Complete! ✅

- Structured logging with zerolog - Issue #6 ✓
  - Log levels: debug, info, warn, error
  - Text format (colorized, human-readable)
  - JSON format (machine-parseable)
  - Contextual logging with structured fields
  - Configurable via CLI, environment, config file
- Per-device sync configuration storage - Issue #16 ✓
  - YAML/TOML configuration files
  - Device-specific smart list assignments
  - Per-list sync settings overrides
  - Device and list resolution helpers
- Daemon/service mode - Issue #11 ✓
  - SIGHUP signal handling for config reload
  - systemd service file with security hardening
  - launchd plist for macOS
  - Comprehensive service installation documentation

### v0.4 Milestone - Planned (Security & Multi-List Support):

- ✅ Multiple smart lists per device - Issue #21 (COMPLETED)
  - Removed single-list limitation
  - Syncs all enabled lists per device (union of all lists)
  - Automatically deduplicates books appearing in multiple lists
  - Backward compatible with single-list SetFilterList API
- ✅ Security hardening - Phase 1 & 2 - Issue #10 (COMPLETED)
  - ✅ Phase 1: Input Validation
    - Device authentication and SHA1 hash verification
    - Device registry with tracked devices
    - Configurable device ignore/filter lists
  - ✅ Phase 2: Rate Limiting and Resource Management
    - IP-based rate limiting (sliding window algorithm)
    - Device-based rate limiting (token bucket algorithm)
    - Concurrent connection limiting (semaphore-based)
    - CLI flags and configuration support
    - 22 comprehensive tests with full coverage
    - SECURITY.md documentation
  - 🚧 Phase 3: Advanced Security (Planned)
    - TLS/SSL encryption for device communication
    - Certificate-based device authentication
    - Enhanced logging and audit trails
    - Intrusion detection capabilities

### v0.5 Milestone - Complete! ✅

- ✅ Concurrent sync support (multi-device) - Issue #18 (COMPLETED)
  - Semaphore-based concurrent connection limiting
  - Per-device sync state tracking with `syncstate.Manager`
  - Prevents same device from syncing twice simultaneously
  - Removed old mutex-based single-device sync limitation
  - Thread-safe sync progress tracking
  - Comprehensive sync state manager tests (12 tests)
- ✅ REST API for monitoring and control (COMPLETED)
  - HTTP server on configurable port (default: 7620)
  - `GET /api/health` - Health check with uptime and version info
  - `GET /api/version` - Build version information
  - `GET /api/sync/status` - Get all active syncs
  - `GET /api/sync/history?limit=N` - Get recent sync history
  - `GET /api/devices` - List all registered devices with sync status
  - `GET /api/stats` - Server statistics and configuration
  - `GET /metrics` - Prometheus metrics endpoint
  - JSON response format for all endpoints
  - Graceful shutdown with 5-second timeout
  - Comprehensive API tests (10 tests)
- ✅ Prometheus metrics integration (COMPLETED)
  - `comic_server_syncs_total` - Counter for sync operations by status
  - `comic_server_active_syncs` - Gauge for current active syncs
  - `comic_server_books_processed_total` - Counter for books by operation (added/updated/deleted)
  - `comic_server_sync_duration_seconds` - Histogram for sync durations
  - Metrics recorded at all sync lifecycle stages (start, complete, fail, abort)
  - Standard Go runtime metrics included automatically

### v0.7 Milestone - Complete! ✅

- ✅ Web UI for server monitoring (COMPLETED)
  - Real-time dashboard with WebSocket updates
  - Device management (registration/unregistration)
  - Sync progress monitoring with progress bars
  - Sync history with file statistics
  - Responsive design with vanilla JavaScript
  - Static file serving with Go embed package
  - WebSocket hub for broadcasting events to all connected clients
  - REST API integration for device and sync operations

### v0.8 Milestone - Complete! ✅

- ✅ Reverse sync (device-to-server metadata sync) (COMPLETED)
  - Syncs reading progress from device to library (CurrentPage, LastPageRead, OpenCount, OpenedTime)
  - Syncs user metadata from device to library (Rating, Notes, Review, Summary, Tags)
  - Syncs Checked flag from device to library
  - Syncs page metadata from device to library (bookmarks, page types)
  - Automatic library save after metadata updates
  - Optimization: skips library save when no changes detected
  - Handles edge cases (missing books, no metadata, no library path)
  - Comprehensive test suite (10 test functions in `reverse_sync_test.go`)
  - Manual testing guide (`TESTING_REVERSE_SYNC.md`)
  - Integrated into sync workflow (runs before forward sync to preserve device changes)

### v0.6 Milestone - Complete! ✅

- ✅ Graceful device disconnect handling (COMPLETED)
  - Network error detection in `internal/protocol/errors.go`
  - Detects connection issues: timeouts, refused/reset/aborted connections, broken pipes, unreachable hosts
  - Differentiates network errors from application logic errors
  - Proper sync state updates with specific error messages
  - Connection timeout detection with configurable deadlines
  - Comprehensive error detection tests (18 test cases)
  - Enhanced logging with error type classification

- ✅ Pagination support for sync history endpoint (COMPLETED)
  - Added `GetHistoryPaginated` method to `syncstate.Manager`
  - Offset-based pagination with metadata (total, offset, limit, has_more, next_offset)
  - Backward-compatible API: `/api/sync/history?limit=N` (legacy) vs `/api/sync/history?limit=N&offset=M` (paginated)
  - Default page size: 20, max: 100
  - Returns pagination metadata: total count, has_more flag, next_offset pointer
  - Comprehensive tests for manager (4 test functions) and API (2 test functions)
  - Handles edge cases: empty history, offset beyond end, limit validation

- ✅ Device list filtering (COMPLETED)
  - Filter by edition: `?edition=Android+Full`, `?edition=Android+Free`, or `?edition=iOS`
  - Filter by sync status: `?syncing=true` or `?syncing=false`
  - Filter by last seen: `?last_seen_after=2024-01-01T00:00:00Z` (RFC3339 format)
  - Combine multiple filters with AND logic
  - Returns 400 Bad Request for invalid timestamp format
  - Comprehensive tests (5 test functions): edition, syncing, timestamp, combined, invalid input
  - Example: `/api/devices?edition=Android+Full&syncing=true` returns only syncing Android Full devices

### v1.0 Milestone - Complete! ✅ 🎉

**Production Ready Release** - All core features implemented, tested, and optimized

- ✅ **Performance Optimization - Issue #9** (COMPLETED)
  - Phase 1: Dirty tracking and batch saves
  - Phase 2: In-memory cache with periodic flush (30s default)
  - Phase 3: Large library profiling and validation (64K+ books)
  - **Results**: 10.3x performance improvement, 10x memory reduction
  - Comprehensive Prometheus metrics for monitoring
  - Performance regression tests in CI
  - See `docs/PERFORMANCE_ANALYSIS.md` and `docs/LARGE_LIBRARY_BENCHMARK.md`

- ✅ **User Documentation - Issue #8** (COMPLETED)
  - Comprehensive documentation in `docs/` directory
  - Installation guide with platform-specific instructions
  - Configuration reference covering all options
  - Complete API reference with examples
  - Troubleshooting guide with common issues and solutions
  - Architecture documentation

**Production-Ready Features:**
- Bidirectional sync (device ↔ server)
- Smart list filtering with per-device configuration
- Web UI with real-time monitoring
- REST API with Prometheus metrics
- Rate limiting and security hardening
- Graceful error handling and network resilience
- Comprehensive test coverage (200+ tests)
- Automated release pipeline (GoReleaser + Release Please)

**Performance:**
- Large library support (64K+ books, 223 MB)
- 10x faster saves with cache batching
- Sub-second sync overhead (amortized)
- Excellent memory efficiency

**v1.0 Status**: Production ready for deployments of any size! 🚀

### 📋 Backlog (v1.1+):

**Future Enhancements:**

- Security Phase 3: TLS/SSL encryption, certificate-based auth
- SQLite storage investigation - Issue #19
- Additional sync options and filters
- ComicVine enrichment: series completeness matchers (in progress - Issue comic-server-4hb)
- ComicVine scraper: metadata tagging without ComicRack dependency
- LLM-powered smart list matchers (per-book classification via local Ollama)

### Web UI (v0.8)

**Access**: http://localhost:7620/ (served by the REST API server)

**Technology Stack**:
- Vanilla JavaScript (no framework dependencies)
- Client-side routing using History API
- WebSocket for real-time updates
- Go's embed package for static file serving
- Responsive CSS Grid/Flexbox layout

**Architecture** (`internal/api/web/`):
- `index.html` - Main dashboard structure with templates
- `css/style.css` - Modern, responsive styling with CSS custom properties
- `css/navigation.css` - Navigation tab styling
- `css/lists.css` - Smart lists browser and detail page styling
- `css/lists-tree.css` - Tree sidebar navigation styling
- `css/deviceDetail.css` - Device detail page styling
- `css/devicesBrowser.css` - Devices browser page styling
- `js/router.js` - Client-side router (History API, no framework)
- `js/navigation.js` - Navigation component with tabs
- `js/websocket.js` - WebSocket client with automatic reconnection
- `js/devices.js` - Device management and registration controls
- `js/deviceDetail.js` - Device detail page with sync history
- `js/devicesBrowser.js` - Devices browser page with filtering
- `js/sync.js` - Sync progress monitoring with progress bars
- `js/syncHistoryBrowser.js` - Sync history browser with pagination
- `js/listsBrowser.js` - Smart lists browser with search/filter
- `js/listDetail.js` - List detail page with preview
- `js/listsTree.js` - Tree sidebar navigation for smart lists
- `js/app.js` - Application initialization and route registration

**Client-Side Routing**:

The UI now features multi-page navigation with the following routes:

- `/` - Dashboard (overview with stats, devices, sync history)
- `/lists` - Smart lists browser with search and filtering
- `/lists/:listId` - List detail page with matchers, devices, and comic preview (with tree sidebar)
- `/devices` - Devices browser page with filtering and search
- `/devices/:deviceId` - Device detail page with sync history
- `/sync` - Sync history browser with pagination

**Routing Features**:
- Browser back/forward button support (popstate events)
- URL updates without page reload (pushState)
- Bookmarkable URLs for direct access
- Parameterized routes (e.g., `/lists/:listId`, `/devices/:deviceId`)
- Automatic route handling on page load

**Tree Sidebar Navigation**:

- **Persistent sidebar** on list detail pages showing library structure
- **Hierarchical display** of smart lists organized in folders
- **Collapsible folders** to navigate nested list structures
- **Active list highlighting** shows current list in the tree
- **State persistence** maintains expanded/collapsed folders across navigation
- **Click to navigate** instantly switch between lists without leaving detail view
- **Responsive design** toggles with hamburger menu on mobile

**Smart Lists as First-Class Entities**:

- **Lists Browser** (`/lists`):
  - Grid view of all smart lists
  - Search by list name (debounced 300ms)
  - Sort by name (A-Z, Z-A) or book count
  - Shows book count and matcher count per list
  - Click card to navigate to detail page

- **List Detail Page** (`/lists/:listId`):
  - Breadcrumb navigation
  - Human-readable matcher display
  - Device assignments section
  - Paginated comic preview (20 per page, max 100)
  - "Load More" button for additional previews
  - Navigate to assigned devices

**Performance Optimizations**:
- List count caching (15 min TTL) for large libraries (65K+ comics)
- Paginated comic previews to avoid loading entire lists
- Debounced search input (300ms delay)
- Cached list evaluation results

**Features**:
- **Real-time Dashboard** - Live server statistics (devices, syncs, uptime, WebSocket clients)
- **Navigation Tabs** - Top-level navigation with active state indicators
  - Badge counts for lists and devices
  - Responsive design (icons only on mobile)
- **Device Sidebar** - Shows all discovered devices with status indicators:
  - Connected (< 2 min since last_seen)
  - Idle (2-30 min)
  - Offline (> 30 min)
  - Syncing (during active sync)
- **Device Management**:
  - Register button for unregistered discovered devices
  - Unregister button with confirmation dialog
  - Device info: model, IP address, last seen timestamp
- **Sync Progress Monitoring**:
  - Active syncs with real-time progress bars
  - Current file being transferred
  - File counts (completed/total)
  - Data transferred and transfer speed
- **Sync History**:
  - Recent sync operations (last 10 by default)
  - File statistics (added/updated/deleted)
  - Duration and completion status
  - Timestamp with relative time display

**WebSocket Events** (`internal/websocket/`):
- `device_discovered` - New device found on network
- `device_connected` - Device connected to server
- `device_disconnected` - Device disconnected
- `device_registered` - Device added to registry
- `device_unregistered` - Device removed from registry
- `sync_started` - Sync operation began
- `sync_progress` - Progress update during sync
- `sync_completed` - Sync finished successfully
- `sync_failed` - Sync encountered error

**Browser Compatibility**:
- Modern browsers with WebSocket support
- ES6+ JavaScript features (classes, async/await, template literals)
- CSS Grid and Flexbox required
- History API support required
- Named capture groups in regex (ES2018)
- Tested: Chrome, Firefox, Safari (latest versions)

**Static File Serving**:
- Files embedded in binary using `//go:embed web` directive
- No external dependencies for deployment
- Served from root path `/` by API server
- Automatic content-type detection

## Release Process

### Automated Release Workflow

This project uses **GoReleaser** + **Release Please** for fully automated releases.

**Tools**:
- **Release Please**: Automates versioning and changelog generation based on Conventional Commits
- **GoReleaser**: Builds multi-platform binaries and creates GitHub releases
- **Docker**: Automated image builds for tagged releases

### How It Works

1. **Commit with Conventional Commits format**:
   ```bash
   git commit -m "feat: add new feature"
   git commit -m "fix: resolve bug"
   git commit -m "feat!: breaking change"
   ```

2. **Release Please creates PR automatically**:
   - Monitors commits on `main` branch
   - Creates/updates a release PR with:
     - Version bump (based on commit types)
     - Updated CHANGELOG.md
     - Version file updates

3. **Merge the release PR**:
   - Review the CHANGELOG
   - Merge the PR to `main`

4. **Release Please tags and creates GitHub Release**:
   - Creates a git tag (e.g., `v0.8.0`)
   - Creates GitHub Release with CHANGELOG

5. **GoReleaser builds and publishes artifacts**:
   - Multi-platform binaries (Linux/macOS/Windows, amd64/arm64)
   - Archives (.tar.gz, .zip)
   - Checksums
   - Attaches to GitHub Release

6. **Docker workflow builds container images**:
   - Triggers automatically on new tag
   - Publishes to GitHub Container Registry

### Commit Message Format

Follow [Conventional Commits](https://www.conventionalcommits.org/):

**Types**:
- `feat:` - New feature (minor version bump)
- `fix:` - Bug fix (patch version bump)
- `docs:` - Documentation changes
- `chore:` - Maintenance tasks
- `refactor:` - Code refactoring
- `test:` - Test changes
- `ci:` - CI/CD changes

**Breaking Changes**:
- Add `!` after type: `feat!:` or `fix!:`
- Or include `BREAKING CHANGE:` in commit body
- Triggers major version bump

**Examples**:
```bash
git commit -m "feat: add sync progress bar to web UI"
git commit -m "fix: resolve memory leak in sync manager"
git commit -m "feat!: change API response format

BREAKING CHANGE: The /api/devices endpoint now returns an object instead of an array."
```

### Local Testing

Test the release process locally without publishing:

```bash
# Validate configuration
goreleaser check

# Build snapshot (no publish)
goreleaser release --snapshot --clean --skip=publish

# Check artifacts
ls -lh dist/
```

### Manual Release (Emergency)

If automation fails, you can manually create a release:

```bash
# 1. Update CHANGELOG.md manually
# 2. Create and push a tag
git tag -a v0.8.0 -m "Release v0.8.0"
git push origin v0.8.0

# 3. GoReleaser will automatically build and publish
```

### Release Artifacts

Each release includes:
- **Binaries**: `comic-server_VERSION_OS_ARCH.tar.gz` (or .zip for Windows)
- **Checksums**: `checksums.txt` (SHA256 hashes)
- **Documentation**: README.md, CLAUDE.md, CHANGELOG.md included in archives
- **Docker Images**: `ghcr.io/duckpuppy/comic-server:VERSION`

### Configuration Files

- `.goreleaser.yml` - GoReleaser configuration
- `release-please-config.json` - Release Please configuration (v4 format)
- `.release-please-manifest.json` - Current version manifest
- `.github/workflows/release-please.yml` - Release Please workflow
- `.github/workflows/release.yml` - GoReleaser workflow
- `.github/workflows/docker-publish.yml` - Docker build workflow
- `CHANGELOG.md` - Auto-generated changelog


<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:ca08a54f -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd dolt push
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
