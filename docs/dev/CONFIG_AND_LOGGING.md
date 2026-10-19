# Configuration and Logging

How config files, environment variables and per-device sync settings fit together, and how logging is set up.

Moved from `CLAUDE.md`. See the index there for the other developer reference files.

## Configuration Management (internal/config/)

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
- `COMIC_SERVER_TRASH_PATH` - Quarantine directory for internal/trash (see Safe File Replace in [ARCHIVE_FILES.md](ARCHIVE_FILES.md))
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
