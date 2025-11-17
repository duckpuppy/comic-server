# comic-server Features

This document describes the key features of comic-server.

## Table of Contents

- [Bidirectional Sync](#bidirectional-sync)
- [Smart List Filtering](#smart-list-filtering)
- [Per-Device Configuration](#per-device-configuration)
- [Web UI](#web-ui)
- [REST API & Prometheus Metrics](#rest-api--prometheus-metrics)
- [Security](#security)

## Bidirectional Sync

comic-server supports **bidirectional synchronization** between your library and devices:

### Forward Sync (Server → Device)

When a device syncs, the server:
- Transfers comic book files (.cbp format)
- Syncs metadata (titles, authors, ratings, etc.)
- Syncs reading lists
- Applies smart list filtering
- Respects per-device sync settings (OnlyUnread, Limit, Sort, etc.)

### Reverse Sync (Device → Server) ✨

**NEW in v0.8!** When a device syncs, the server automatically updates the library with changes from the device:

#### Reading Progress
- **CurrentPage**: The page you're currently on
- **LastPageRead**: The last page you viewed
- **OpenCount**: How many times you've opened the book
- **OpenedTime**: When you last opened the book

#### User Metadata
- **Rating**: Your star rating (0-5 stars)
- **Notes**: Personal notes about the comic
- **Review**: Your review text
- **Summary**: Custom summary
- **Tags**: Custom tags (comma-separated)

#### Page Metadata
- **Bookmarks**: Named bookmarks on specific pages
- **Page Types**: Front cover, back cover, story, advertisement, etc.

#### Other
- **Checked**: Whether you've marked the book as checked

### How It Works

1. Device initiates sync
2. **Reverse sync runs FIRST** - Server reads device state and updates library
3. Library is saved to disk (if changes detected)
4. Forward sync runs - Server sends updated books to device
5. Sync completes

This ensures that your reading progress is never lost, even if the library has been updated since your last sync.

### Example Workflow

```
1. You read "Batman #1" on your tablet to page 15
2. You rate it 5 stars
3. You add a bookmark on page 10 called "Epic Battle"
4. You sync with the server
5. Server updates library:
   - CurrentPage: 15
   - Rating: 5.0
   - Pages: [... Page 10: Bookmark="Epic Battle" ...]
6. Library file is saved to disk
7. Forward sync continues (if needed)
```

### Performance Optimization

The server is smart about when to save the library:
- ✅ Saves when metadata changes are detected
- ❌ Skips save when no changes (optimization for large libraries)
- 📊 Logs which fields changed for debugging

## Smart List Filtering

Control which comics sync to each device using **smart lists** from your ComicRack library.

### What are Smart Lists?

Smart lists are dynamic filters defined in your ComicRack library (ComicDb.xml). They use rules to match comics based on:
- Series name
- Publisher
- Year range
- Genre
- Rating
- Read status
- Page count
- Custom tags
- And many more...

### How to Use

```bash
# List available smart lists
comic-server config list-smartlists --library /path/to/ComicDb.xml

# Add a list to a device
comic-server config add-list "My Tablet" "Currently Reading" \
  --library /path/to/ComicDb.xml \
  --only-unread \
  --limit 50

# Multiple lists per device
comic-server config add-list "My Tablet" "Marvel Comics" --library /path/to/ComicDb.xml
comic-server config add-list "My Tablet" "DC Comics" --library /path/to/ComicDb.xml
```

### Multiple Lists Per Device

Devices can have **multiple smart lists** assigned:
- Books from all lists are synced (union of all lists)
- Books appearing in multiple lists are automatically deduplicated
- Each list can have its own sync settings

## Per-Device Configuration

Each device can have its own sync settings stored in the config file.

### Device-Level Settings

```yaml
devices:
  my-tablet:
    device_id: "tablet-abc123"
    friendly_name: "Samsung Galaxy Tab"
    lists:
      - list_id: "{GUID-1234}"
        list_name: "Currently Reading"
        enabled: true
        settings:
          only_unread: true    # Only sync unread books
          limit: true          # Limit number of books
          limit_value: 50      # Max 50 books
          limit_type: books    # Count books (vs. size in MB)
          sort: true           # Sort before limiting
          list_sort_type: series  # Sort by series name
```

### Sync Options

| Option | Description | Values |
|--------|-------------|--------|
| `only_unread` | Only sync unread books | `true` / `false` |
| `only_checked` | Only sync checked books | `true` / `false` |
| `limit` | Enable book limit | `true` / `false` |
| `limit_value` | Max books or MB | Number |
| `limit_type` | Limit by count or size | `books` / `size` |
| `sort` | Sort books before limiting | `true` / `false` |
| `list_sort_type` | Sort method | `series`, `published`, `added` |
| `keep_last_read` | Keep most recently read book | `true` / `false` |

### Example Scenarios

**Scenario 1: Tablet with limited storage**
```yaml
settings:
  limit: true
  limit_value: 25
  limit_type: books
  only_unread: true
  keep_last_read: true
```
Syncs max 25 unread books, plus keeps the last book you were reading.

**Scenario 2: Phone for commute reading**
```yaml
settings:
  limit: true
  limit_value: 500
  limit_type: size  # 500 MB
  sort: true
  list_sort_type: published
```
Syncs most recent 500 MB of books (by publication date).

**Scenario 3: Unlimited tablet**
```yaml
settings:
  only_unread: false
  limit: false
```
Syncs all books from the smart lists (no filtering).

## Web UI

comic-server includes a built-in web interface for monitoring and management.

### Access

Open your browser to: `http://server-ip:7620/`

### Features

**Dashboard:**
- Real-time server stats (uptime, devices, syncs)
- WebSocket updates (no manual refresh needed)
- Active sync progress with progress bars
- Recent sync history

**Device Management:**
- View all discovered devices
- Register/unregister devices
- View device details (model, IP, edition, last seen)
- View per-device sync history

**Smart Lists Browser:**
- Browse all smart lists in library
- View list details (matchers, devices, book preview)
- Search and filter lists
- Navigate with tree sidebar

**Sync History:**
- Paginated sync history
- Filter by device or status
- View file statistics (added/updated/deleted)
- Duration and timestamps

### Technology

- Vanilla JavaScript (no framework dependencies)
- Client-side routing (History API)
- WebSocket for real-time updates
- Responsive CSS Grid/Flexbox layout
- Embedded in binary (no external files needed)

## REST API & Prometheus Metrics

comic-server exposes a REST API and Prometheus metrics for monitoring and integration.

### REST API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/health` | GET | Health check with uptime and version |
| `/api/version` | GET | Build version information |
| `/api/devices` | GET | List all registered devices |
| `/api/devices?edition=Android+Full` | GET | Filter devices by edition |
| `/api/devices?syncing=true` | GET | Filter currently syncing devices |
| `/api/sync/status` | GET | Get all active syncs |
| `/api/sync/history` | GET | Get recent sync history (paginated) |
| `/api/stats` | GET | Server statistics and configuration |
| `/metrics` | GET | Prometheus metrics |

See [docs/API.md](API.md) for full API reference.

### Prometheus Metrics

```
# Sync operations counter
comic_server_syncs_total{status="completed|failed|aborted"}

# Active syncs gauge
comic_server_active_syncs

# Books processed counter
comic_server_books_processed_total{operation="added|updated|deleted"}

# Sync duration histogram
comic_server_sync_duration_seconds
```

### Example Prometheus Configuration

```yaml
scrape_configs:
  - job_name: 'comic-server'
    static_configs:
      - targets: ['localhost:7620']
    scrape_interval: 15s
```

### Example Grafana Dashboard

Key metrics to monitor:
- Active syncs over time
- Sync success/failure rate
- Books processed per sync
- Sync duration (p50, p95, p99)
- Device count (registered, active, syncing)

## Security

comic-server includes multiple security features:

### Device Authentication

- SHA1 hash validation of device metadata
- Device registry tracks known devices
- Device ignore/filter lists for blocking unwanted devices

### Rate Limiting

**IP-based rate limiting (sliding window):**
- Limits connections per IP address
- Configurable window size and max connections
- Prevents connection flooding

**Device-based rate limiting (token bucket):**
- Limits sync operations per device
- Configurable refill rate and bucket size
- Prevents sync spam

### Resource Management

- Concurrent connection limits (semaphore-based)
- Per-device sync state tracking
- Prevents same device from syncing twice simultaneously

### Configuration

```yaml
server:
  # Ignore specific devices
  ignore_devices:
    - "192.168.0.24"     # By IP
    - "SM-T970"          # By device ID
    - "Untrusted Tablet" # By device name

  # Connection limits
  max_concurrent_sync: 5  # Max simultaneous syncs (0 = unlimited)
```

See [docs/SECURITY.md](SECURITY.md) for detailed security documentation.

## Platform Support

### Server

- **Linux**: amd64, arm64 (Raspberry Pi 4+)
- **macOS**: Intel, Apple Silicon (M1/M2/M3)
- **Windows**: amd64

### Devices

- **Android**: ComicRack Free, ComicRack Full
- **iOS**: ComicRack for iOS

### Deployment

- **Binary**: Direct installation
- **Docker**: Multi-platform images (linux/amd64, linux/arm64)
- **systemd**: Linux service
- **launchd**: macOS service

## See Also

- [Installation Guide](INSTALLATION.md)
- [Configuration Reference](CONFIGURATION.md)
- [API Reference](API.md)
- [Troubleshooting Guide](TROUBLESHOOTING.md)
- [Docker Deployment Guide](DOCKER.md)
- [Architecture Overview](ARCHITECTURE.md)
