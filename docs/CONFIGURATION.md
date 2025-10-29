# Configuration Reference

This guide covers all configuration options for comic-server.

## Configuration Methods

Configuration can be provided through (in order of precedence):

1. **Command-line flags** (highest priority)
2. **Environment variables**
3. **Configuration file** (lowest priority)

## Configuration File

### Location

The server looks for configuration files in the following locations:

1. `./config.yaml` (current directory)
2. `~/.config/comic-server/config.yaml` (user config)
3. `/etc/comic-server/config.yaml` (system-wide)

You can also specify a custom location:

```bash
comic-server server --config /path/to/config.yaml
```

### Supported Formats

Configuration files can be in YAML or TOML format:

- `config.yaml` or `config.yml`
- `config.toml`

### Example Configuration File

**YAML format (config.yaml):**

```yaml
# Server configuration
server:
  # Path to ComicRack library file (required)
  library_path: "/home/user/.local/share/ComicRack/ComicDb.xml"

  # Server ports
  discovery_port: 7615      # UDP multicast discovery
  command_port: 7614        # TCP device communication
  api_port: 7620           # REST API and metrics

  # Automatic sync on device discovery
  auto_sync: false

  # Logging configuration
  log_level: "info"        # debug, info, warn, error
  log_format: "text"       # text, json

  # Connection limits
  max_concurrent_connections: 10
  max_connections_per_ip: 3
  max_requests_per_device: 100

  # Devices to ignore (by IP, ID, or name)
  ignore_devices:
    - "192.168.0.24"
    - "SM-T970"
    - "Production Tablet"

# Per-device sync configuration
devices:
  # Device identified by device_id
  tablet-abc123:
    device_id: "tablet-abc123"
    device_name: "Samsung Galaxy Tab"

    # Smart lists to sync
    lists:
      - list_id: "6300352f-35f2-4f98-8953-6ef29162122a"
        list_name: "Recent Comics"
        enabled: true
        settings:
          only_unread: true
          limit: true
          limit_value: 50

      - list_id: "7411463a-46e3-5g09-9a64-7fg30273233b"
        list_name: "Marvel Collection"
        enabled: true
        settings:
          only_unread: false
          limit: false
```

**TOML format (config.toml):**

```toml
[server]
library_path = "/home/user/.local/share/ComicRack/ComicDb.xml"
discovery_port = 7615
command_port = 7614
api_port = 7620
auto_sync = false
log_level = "info"
log_format = "text"
max_concurrent_connections = 10
max_connections_per_ip = 3
max_requests_per_device = 100
ignore_devices = ["192.168.0.24", "SM-T970", "Production Tablet"]

[[devices]]
device_id = "tablet-abc123"
device_name = "Samsung Galaxy Tab"

[[devices.lists]]
list_id = "6300352f-35f2-4f98-8953-6ef29162122a"
list_name = "Recent Comics"
enabled = true

[devices.lists.settings]
only_unread = true
limit = true
limit_value = 50
```

## Command-Line Flags

All configuration options can be set via command-line flags:

### Server Command

```bash
comic-server server [flags]
```

**Required Flags:**

- `--library PATH` - Path to ComicRack library file (ComicDb.xml)

**Optional Flags:**

- `--config PATH` - Path to configuration file
- `--auto-sync` - Automatically sync devices on discovery (default: false)
- `--log-level LEVEL` - Logging level: debug, info, warn, error (default: info)
- `--log-format FORMAT` - Log format: text, json (default: text)
- `--discovery-port PORT` - UDP discovery port (default: 7615)
- `--command-port PORT` - TCP command port (default: 7614)
- `--api-port PORT` - REST API port (default: 7620)
- `--ignore-device DEVICE` - Ignore device by IP, ID, or name (can be specified multiple times)
- `--max-concurrent INT` - Maximum concurrent connections (default: 10)
- `--max-per-ip INT` - Maximum connections per IP (default: 3)
- `--max-per-device INT` - Maximum requests per device (default: 100)

**Examples:**

```bash
# Basic usage
comic-server server --library ~/.local/share/ComicRack/ComicDb.xml

# With auto-sync enabled
comic-server server --library ~/.local/share/ComicRack/ComicDb.xml --auto-sync

# Debug logging in JSON format
comic-server server --library ~/.local/share/ComicRack/ComicDb.xml \
  --log-level debug --log-format json

# Ignore specific devices
comic-server server --library ~/.local/share/ComicRack/ComicDb.xml \
  --ignore-device 192.168.0.24 \
  --ignore-device "Galaxy Tab"

# Custom ports
comic-server server --library ~/.local/share/ComicRack/ComicDb.xml \
  --discovery-port 8615 \
  --command-port 8614 \
  --api-port 8620

# With connection limits
comic-server server --library ~/.local/share/ComicRack/ComicDb.xml \
  --max-concurrent 20 \
  --max-per-ip 5 \
  --max-per-device 200
```

