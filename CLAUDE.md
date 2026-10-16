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

**Export Command** (comic-server-bcl6): the reverse of import - reconstructs
a full `ComicDb.xml` (every book, every smart/reading list with matchers)
from the SQLite database, for real ComicRackCE to open. Three
comic-server-only smart list matcher types (`ComicServerCVSeriesCompleteMatcher`,
`ComicServerCVMissingCountMatcher`, `ComicServerCVPercentOwnedMatcher` - see
ComicVine Enrichment below) aren't recognized by ComicRackCE, so exporting a
smart list containing one drops just that matcher (keeping the list's other
matchers/structure intact) and prints a warning per drop.

```bash
# Export SQLite database back to ComicDb.xml
comic-server export --db /path/to/library.db --xml /path/to/ComicDb.xml

# Preview without writing the XML file
comic-server export --db /path/to/library.db --xml /path/to/ComicDb.xml --dry-run

# Verbose output
comic-server export --db /path/to/library.db --xml /path/to/ComicDb.xml --verbose
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

**Optional `ComicBook.xml`** (`cbz_convert.embed_comic_book`, off by default like ComicRack's "Embed Library Info"): also writes ComicRackCE 0.9.184's `ComicBook.xml` - the full library record (read progress, custom values, catalog fields, page info) - next to `ComicInfo.xml` (`BuildComicBookXML` in `comicbook.go`; format read from `../ComicRackCE` `ComicBook.Serialize`, PR #261). Same schema as a `<Book>` in ComicDb.xml with root `<ComicBook>`; Id/File/file times+size/last-opened list/dirty flags/missing/NewPages are blanked as CE does; `Pages` and page-indexed progress are re-based after Deleted pages are dropped. ComicRack's reader lets `ComicInfo.xml` win on shared fields. Output is not verified against a real ComicRack install (this repo has no .NET/Windows), only against the library schema ComicRack already reads from ComicDb.xml. Reading `ComicBook.xml` out of archives is not implemented.

**Write stale metadata back** (comic-server-os6x): `cbzconvert.UpdateMetadata` rewrites an existing CBZ's `ComicInfo.xml` (and `ComicBook.xml` with embed on) in place after library metadata changes. Page entries are raw-copied (`zip.Writer.Copy`), nothing is dropped or reordered, and the original goes to `internal/trash` via `Replace`. Staleness (`cbzconvert.IsStale`): `ComicInfoIsDirty`, or `ComicBookIsDirty` when embedding, or a mismatch with the fingerprint stored in the `comic_server_written_hash` custom value at the last write (`MarkWritten`). A book comic-server never wrote has no baseline, so it is not stale unless a dirty flag says so. Keys prefixed `comic_server_` are excluded from the fingerprint. Only CBZ/ZIP files qualify. Endpoints: `GET /api/library/workflow/stale-writes`, `POST /api/library/workflow/write-metadata`; "Out-of-date Files" card on the workflow page (only when `cbz_convert.enabled`). Reading progress is deliberately not fingerprinted.

**Unknown ComicInfo.xml elements are kept** (comic-server-y95y): every write (Convert, Write Metadata, Variant covers) builds with `BuildComicInfoXMLKeeping`, which copies elements outside the schema (a tagger's lowercase `<writer>`, `<Issue>`, `<HasBeenRead>`...) from the file's existing ComicInfo.xml; a case-twin of a schema field the library writes is dropped. Schema fields still come only from the library, so clearing one in the library clears it in the file (no merge). A 200-book prod sample (2026-10-14) found no book whose file had a schema field the library lacked; 20% had non-schema elements. The book page's ComicInfo compare uses the same build. The `Pages` block (comic-server-mgac) is written like ComicRackCE's export: one `<Page>` per page the library knows (Type, Bookmark, ImageSize/Width/Height, Key), Deleted pages dropped and renumbered on Convert but kept in place on Write Metadata / Variant covers; attributes the library does not store (DoublePage, Rotation...) are copied from the file's entry for the same page; with no library page info the file's own block is kept; with neither, the old page-0 FrontCover entry is written.

**Explicitly out of scope** (matches ComicRack's own defaults, not built): page re-encoding/resizing/rotation, combining multiple books into one archive.

**Trigger**: `POST /api/library/lists/:listId/convert-cbz` - runs over every book currently matched by one smart list, mirroring the user's real ComicRack workflow (multi-select a smart list's results, invoke Convert to CBZ) and comic-server-pkk.1's own scan-info endpoint pattern. A "Convert this list" button on the list detail page calls it, behind a `confirm()` prompt (destructive-adjacent, unlike scan-info's metadata-only write).

**Configuration** (`ServerConfig.CBZConvert`):
- `cbz_convert.enabled` - off by default, same precedent as `scan_info.enabled`. `Config.Validate` rejects `enabled: true` without `server.trash_path` also set - this feature cannot run without a quarantine directory.

### Duplicate Review (internal/duplicates/)

comic-server-as8l. `/duplicates` page + `GET /api/library/duplicates` (`?covers=1` adds the slower within-series cover-dHash signal) and `POST /api/library/duplicates/resolve` (`keep` | `keep_trash` | `keep_all`). Detection (`duplicates.Detect`, pure) groups by ComicVine issue ID, series+number+volume+format (reuses `library.CompressedSeriesName`), cover dHash (anchor-based, Hamming <= 5), and copy-style filenames; groups spanning > 3x page count are dropped, and a fileless book (physical-copy entry or Wanted placeholder) never groups with a file-backed one (`kind` prefix on the CV and series-number bucket keys). A "Re-scrape the kept book" checkbox makes the page POST `/api/scrape {book_ids:[keeper]}` after Keep / Keep and trash (full search, not fast rescrape, since the old tag may be the wrong one; 409/503 shown as a toast, the keep still stands). `keep_trash` quarantines losers via `internal/trash` (never `os.Remove`) and refuses to trash a file shared with the keeper. Decisions live in config.db (`duplicate_keep_all` keyed by hash of member IDs, `duplicate_resolved_books`; tables created lazily, no schema bump). `keep_trash` also removes each loser's library record via `library.Backend.DeleteBook` (XML: dropped from the library; SQLite: soft-deleted with `deleted_at`, like an import retiring a vanished book; logged to the audit log). `duplicate_resolved_books` remains as the safety net against a re-import resurrecting the record. The book is not removed from reading lists' explicit member lists; unresolvable members are ignored on read.

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

### Browser Testing (Playwright)

For verifying web UI changes (`internal/api/web/`) with a real headless
browser instead of just reading the JS/CSS. Setup is npm-based
(`package.json`, `.gitignore` allows it and ignores `node_modules/`):

```bash
npm install                    # installs the playwright package
npx playwright install chromium  # downloads the actual browser binary (~150-300MB, one-time)
```

`npx playwright install --with-deps` (which also apt-installs system
libraries) needs root and will fail in this sandbox - skip `--with-deps`.
Chromium launches fine headless without it here.

A driver script must live inside this repo (or anywhere under it) for
Node's `require('playwright')` to resolve via `node_modules` - a script
under `/tmp` or the scratchpad directory won't find it. Write one-off
driver scripts as dotfiles in the repo root (e.g. `.dm-click-through.js`),
delete them when done, and confirm `git status` is clean afterward (they're
untracked, not gitignored, so `git status` will show them until removed).

**Point `--library`/`--config` at a scratch copy, never
`testdata/library/ComicDb.xml` directly** - any write path (a rule apply,
a sync, a metadata edit) will silently overwrite the real committed
fixture through a real headless browser session exactly like it would
through any other client. Copy `testdata/library/` to a scratch directory
first (see the Test Library section above and the smoke-testing memory on
`book.FilePath` being relative to server cwd, not `--library`'s directory)
and point everything at the copy.

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

## Needs Attention Flags

`internal/attention` (comic-server-z77p) derives named, severity-ranked flags from each book record alone: `is_pdf`, `is_zip`, `needs_conversion`, `cover_only`, `no_series`, `low_page_count`, `unknown_publisher`, `missing_format`, `missing_count`. Computed in the same pass as the workflow stage cache (`workflowCacheEntry.booksByFlag`), so it shares `InvalidateWorkflowCache`. `GET /api/library/attention` returns flags ranked severity > count; `GET /api/library/attention/:flag/books` pages the matching books. Shown as a "Needs attention" panel on the workflow page. Idea from ComicManager (ideas only, AGPL). A flag can be **accepted** (hidden) on one book (comic-server-x7rd): `attention.KeyAccepted` = `comic_server_accepted_flags` custom value (`+`-joined), skipped in the workflow-cache pass, set via `POST /api/library/books/:id/accept-flag {flag, accepted}` (audited as `accept_flag`); Accept buttons on flag drill-ins and the book page, which also lists accepted flags with an undo. `file_missing` (comic-server-4q1z) comes from a background pass (`internal/api/diskcheck.go`): stats every book's resolved path (off the request path, metadata only, 10ms pause per 500 books, every `disk_check_interval_hours` = 24 by default, `-1` disables; env `COMIC_SERVER_DISK_CHECK_INTERVAL_HOURS`; chosen slow because the library sits on shared disks that also stream media), merges the last finished result into the flag cache, and invalidates the cache when a pass ends. It is off unless `SetDiskCheckInterval` is called (cmd/server.go does from config; not re-read on SIGHUP); results are empty until the first pass finishes. A wrong path mapping flags every book, so check mapping first if the count is huge. Cross-checks (comic-server-8fee, `internal/attention/crosscheck.go`): `count_mismatch` (Count > 0 differs from a synced volume's CV issue count), `metadata_mismatch` (the tagged `comicvine_issue` is cached with a different number, or under a different volume than `comicvine_volume`), `cv_volume_conflict` (siblings with the same compressed series + Volume > 0 + Format tagged with different CV volumes; the minority is flagged, both sides on a tie), `format_mismatch_in_volume` (comic-server-0vu9: books of one `comicvine_volume` whose non-empty Format differs from the volume's majority; same `minorities` helper). CV data is a memoised snapshot (`Server.cvAttentionData`, 10 min TTL): synced counts plus `Cache.IssueRefs` for only the library's issue IDs, chunked 500 per query. Unknown (unsynced/uncached) is never flagged. `corrupted_archive` (comic-server-4inw) is recorded **lazily**, never by a scan: every archive open that fails goes through `comicvine.openFailure`, which calls `comicvine.SetArchiveFailureHook` (wired in cmd/server.go to `Server.RecordArchiveFailure`) when the failure looks like corruption (`IsCorruptOpenError`: the container did not parse; missing file, permission, I/O or mount errors and password-protected RARs are not corruption). Failures live in the config.db `archive_health` table (lazy, keyed by resolved path with the file's mtime-size), are patched into the cached flag lists in place, and clear themselves when the file's version changes; Accept works as for any flag. The list therefore only knows files that something read (cover requests, page views, convert, scrape cover checks) and grows as the library is browsed. **Optional archive scan** (comic-server-yn10, `internal/api/archivescan.go`): `archive_scan_interval_hours` / env `COMIC_SERVER_ARCHIVE_SCAN_INTERVAL_HOURS`, **0 = off (default)**. A background pass opens each CBZ/ZIP/CBR/RAR/CB7/7z index via `comicvine.ListPageNames` (no page data; failures reach the same hook), 20 opens then a 250 ms pause. A file that opens cleanly is stored with its mtime-size in config.db `archive_verified` (lazy table), so a restart resumes and later passes read only new or changed files; non-corruption errors (I/O, permission) are retried next pass. Not re-read on SIGHUP. Not done: scrape results (`scrape_no_match` is covered by the Scrape Review card; `scrape_error` not done).

## Relink and Remove (missing files)

**Letter case** (comic-server-bj3j): a library written on Windows ignores case, a Linux disk does not (prod 2026-10-15: 500 books whose folder was `Journey Into Mystery` on disk, `Journey into Mystery` in the XML). `pathmap.ExistingCase` (called last in `Config.ResolveLibraryFilePath`, so every file read, the disk check and sync use it) returns the on-disk spelling when the exact path is missing and each component has exactly one case-insensitive match; two spellings are never guessed. Directory listings are cached 5 minutes. An existing path costs one extra stat. The stored `FilePath` is not rewritten.

comic-server-heox. A book flagged `file_missing` (disk pass, see Needs Attention Flags) gets two fixes. **Relink** (`internal/relink`, `POST /api/library/books/:id/relink {apply}` and bulk `POST /api/library/workflow/relink-missing {apply, book_ids?}`) looks only in the book's own old folder - one directory listing per folder, never a library walk (shared disks). Strategies in order, each needing exactly one candidate: same file name ignoring case and extension (an archive converted or renamed in place), then a name that parses to the same compressed series + issue number (`comicvine.ParseFilename`). Two fits is `ambiguous` and is reported, never guessed; a file another book already uses never matches; the old folder gone is `folder_missing`. `apply` false is a preview that changes nothing; apply sets only `FilePath` (kept in the library's own path style, so a Windows raw path stays Windows-style) and `FileSize`, audited as `relink`, and drops the book from the in-memory disk-check result so the flag clears at once. **Remove record** (`POST .../books/:id/remove-record`) deletes only the library record, never a file, and refuses (409) when the file is back or the book has no file; audited as `remove_record`. UI: "Relink missing files…" (preview, confirm, apply) on the file_missing list; "Relink file…" / "Remove record…" on a missing book's page. **Manual pick** (comic-server-679r): `POST .../books/:id/relink` also takes `file_path` (a path on the server's disk, from the `browseServerFile` picker); the book page offers it when the old-folder search finds nothing. The pick must be a regular comic archive, not used by another book, and (when `library_root` + `library_source_root` are set) under `library_root`; it is stored under `library_source_root` in the old path's separator style. Statuses `pick_invalid` / `pick_taken`; same preview/apply and audit as the folder search. The picker opens in the book's old folder, or its nearest parent that still exists (`RelinkOutcome.start_dir`, `nearestExistingDir`); `/` when none is left. Not done: the expected-folder search ComicManager has (needs the organizer profile).

## Not in ComicVine

comic-server-5wer (phase 1). Collected volumes, web comics and other books ComicVine does not list get an explicit decision instead of sitting at the Scrape stage forever. `comic_server_cv_nomatch` = `<reason>|<YYYY-MM-DD>` (reasons `collected`, `not_listed`, `web_comic`, `other`; `workflow.SetNoCVMatch` / `ClearNoCVMatch` / `NoCVMatch`). `workflow.InferStage` no longer returns `StageScrape` for an untagged book that is `ExemptFromScrape` (the marker **or** the older `CVDBSKIP` tag; prod 2026-10-16: 82 of 83 CVDBSKIP books were stuck at Scrape). Setting the marker advances a stored Scrape stage; clearing it puts a book with no CV tag and no CVDBSKIP back at Scrape. Whole-library scrapes skip marked books (reason "marked not in ComicVine", `retry_held_back` does not re-include them); a scrape by `book_ids` still works, and any successful `WriteMetadata` removes the marker. `POST /api/library/books/:id/cv-nomatch {set, reason}` (audited as `cv_nomatch`; 409 for a fileless book). UI: "None of these - not in ComicVine" on each Scrape Review item (a marked book drops out of the review list), "Mark as not in ComicVine" / Undo and a badge on the book page, a count card on the workflow page (`cv_nomatch` in `GET /api/library/workflow`). **CVDBSKIP migration**: books that already had a *stored* Scrape stage stayed there (InferStage only covers unstaged books), so `Server.MigrateCVDBSkip` (`internal/api/cvnomatch_migrate.go`) copies the tag into the marker (reason `legacy_cvdbskip`, not offered in the UI) and advances the stage. The tag is left on the book (ComicRack still reads it). Only files without a CV tag or marker are touched; idempotent; runs in the background at every start and after every library import (ComicRack's XML still carries the tag). Audited as `cv_nomatch`. **Phase 2 - match by pasted link/ID**: `comicvine.ParseCVRef` reads an issue/volume URL (comicvine.gamespot.com / comicvine.com only), a typed id (`4000-N` issue, `4050-N` volume; other types such as characters are refused with a clear message) or a bare number (an issue id). `POST /api/library/books/:id/cv-match {ref, issue_id, apply}`: `apply` false previews and writes nothing (an issue: cover, number, date, volume, publisher, issue count next to the book's own pages/year; a volume: its issue list to pick from, then `issue_id`); `apply` true runs `Scraper.ApplyIssue` (same `WriteMetadata` as a scrape, no number or file name check since the user chose, audited as `cv_match`, marks any pending review of the book resolved via `Cache.ResolvePendingReview`, removes the not-in-ComicVine marker). Uses the paced, circuit-broken client; an open circuit is a 503 with the retry time, other ComicVine failures 502; a preview costs one issue call (cached forever) and no volume call for a synced volume. 409 for a fileless book on apply. UI: `js/cvMatch.js` (`CVMatchBox`) in a "Match by ComicVine link or ID" section on the book page and on each Scrape Review item. **Phase 3 - unnumbered books**: the issue number to look for is the file name's, else the library `Number` (`issueNumberFor`); a scrape never guesses #1. A book with neither, whose volume matched with high confidence, becomes `pending_review` (candidates = top 3 volumes) instead of the old dead-end "no matching issue number" failure; with `auto_only` it is skipped. So it is held back from whole-library scrapes and shows in Scrape Review, where `needs_issue` (computed at list time, `comicvine.NeedsIssueNumber`) swaps Accept for "Choose issue…", which opens the match box on that volume's issue list (Phase 2 preview, then confirm). `AcceptReview` also uses the library Number. Not built: auto-picking the only issue of a one-issue volume (the user still confirms). **Phase 4 - review page polish**: every candidate row has an "Issues…" button (next to Accept) that opens the match box on that volume's issue list (the Phase 2 volume preview: cached, one paced call when the volume is unsynced), and each item has a "Re-scrape" button (`POST /api/scrape {book_ids:[id]}`). `Cache.UpsertScrapeJobBook` deletes a book's pending_review rows from OTHER jobs when it stores a new pending_review row, so a re-scrape replaces stale candidates instead of listing the book twice; a scraped (matched) result removes them too, so a Re-scrape that now matches drops the book from review; other outcomes (skipped, failed) leave a pending review alone. `AcceptReview` records the outcome in the job that holds the book's pending row (`Cache.PendingReviewJobID`), not only the job passed in, since the web UI passes the latest job and any later scrape replaces it. Not browser-smoke-tested. **Live scrape log**: Re-scrape on the book page and on Scrape Review opens a log dialog (`js/scrapeLog.js`, `ScrapeLog.open(jobId)`). The scraper writes plain-sentence steps through a ctx-carried sink (`comicvine/joblog.go`, `jobLog(ctx, ...)`, so the client and pacer can report ComicVine calls, rate-limit waits and an open circuit without knowing about jobs); `Scraper.SetJobLog` gets them (also logged at debug with `job_id`). The API keeps the last 2000 lines of the last 10 jobs in memory (`internal/api/scrapelog.go`), pushes each as a `scrape_log` WebSocket event, and serves the backlog at `GET /api/scrape/log?job_id=` (`running` false once the job ended). `websocket.js` sets `window.wsClient`: pages that hooked `window.wsClient` (Scrape Review live refresh, Devices) silently never subscribed before. **Phase 5 - close-out**: `GET /api/library/workflow/cv-nomatch` pages the marked books (newest mark first, with reason and date; from the workflow cache's `cvNoMatch` list), shown from a View button on the workflow page's "Not in ComicVine" card with an Undo per row. `POST` on the same path `{book_ids, set, reason}` marks or clears many books in one save (audited per book as `cv_nomatch`; fileless or unknown ids come back in `skipped`; at most 5000 ids); Scrape Review has checkboxes, Select all and "Mark selected as not in ComicVine". For an unnumbered book, a candidate volume ComicVine lists with exactly one issue, whose issue is already cached, gets `only_issues[volume_id]` in the review list (cache only, no API call) and a "Use #N…" button that opens the match preview on that issue; the user still confirms. The Phase 5 UI was smoke-tested in a headless browser (review queue mocked).

## Format Detection

comic-server-izc7. `internal/formatdetect` proposes a `Format` from the book record: ordered rules, first match wins, order taken from ComicManager's formatting guide (ideas only): Director's Cut and Black & White (series/title), Omnibus and TPB (series/title/summary), 80+ pages with "Collects"/"#1-5" in the summary is TPB, 500+ pages is Omnibus, Annual, Preview(s), then by `Count`: none = no match (so `missing_format` stays), over 15 = Series, **issue number over 15 = Series even with a small count (continuation / renumbered series)**, 2-15 Limited Series, 1 One-Shot. Chosen over Data Manager rules because it needs state DM rules cannot hold (our own last write, the count it was based on) and ships defaults (DM rulesets are user data). Hardcover/Absolute/Graphic Novel are manual-only.

- **Never overwrites a manual choice**: only an empty `Format`, or one still equal to `comic_server_format_auto` (what detection last wrote), is touched. `comic_server_format_locked=1` skips a book outright. A count change re-detects books whose Format is still ours.
- **API**: `GET /api/library/workflow/format-detect` previews (total, per-format counts, first 100 rows) and writes nothing; `POST` applies, audited as `format_detect`. Wanted placeholders (no FilePath) are skipped; paper copies are not.
- Known risk: a regular issue whose summary mentions "omnibus"/"TPB" (e.g. "collected in the TPB") is classified by that keyword, as in ComicManager. Check the preview before applying.
- UI: "Detect Format" card on the workflow page (comic-server-6y63). Not done: feeding Format into organizer folder structure.

## Count Fill

comic-server-08jr. `internal/countfill` fills `Count` from the ComicVine cache (`cv_volumes.issue_count` via `Cache.SyncedIssueCounts`), so format detection has a count to work with. Only a `Count` of 0, or one still equal to `comic_server_count_auto` (what this last wrote), is touched; that lets a re-run follow a growing ongoing series without overwriting hand-set counts. Untagged books and volumes not yet synced are skipped (unknown is not zero; the preview reports them as `waiting`). Wanted placeholders are included (Count is metadata, no file I/O).

- **API**: `GET /api/library/workflow/count-fill` previews (total, volumes, waiting, first 100 rows); `POST` applies, audited as `count_fill`. 503 without a ComicVine cache.
- **Files**: filled counts are normal metadata and go into ComicInfo.xml on the next comic-server write. Only books comic-server already wrote (they have `comic_server_written_hash`) show as stale afterwards; nothing sets ComicRack's dirty flags. Decided 2026-10-13 over keeping CV counts out of files.
- Run count fill before format detection. UI: "Fill Issue Counts" and "Detect Format" cards on the workflow page (comic-server-6y63); preview loads only on click (each GET scans the library), Apply behind a confirm.

## Book Edit Form

`POST /api/library/books/:id/edit {fields, read}` (`internal/api/bookedit.go`), the "Edit" button and form on the book page, and "Mark as read/unread" there. Only fields sent are written (the form sends just what changed), through `datamanager.SetFieldString`; the allowed set is `editableFields` (FilePath and other file fields are not editable). Ratings 0-5, Month/Day/Year ranges and Yes/No/Manga values are checked; "Unknown" is stored empty. `read` follows ComicRackCE `MarkAsRead`/`MarkAsNotRead`. Changing a ComicInfo field on a book with a file sets `ComicInfoIsDirty` (so it shows under Out-of-date Files); any change sets `ComicBookIsDirty`; a field the organizer could use sends a filed book back to `StageToMove` (not Rating, Summary, Notes, Review or collection fields). Audited as `edit`. Not done: custom values, multi-book edit.

## Workflow Hero

comic-server-p7ti. Top of the workflow page: percent clean (Organized / every staged book; hidden when no book is staged), a 14-day sparkline of stage advances with this-week vs last-week delta, and a cover strip of the most recently advanced books. No new table: `GET /api/library/workflow/throughput` folds `audit_log` rows whose stage moved forward (`configdb.ListStageMoves`, `buildThroughput` in `internal/api/throughput.go`). A book moving two stages counts twice; rollbacks count nothing. History is only as old as the audit log (180 days, and it only exists from vqsj onward), so a fresh install shows zeros. Reverse sync and CLI scrape/backfill are not audited, so they do not appear.

## Archive Content Sniffing

comic-server-0ebv. `comicvine.SniffArchive` reads a file's magic bytes (ZIP, RAR, 7z); `ExtractCover`, `ReadAllPages` and `ReadComicInfoXMLBytes` dispatch on the real container, falling back to the extension when content is unrecognised, so a RAR named `.cbz` reads instead of failing as "corrupt". Sniffing costs nothing extra: it happens only when the file is being read anyway. There is deliberately **no** bulk misnamed-archive scan - an earlier version opened every file in the disk pass and was removed (66K data reads on shared raidz disks); a `misnamed_archive` flag, if wanted, should be recorded lazily from reads that already happen. "Convert this list" repacks a misnamed archive by content. `workflow.GetStage` stays extension-only on purpose (no I/O on the 66K-book hot path), so misnamed books do not enter the Convert stage by themselves.

**Decided not to build**: a hidden backup before in-place patches (`trash.Replace` already quarantines the original and restores it on a failed rename, see Safe File Replace) and a "Rezip" recovery action (needs real corrupt samples to be worth designing). Plain `.zip` rename-to-`.cbz` is not special-cased; Convert covers it. **`.cbw` is not an archive**: it is a ComicRack web-comic pointer (an XML list of remote page URLs), so it is never converted: `cbzconvert.IsWebComicPointer`, stage inference skips it, and a convert run advances any already-staged `.cbw` book past the Convert stage (reported as `skipped`). Found because a user described `.cbw` as renamed archives; the file header (`<?xml`) said otherwise - check the bytes before assuming. Unrecognised content is left to the `corrupted_archive` bead.

## Missing Issues

comic-server-buhs. `/missing-issues` page: per ComicVine volume, which issues the library owns and which it does not. `GET /api/library/series-completeness` (ranked volume rollup, `q`, `only_missing`, paged; memoized 2 min on the Server) and `GET /api/library/series-completeness/:volumeId` (every issue, numerically ordered, with owned / wanted / missing state and the matching book id), both from `comicvine.BuildVolumeSummaries` / `BuildSeriesCompleteness` over the same cache `BuildCompletenessMap` uses. `POST .../:volumeId/wanted {issue_ids}` creates Wanted placeholders (no `FilePath`) carrying `comicvine_volume`/`comicvine_issue`, skipping owned or already-wanted issues; audited as `wanted_from_missing`.

**A Wanted placeholder (no `FilePath`, not a paper copy) is never "owned"** in completeness, including in `BuildCompletenessMap` and so the CV matchers; a paper copy is owned (see Paper Copies). Only volumes whose issue list is synced appear (unknown completeness is not zero). Owned books of a volume with no issue id are reported as `unmatched_owned`, not counted as owned. The book page shows a completeness bar for the book's own volume (comic-server-n2yb; lazy fetch of `GET /api/library/series-completeness/:volumeId`, silent when ComicVine is off or the volume is unsynced) and `/missing-issues?volume=<id>` opens that volume. Not done: variants/annuals edge cases from ComicManager's page, a completion bar on list pages.

## Paper Copies

comic-server-vffq. A paper copy is a book with no `FilePath` and ComicRack's own `BookCollectionStatus` = "Physical" (any case; `library.IsPhysical`). Every other fileless book is a Wanted placeholder (`library.IsWantedPlaceholder`) - the user's existing fileless books (1,529 on prod, 2026-10-14) are Wanted, so the marker must be explicit. ComicRack's field was chosen over a `comic_server_` value so ComicRack shows it, smart lists match it and export keeps it. Paper copies count as **owned** in completeness / Missing Issues (decided 2026-10-14), are left out of the Wanted list, cannot be linked to a file via the Wanted link flow (409), and get format detection (metadata only); they never enter the workflow (no file). `BookLocation`/`BookCondition`/`BookNotes` hold storage, grade and notes.

- **API**: `POST /api/library/physical` adds one (409 if a paper copy with the same compressed series + Volume + number exists); `POST /api/library/physical/import {csv, apply}` previews or applies a CSV: header row matched case-insensitively ignoring spaces/_/-/./# with aliases (Issue, Vol, Storage/Box, Grade, CV Issue...), Series required, 10,000-row cap, rows already present (library or earlier in the file) are `exists` and skipped so re-imports add nothing. Audited as `physical_add`.
- **UI**: `/physical` (Library > Paper Copies): add form that keeps series/volume/publisher/location for the next issue, CSV paste or file with preview then Import. Browse rows and the book page show a Physical / Wanted badge (`ComicPreview.kind`, `BookDetail.kind`). The router now URL-decodes route params, so ids like `{guid}` (every comic-server-created book) open on the book page.
- Duplicates still put all fileless books in one bucket kind, so a paper copy and a Wanted entry for the same issue are shown as a group.

## Variant Covers

comic-server-0u48. `/variant-covers` page + `GET|POST /api/library/variant-covers/:bookId`. ComicVine's issue endpoint has an undocumented `associated_images` list (`id`, `original_url`, `caption` usually null, `image_tags` observed only as the catch-all "All Images"); nothing marks an entry as a variant cover, so the flow is **preview then apply**: the page shows every candidate with a thumbnail and the user ticks the real covers. `comicvine.Client.FetchIssueImages` (paced, circuit-broken) lists them; `DownloadImage` fetches from the CDN, restricted to https `*.gamespot.com`, 25 MB cap, content sniffed (not trusted from URL). The POST re-fetches the issue server-side and selects by id, so client-supplied URLs are never downloaded.

`cbzconvert.AddVariantCovers` raw-copies the existing entries, appends `zzzz-cv-variant-<imageID>.<ext>` entries (appended, not inserted after the cover, so existing page indexes/bookmarks/progress are untouched) and rewrites ComicInfo.xml (and ComicBook.xml with embed on) with FrontCover pages at the new indexes, via `trash.Replace`. Idempotency: same entry name is skipped, and added ids are kept in the `comic_server_variant_cover_ids` custom value (`+`-joined). CBZ/ZIP only; requires `cbz_convert.enabled` + `trash_path` + a configured ComicVine key; audited as `variant_covers`. Not done: automatically guessing which images are variants (filename-suffix investigation bead), the CV main cover as a candidate (it has no id), CBR/CB7 sources, batch/scrape-time insertion.

## Page Manager

comic-server-j1cv. `/page-manager?book=<id>` (book page "Manage pages…") reorders, deletes and adds pages of one CBZ in a single atomic write. `GET /api/library/books/:id/pages` lists pages (name, size, library type/bookmark, plus a `version` = mtime-size of the file), `GET .../pages/:n?w=` serves a JPEG thumbnail, `POST .../pages` (multipart: `plan` JSON `{version, order:[{page:N}|{upload:K}]}` + repeated `upload` files) applies. The page stages everything client-side; a stale `version` is refused with 409 so a plan never applies to a different file than the one shown.

`cbzconvert.ManagePages` raw-copies kept pages under new zero-padded names (`000.jpg`...; readers order by entry name, so renaming is what makes an order stick), drops pages left out of the plan **from the file** (original to `internal/trash` via `Replace`, decided 2026-10-15 over only marking `Deleted`), and rewrites ComicInfo.xml (and ComicBook.xml with embed on) in the same write. Page-indexed library data is **remapped, not reset**: `Pages`, `CurrentPage`, `LastPageRead`, `PreferredFrontCover` follow their page; a pointer at a deleted page moves to the nearest kept page before it; file-only page attributes (DoublePage...) are re-indexed in the existing ComicInfo.xml so they follow the page. Uploads are content-sniffed (JPEG/PNG/GIF/WebP, 25 MB each), never trusted by file name. CBZ/ZIP only; same gates as Variant Covers (`cbz_convert.enabled` + a trash path); audited as `page_manager`.

`variantApplyBlocker` (shared by Variant Covers, Write metadata and Manage pages on the book page) reads the **effective** trash path (`effectiveTrashConfig`, config.db first): config.yaml's `trash_path` is cleared in memory after the config.db migration, so reading `cfg.Server.TrashPath` made every migrated install report "trash_path is not configured". Not done: PDF/CBR/CB7 sources, page-type editing in the UI, undo (the trashed original is the undo, via the Trash page).

## Reader

comic-server-65o8. `/reader?book=<id>&page=<n>` (book page "Read") is a basic in-browser page viewer: prev/next/first/last, arrow keys / space / Home / End, click left third = back and the rest = forward, `f` toggles fit-screen / fit-width, next and previous page preloaded, `page` kept in the URL with `replaceState`. Pages come from the Page Manager routes: `GET /api/library/books/:id/pages` (names; sizes for CBZ only) and `GET .../pages/:n?full=1` (original bytes, content-sniffed, `image/*` only, `nosniff`). `comicvine.ListPageNames` / `comicvine.ReadPage` read one entry of CBZ/CBR/CB7 by the real container (sniffed), in the same page order as `ReadAllPages`, so a page index means the same page everywhere. Reading needs none of the write gates (`cbz_convert`, trash); `BookDetailActions.reader_blocker` covers no file, `.cbw` pointers and unknown formats.

**No progress write-back and no manga/RTL mode** (reading progress belongs to device sync and Komga; decided as v1 scope). A CBR page request lists the archive then reads one entry, and a solid RAR cannot be read at random (same limit as covers), so very large RARs are slow per page.

## Browse Quick Search and Sort

`POST /api/library/browse` (and the Browse-scoped Data Manager endpoints) accept an optional `query`, `sort` and `sort_desc` (comic-server-mzkf). `query` narrows the matched set and never widens it: with no matchers and no `show_all` the result is still empty. Plain words must each appear in series/title/publisher; `#N` matches issue number (leading zeros ignored), `(YYYY)` year, `VN` volume (`library.ParseSearchQuery`, `internal/library/searchtokens.go`). `sort` is one of series, number (numeric-aware), title, publisher, year, volume, pages; books with an unset value (no page count, no year...) sort last in both directions. The query is passed to the Data Manager preview/apply scope so a bulk action acts on exactly what the preview shows; it is not saved with "Save as Smart List".

**Filter rail and views** (comic-server-j6lk): `facets {publishers, formats, stages, flags}` on the browse and Browse-DM requests narrow after the query (OR within a facet, AND across; `""` = no value; publisher/format case-insensitive; flags from the workflow cache, so accepted flags are excluded). The first page (offset 0) returns `facets` counts over the matchers+query set before any facet, so ticking one option keeps the others visible (`internal/api/browse_facets.go`). The page keeps matchers, show_all, query, sort and facets in the URL (`history.replaceState`), so Back from a book page and bookmarks restore it; the Table/Grid/Compact view is a per-browser localStorage preference. Facets are not saved by "Save as Smart List".

## Browse Bulk Edit

comic-server-k1bt.2. ComicRack's multi-book Info dialog for Browse. Rows have checkboxes (table, grid, compact); ticking every loaded row offers "Select all N matching", which makes the Browse filter (matchers/show_all, query, facets) the scope instead of ids. `js/browseBulk.js` (`BrowseBulk`) draws the bar and editor; a fresh search clears the selection. `POST /api/library/browse/bulk-edit {book_ids | browse, expected_count, edit, apply}` runs the single-book edit (`BookEditRequest`, same field checks, dirty flags, stage regression and `edit` audit rows) over the scope. `apply` false previews (counts per field, first 20 books) and writes nothing; apply needs `expected_count` equal to the scope's size now (409 otherwise, so the library cannot shift under a preview). Every book is computed before any write, so a rejected value writes nothing. Cap 10,000 books (413, narrow the filter), written 500 at a time. List fields (Genre, credits, Characters, Teams, Locations, Tags) take `list_add` / `list_remove` (case-insensitive, existing spelling kept) besides replace; the single-book endpoint accepts them too. Number is not offered in the bulk form. Mark read/unread uses `edit.read`. No undo: the old values are in the History page. Browser-smoke-tested on a scratch library (note the XML backend flushes to disk about 30 s later).

## Scrape Hold-back

comic-server-fohc. A whole-library scrape (`POST /api/scrape` with no `book_ids`) leaves out books a retry would not help: tagged `CVDBSKIP` (always), and books whose latest outcome across all jobs (`Cache.LatestScrapeResults`) needs the user - `comicvine.HeldBackReason`: pending review, "no candidate volumes found", "no matching issue number found in selected volume". API errors/timeouts are retried. `options.retry_held_back` re-includes the outcome-based ones (never CVDBSKIP). Scraping by `book_ids` (book page Re-scrape, Duplicates) ignores all of this. The 202 response carries `total_books` and `held_back` (reason -> count); the workflow Scrape card shows them.

**Publisher filter** (comic-server-2w44, UI/DB in comic-server-pl94): case-insensitive substrings of the CV volume's publisher. `comicvine.PublisherFilter` drops candidates in `scrapeOneBook` before scoring (allow narrows, block carves out, empty publisher passes). Not applied to `AcceptReview` (manual pick) or `FastRescrape`. Every candidate excluded gives `errAllFiltered`, which `HeldBackReason` treats as held back. Source of truth: the config.db `scrape_publisher_filter` table (lazy, no schema bump) once saved from Settings > Scrape publisher filter (`GET|PUT /api/settings/scrape-publisher-filter`, response carries `source` = saved|config); until then `server.scrape_publisher_allow|block` / env `COMIC_SERVER_SCRAPE_PUBLISHER_ALLOW|BLOCK` apply (`Server.effectiveScrapePublisherFilter`). Saving two empty lists overrides config.yaml with no filter. The scraper reads the filter once at the start of each job (`SetPublisherFilterSource`), so a save needs no restart and a batch uses one consistent filter. Idea from ComicManager (ideas only). The older built-in -6 Panini/mirror-publisher penalty in `publisherScore` is unchanged. Saves are not audit-logged (the audit log is per book; trash settings are not either).

## Audit Log and Rollback

`internal/audit` (comic-server-vqsj) records what each pipeline action changed. One wrapper, `audit.Backend` (`s.audited(action)` in `internal/api/audit.go`), sits around `library.Backend`: every `UpdateBook(s)` through it diffs the incoming book against the stored one (scalar fields by value, custom values per key, slice fields as a count; the workflow stage is reported as `stage_before`/`stage_after`) and appends rows to the `audit_log` table in config.db (lazy `CREATE TABLE IF NOT EXISTS`, no schema bump). Entries are written only after the update succeeds; a log failure never fails the action.

- **Wrapped paths**: convert_cbz, write_metadata, scan_info, data_manager, organize, scrape (via `SetScraper`), link_wanted, rollback; duplicate trashing adds an entry with `audit.Record`. **Not** wrapped: device reverse sync (reading progress changes constantly and is not a pipeline action) and the CLI `scrape`/`workflow backfill` commands.
- **`audit.Backend.GetBook` returns a copy.** The XML backend's `GetBook` returns a pointer into the live library, so a caller editing it in place would hide its own diff. Anything that read-modify-writes a book should read through the wrapper.
- **Retention**: `audit.Retention` = 180 days, pruned at most hourly on reads of the list endpoint.
- **API**: `GET /api/library/audit?book_id=&action=&limit=&offset=`; `POST /api/library/audit/rollback {book_id, stage}`. History page at `/history`.
- **Rollback is stage-only and backwards-only.** It resets the stage via `workflow.RegressToStageIfPast` so the pipeline re-runs the book; it does not restore field values (the old values are in the entry's diff) and does not move files back. It is itself logged.

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

### Virtual Tag Matchers (`ComicBookVirtualTag1Matcher`..`20Matcher`)

ComicRack-compatible (same XML types, comic-server-65u). Each is a plain string matcher over a template-computed value: `server.virtual_tags` in config.yaml defines slot id + `caption_format`, and `internal/library/virtualtags.go` expands it per book (`VirtualTagValue`) using a port of ComicRackCE's `ExtendedStringFormater` (`captionformat.go`: `{Field}`, `{Field:fmt}`, `[optional]` groups, `\X` escapes; `captionfuncs.go`: the 13 `$func<>` functions, a small C#-style expression evaluator for `$if`/`$expr`, .NET date/number formatting). Matchers are one generic path - the slot number comes from the matcher `Type` ("VirtualTag7"), no per-slot types. Definitions are config-only (ComicRack keeps them in desktop settings, not the library XML), registered via `library.SetVirtualTags` at startup and SIGHUP. Never SQL-translated (computed value), so any list containing one evaluates fully in memory. Deliberately faithful to ComicRack's quirks (unescaped `>`/`[`/`$` inside a function call break it, `$month`/`$day` are D4-padded, `success |= nested` bracket bookkeeping) - check `../ComicRackCE/cYo.Common/Text/ExtendedStringFormater.cs` before "fixing" any. Not ported: `{{CustomValue}}` lookups, per-book result cache (recomputed on demand instead), proposed/filename values. See docs/CONFIGURATION.md "Virtual Tags" for user-facing docs.

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
- `/books/:bookId` - Book detail page (comic-server-br14): record, file, stage, attention flags, ComicVine links, this book's audit history, and an on-demand compare of the file's ComicInfo.xml with what Write Metadata would write (`GET /api/library/books/:id` and `/:id/comicinfo`; `Pages` skipped, empty == absent). Actions (comic-server-2s4f): Re-scrape (`POST /api/scrape {book_ids}`), Write metadata to file (`POST .../workflow/write-metadata {book_ids}` writes those CBZs even when not stale; no body = every stale book), Variant covers (`/variant-covers?book=`), stage rollback (stages before the current one). `BookDetail.actions` carries a `*_blocker` reason per action so a disabled button says why. Characters, teams and the main character on the book page are chips linking to Browse with a `Characters`/`Teams` (list membership) or `MainCharacterOrTeam` (equals) matcher in the `m` URL param (comic-server-n2yb). In-app links use `<a href=... data-route>` (one delegated click handler in app.js); Browse, list cards, workflow drill-ins, Duplicates and History link here.

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
- **Navigation** - five top entries (comic-server-gkzt): Dashboard, Library ▾, Pipeline ▾, Devices ▾, Settings. Groups are defined in `Navigation.ENTRIES` (`js/navigation.js`); a group button is active when one of its pages is, and its badge sums the items marked `alert` (Scrape Review, Trash). Icons only on mobile. The bar does not scroll (overflow would clip the menus) - add a page to a group, not a new top entry.
  - Data Manager rules editor (`/datamanager/rules`) is full width; each tree row is one line, with Rename/Move/Delete (and New ruleset/subfolder on folders) in a "⋯" row menu
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
## Task Tracker (GitHub Issues)

Open work is tracked as GitHub Issues in the **private** repo `duckpuppy/comic-server-tracker` (moved from beads on 2026-10-16: cloud threads cannot push beads' Dolt data ref). This repo is public, so keep prod host names, paths and other private detail in the tracker or the handoff notes, not in commits, PR bodies or code. Use `gh` (or the GitHub MCP tools in cloud threads); do NOT use TodoWrite, TaskCreate or markdown TODO lists.

- **Find work**: `gh issue list --repo duckpuppy/comic-server-tracker --label P1` (or `P2`...); `gh issue view N --repo ...`.
- **Priority** is the label `P0` (critical) to `P4` (backlog). **Type** is the label `bug`, `feature`, `task` or `epic`.
- **Create**: `gh issue create --repo duckpuppy/comic-server-tracker --title ... --body ... --label P2,feature`. Put design, acceptance criteria and notes in the body; there is no length pressure.
- **Epics**: make children native sub-issues (cloud MCP: `issue_write` create with `parent_issue_number`, or `sub_issue_write`; CLI: `gh api -X POST repos/duckpuppy/comic-server-tracker/issues/PARENT/sub_issues -F sub_issue_id=<child database id>`).
- **Dependencies**: `gh api -X POST repos/duckpuppy/comic-server-tracker/issues/N/dependencies/blocked_by -F issue_id=<blocker database id>` (get the id with `gh api repos/.../issues/M --jq .id`). No MCP tool exists for this.
- **Close from a PR**: put `Closes duckpuppy/comic-server-tracker#N` in the PR body; merging closes the issue even though the repo differs. The line reveals the private repo name and number in a public PR, nothing else.
- **Old IDs**: the 14 beads still open on 2026-10-16 became issues #1-#14 and keep their `comic-server-xxxx` id in the title. The ~278 closed beads stay in the local beads database (`bd show <id>`) as a read-only archive. Older docs, commits and memory notes cite those ids.
- **Not in the tracker**: durable facts about the code belong in this file; `bd remember` is no longer used.

## Standing Notes

Carried over from `bd remember` entries when beads was retired:

- **Smoke-testing anything that opens `book.FilePath`** (covers, sync, cbzconvert...) against a scratch copy of `testdata/library`: `FilePath` entries in `ComicDb.xml` are RELATIVE (for example `testdata/library/comics/...`) and resolve against the SERVER PROCESS's cwd, not the library file's directory. Pointing `--library`/config at a scratch copy is not enough: also run the server with its cwd inside a scratch tree that mirrors the relative path, or it silently reads and writes the REAL fixture files. Those files are gitignored, so a clean `git status` does NOT prove nothing was touched; verify by content or hash. This once overwrote four committed comic files' ComicInfo.xml during the CBZ conversion work.
- **Untagged books** (about 1,650 of 66K) are mostly hand-collected web comics with no ComicVine id. Do not fuzzy-match them for enrichment.
- **ComicRack matcher compatibility**: existing `ComicBookXxxMatcher` types must stay compatible with ComicRackCE smart lists. New comic-server-only matcher types are fine; ComicRack just will not recognise them.
- **ComicVine scraper reference**: https://github.com/cbanack/comic-vine-scraper (the ComicRack plugin this scraper replaces).

## Session Completion

**When ending a work session**, complete ALL steps below. Work is NOT complete until `git push` succeeds.

1. **File issues for remaining work** in the tracker above.
2. **Run quality gates** (if code changed): `just ci` (tests, lint, build).
3. **Update issue status**: close finished issues (or let the PR's `Closes` line do it), comment on in-progress ones.
4. **Push**:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up**: clear stashes, prune merged branches.
6. **Hand off**: give the next session the context it needs.

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds.
- NEVER stop before pushing; that leaves work stranded locally.
- NEVER say "ready to push when you are"; push it yourself.
- If push fails, resolve and retry until it succeeds.