## Environment Variables

All configuration options can be set via environment variables with the `COMIC_SERVER_` prefix:

### Server Configuration

- `COMIC_SERVER_LIBRARY_PATH` - Library file path
- `COMIC_SERVER_AUTO_SYNC` - Auto-sync on discovery (true/false)
- `COMIC_SERVER_LOG_LEVEL` - Logging level (debug/info/warn/error)
- `COMIC_SERVER_LOG_FORMAT` - Log format (text/json)
- `COMIC_SERVER_DISCOVERY_PORT` - Discovery port (default: 7615)
- `COMIC_SERVER_COMMAND_PORT` - Command port (default: 7614)
- `COMIC_SERVER_API_PORT` - API port (default: 7620)
- `COMIC_SERVER_MAX_CONCURRENT` - Maximum concurrent connections
- `COMIC_SERVER_MAX_PER_IP` - Maximum connections per IP
- `COMIC_SERVER_MAX_PER_DEVICE` - Maximum requests per device

**Examples:**

```bash
# Set library path
export COMIC_SERVER_LIBRARY_PATH="$HOME/.local/share/ComicRack/ComicDb.xml"

# Enable auto-sync
export COMIC_SERVER_AUTO_SYNC=true

# Set debug logging
export COMIC_SERVER_LOG_LEVEL=debug
export COMIC_SERVER_LOG_FORMAT=json

# Run server
comic-server server
```

### Using with systemd

Add environment variables to systemd service:

```ini
[Service]
Environment="COMIC_SERVER_LIBRARY_PATH=/home/user/.local/share/ComicRack/ComicDb.xml"
Environment="COMIC_SERVER_AUTO_SYNC=true"
Environment="COMIC_SERVER_LOG_LEVEL=info"
```

### Using with Docker

Pass environment variables to container:

```bash
docker run -d \
  -e COMIC_SERVER_LIBRARY_PATH=/data/ComicDb.xml \
  -e COMIC_SERVER_AUTO_SYNC=true \
  -e COMIC_SERVER_LOG_LEVEL=info \
  --network host \
  comic-server
```

## Configuration Options Reference

### Server Settings

#### library_path (required)

Path to the ComicRack library XML file.

**Type:** string
**Default:** (none - must be specified)
**Example:** `/home/user/.local/share/ComicRack/ComicDb.xml`

Common locations:
- Linux: `~/.local/share/ComicRack/ComicDb.xml`
- macOS: `~/Library/Application Support/ComicRack/ComicDb.xml`
- Windows: `%APPDATA%\ComicRack\ComicDb.xml`

#### auto_sync

Automatically initiate sync when a device is discovered.

**Type:** boolean
**Default:** `false`
**Recommended:** `true` for automated setups, `false` for manual control

When enabled, the server will immediately start syncing discovered devices. When disabled, devices are registered but sync must be triggered manually via API.

#### discovery_port

UDP port for device discovery (multicast).

**Type:** integer
**Default:** `7615`
**Range:** 1024-65535
**Note:** Changing this requires modifying device apps

#### command_port

TCP port for device communication.

**Type:** integer
**Default:** `7614`
**Range:** 1024-65535
**Note:** Changing this requires modifying device apps

#### api_port

TCP port for REST API and Prometheus metrics.

**Type:** integer
**Default:** `7620`
**Range:** 1024-65535

### Logging Settings

#### log_level

Logging verbosity level.

**Type:** string
**Default:** `info`
**Options:**
- `debug` - Verbose logging for development
- `info` - Normal operational logging
- `warn` - Warnings and errors only
- `error` - Errors only

**Recommendation:** Use `info` for production, `debug` for troubleshooting

#### log_format

Log output format.

**Type:** string
**Default:** `text`
**Options:**
- `text` - Human-readable format (colorized when outputting to terminal)
- `json` - Structured JSON format (for log aggregation)

**Recommendation:** Use `text` for local debugging, `json` for production/monitoring

### Security Settings

#### max_concurrent_connections

Maximum number of concurrent device connections.

**Type:** integer
**Default:** `10`
**Range:** 1-1000

Limits the total number of devices that can connect simultaneously. Prevents resource exhaustion.

#### max_connections_per_ip

Maximum connections allowed from a single IP address.

**Type:** integer
**Default:** `3`
**Range:** 1-100

Prevents a single IP from monopolizing server resources. Useful for rate limiting.

#### max_requests_per_device

Maximum number of requests a device can make per minute.

**Type:** integer
**Default:** `100`
**Range:** 1-10000

Rate limits individual devices to prevent abuse. Set to 0 to disable.

### Device Management

#### ignore_devices

List of devices to ignore during discovery and sync.

**Type:** array of strings
**Default:** `[]` (empty list)

Devices can be identified by:
- IP address (e.g., `192.168.0.24`)
- Device ID (e.g., `SM-T970`)
- Device name (e.g., `"Production Tablet"`)

**Example:**
```yaml
server:
  ignore_devices:
    - "192.168.0.24"
    - "tablet-abc123"
    - "Work Tablet"
```

**Use cases:**
- Protecting production devices during development
- Temporarily disabling specific devices
- Testing with subset of devices

### Per-Device Configuration

#### Device Block

Configure sync settings for specific devices.

**Structure:**
```yaml
devices:
  device-id-here:
    device_id: "device-id-here"
    device_name: "Friendly Name"
    lists:
      - list_id: "uuid-here"
        list_name: "List Name"
        enabled: true
        settings:
          only_unread: true
          limit: true
          limit_value: 50
```

#### Device Settings

- **device_id** (string, required) - Unique device identifier
- **device_name** (string, optional) - Friendly name for logging

#### List Settings

- **list_id** (string, required) - UUID of the smart list
- **list_name** (string, optional) - Name for logging/reference
- **enabled** (boolean, required) - Whether to sync this list
- **settings.only_unread** (boolean, optional) - Only sync unread comics
- **settings.limit** (boolean, optional) - Enable book limit
- **settings.limit_value** (integer, optional) - Maximum books to sync (if limit is true)

**Example use cases:**

1. **Limit sync to recent unread comics:**
```yaml
settings:
  only_unread: true
  limit: true
  limit_value: 50
```

2. **Sync entire collection:**
```yaml
settings:
  only_unread: false
  limit: false
```

3. **Testing with small subset:**
```yaml
settings:
  only_unread: false
  limit: true
  limit_value: 10
```

## Configuration Reload

The server supports configuration reload without restart using SIGHUP:

```bash
# Linux/macOS
kill -HUP $(pgrep comic-server)

# systemd
sudo systemctl reload comic-server

# Docker
docker kill -s HUP comic-server
```

**What gets reloaded:**
- Log level and format
- Device ignore list
- Per-device configurations
- Connection limits

**What requires restart:**
- Library path
- Port numbers
- Auto-sync setting

## Finding Configuration Values

### Finding Device IDs

Use the devices API endpoint:

```bash
curl http://localhost:7620/api/devices | jq '.devices[] | {id, name, ip}'
```

### Finding List IDs

List IDs are UUIDs from the ComicRack library. To find them:

1. Open ComicRack desktop application
2. Right-click on a smart list
3. View properties
4. Copy the list ID

Or parse the library XML:

```bash
grep -A 2 "SmartList" ~/.local/share/ComicRack/ComicDb.xml | grep 'Id=' | sed 's/.*Id="\([^"]*\)".*/\1/'
```

## Validation

The server validates configuration on startup and will exit with an error message if invalid.

Common validation errors:

- Library file not found or not readable
- Invalid port numbers (< 1024 or > 65535)
- Invalid log level or format
- Malformed device configuration

Check logs for detailed validation errors:

```bash
comic-server server --library /path/to/ComicDb.xml --log-level debug
```

## Best Practices

### Production Configuration

```yaml
server:
  library_path: "/var/lib/comic-server/ComicDb.xml"
  auto_sync: true
  log_level: "info"
  log_format: "json"
  max_concurrent_connections: 20
  max_connections_per_ip: 5
  max_requests_per_device: 200
```

### Development Configuration

```yaml
server:
  library_path: "./testdata/ComicDb.xml"
  auto_sync: false
  log_level: "debug"
  log_format: "text"
  ignore_devices:
    - "192.168.0.24"  # Production tablet
```

### Minimal Configuration

```bash
# Just specify the library path
comic-server server --library ~/.local/share/ComicRack/ComicDb.xml
```

## Troubleshooting

### Configuration Not Loading

Check configuration file location and permissions:

```bash
ls -l ~/.config/comic-server/config.yaml
```

### Values Not Taking Effect

Remember precedence order:
1. Command-line flags (highest)
2. Environment variables
3. Configuration file (lowest)

Use debug logging to see loaded configuration:

```bash
comic-server server --library /path/to/library.xml --log-level debug
```

### Invalid Configuration

The server will print detailed error messages on startup:

```
Error: invalid configuration: library_path is required
```

Check the error message and fix the configuration accordingly.

## See Also

- [Installation Guide](INSTALLATION.md) - Installing and setting up the server
- [API Reference](API.md) - REST API and monitoring
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions
