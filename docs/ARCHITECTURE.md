# Comic Server Architecture

This document describes the architecture of comic-server, a headless ComicRack wireless sync server.

## Table of Contents

- [Overview](#overview)
- [System Architecture](#system-architecture)
- [Data Storage Strategy](#data-storage-strategy)
- [Component Details](#component-details)
- [Communication Protocols](#communication-protocols)
- [Migration Roadmap](#migration-roadmap)

## Overview

Comic-server is a standalone Go application that implements the ComicRack wireless synchronization protocol, allowing Android and iOS ComicRack clients to sync without requiring the full ComicRack desktop application.

**Key Goals:**
- Headless operation (no GUI required)
- ComicRack protocol compatibility
- Web-based management UI
- Extensible for future ComicRack replacement

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                       comic-server                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
│  │   Discovery  │  │   Protocol   │  │     Sync     │    │
│  │   (UDP)      │  │   (TCP)      │  │    Engine    │    │
│  └──────────────┘  └──────────────┘  └──────────────┘    │
│         │                  │                  │            │
│  ┌──────────────────────────────────────────────────┐     │
│  │           Device Registry                        │     │
│  └──────────────────────────────────────────────────┘     │
│         │                                                  │
│  ┌──────────────────────────────────────────────────┐     │
│  │           Library Management                     │     │
│  │  (XML parsing, smart list evaluation)           │     │
│  └──────────────────────────────────────────────────┘     │
│         │                                                  │
│  ┌──────────────────────────────────────────────────┐     │
│  │        Configuration & Storage                   │     │
│  │  v0.7: YAML + XML | v0.8+: SQLite               │     │
│  └──────────────────────────────────────────────────┘     │
│                                                             │
│  ┌──────────────────────────────────────────────────┐     │
│  │            REST API + WebSocket                  │     │
│  │  (Web UI, monitoring, configuration)            │     │
│  └──────────────────────────────────────────────────┘     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
           │                          │
    ┌──────▼──────┐          ┌────────▼────────┐
    │  ComicRack  │          │    Web UI       │
    │   Clients   │          │   (Browser)     │
    │ (Android/iOS)│         └─────────────────┘
    └─────────────┘
```

## Data Storage Strategy

Comic-server uses a phased migration approach to transition from ComicRackCE's XML format to a modern SQLite database.

### Current State (v0.7)

**Library Data:** ComicDB.xml (read-only)
- Comics metadata, reading lists, smart lists
- Managed by ComicRackCE or imported from existing installation
- Server parses XML for sync operations

**Configuration:** config.yaml (read-write)
- Server settings (ports, library path, rate limits)
- Device registrations
- Smart list assignments per device
- Sync settings

**Structure:**
```yaml
server:
  library_path: "/path/to/ComicDb.xml"
  server_port: 7620
  discovery_port: 7615
  auto_sync: false
  max_concurrent_connections: 5

devices:
  "device-guid-123":
    device_id: "device-guid-123"
    friendly_name: "Samsung Galaxy Tab"
    last_seen: 2025-01-15T10:30:00Z
    lists:
      - list_id: "list-guid-456"
        list_name: "To Read"
        enabled: true
        settings: null  # Uses device defaults
    default_settings:
      max_books: 100
      sync_covers: true
```

### Future (v0.8+): SQLite Migration

**Phase 1 (v0.8):** Dual Format Support
- SQLite database introduced alongside XML
- Import tool: XML → SQLite
- Export tool: SQLite → XML (for compatibility)
- Server reads from either source

**Phase 2 (v0.9):** SQLite Primary
- SQLite becomes primary storage
- XML export for backup/compatibility
- New features use SQLite exclusively

**Phase 3 (v1.0+):** Full Library Management
- Web UI for library management
- Comic metadata editing
- Smart list creation/editing
- File organization tools
- API-based metadata fetching

### Proposed SQLite Schema (v0.8+)

```sql
-- Comics and metadata
CREATE TABLE comics (
    id INTEGER PRIMARY KEY,
    guid TEXT UNIQUE NOT NULL,
    series TEXT,
    title TEXT,
    volume INTEGER,
    issue_number TEXT,
    publisher TEXT,
    release_date DATE,
    file_path TEXT,
    file_size INTEGER,
    page_count INTEGER,
    added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    modified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Smart lists
CREATE TABLE lists (
    id INTEGER PRIMARY KEY,
    guid TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,  -- 'static', 'smart'
    matcher_mode TEXT,    -- 'and', 'or'
    parent_id INTEGER REFERENCES lists(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Smart list matchers
CREATE TABLE list_matchers (
    id INTEGER PRIMARY KEY,
    list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    field TEXT NOT NULL,
    operator TEXT NOT NULL,
    value TEXT,
    negate BOOLEAN DEFAULT FALSE
);

-- Static list items
CREATE TABLE list_items (
    list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    comic_id INTEGER NOT NULL REFERENCES comics(id) ON DELETE CASCADE,
    position INTEGER,
    PRIMARY KEY (list_id, comic_id)
);

-- Devices
CREATE TABLE devices (
    id TEXT PRIMARY KEY,  -- Device GUID
    friendly_name TEXT,
    model TEXT,
    manufacturer TEXT,
    edition TEXT,
    last_seen TIMESTAMP,
    registered BOOLEAN DEFAULT FALSE
);

-- Device list assignments
CREATE TABLE device_lists (
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    enabled BOOLEAN DEFAULT TRUE,
    max_books INTEGER,
    sync_covers BOOLEAN,
    PRIMARY KEY (device_id, list_id)
);

-- Sync history
CREATE TABLE sync_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL REFERENCES devices(id),
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    status TEXT NOT NULL,  -- 'in_progress', 'completed', 'failed'
    books_added INTEGER DEFAULT 0,
    books_updated INTEGER DEFAULT 0,
    books_deleted INTEGER DEFAULT 0,
    error_count INTEGER DEFAULT 0,
    error_message TEXT
);

-- Metadata cache (for API lookups)
CREATE TABLE metadata_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    comic_id INTEGER UNIQUE REFERENCES comics(id) ON DELETE CASCADE,
    source TEXT NOT NULL,  -- 'comicvine', 'marvel', etc.
    external_id TEXT,
    fetched_at TIMESTAMP,
    raw_data TEXT  -- JSON blob
);

-- Indexes
CREATE INDEX idx_comics_series ON comics(series);
CREATE INDEX idx_comics_publisher ON comics(publisher);
CREATE INDEX idx_lists_type ON lists(type);
CREATE INDEX idx_sync_history_device ON sync_history(device_id);
CREATE INDEX idx_sync_history_started ON sync_history(started_at);
```

## Component Details

### Discovery (internal/device/discovery.go)

**Purpose:** Discover ComicRack devices on the local network

**Protocol:** UDP multicast on 224.34.123.90:7615

**Broadcast Format:**
```
ComicRack[Variant]:{device_id}[:Sync]
```

**Variants:**
- `ComicRack` - Desktop edition
- `ComicRackAndroid` - Android app
- `ComicRackiOS` - iOS app

**Flow:**
1. Server joins multicast group
2. Listens for device broadcasts
3. Parses device ID and sync request flag
4. Sends to device registry for processing

### Protocol Handler (internal/protocol/)

**Purpose:** Binary protocol communication with devices

**Port:** TCP 7614 (device communication)

**Encoding:** Big-endian binary

**Data Types:**
- `INT` (4 bytes): Signed 32-bit integer
- `LONG` (8 bytes): Signed 64-bit integer
- `STRING`: INT(length) + UTF-8 bytes
- `BOOL` (1 byte): 0 or 1
- `DATA`: INT(length) + raw bytes

**Commands:**
- `ReadFile` - Read file from device
- `WriteFile` - Write file to device
- `FileExists` - Check if file exists
- `DeleteFile` - Delete file from device
- `CommandInfo` - Get device info (comicrack.ini)

### Sync Engine (internal/sync/)

**Purpose:** Synchronize comics to devices based on smart lists

**Key Concepts:**

1. **Smart List Filtering:**
   - Evaluates smart list matchers against library
   - Supports multiple lists (union of all matches)
   - Configurable per-device settings

2. **Sync Settings:**
   ```go
   type SharedListSettings struct {
       MaxBooks      int  // Book limit (0 = unlimited)
       SyncCovers    bool // Include cover images
       SyncMetadata  bool // Include metadata
   }
   ```

3. **Device Limits:**
   - Android Free Edition: 100 books max
   - Android Full/iOS: Unlimited
   - Enforced during sync operation

**Sync Flow:**
1. Load device configuration (smart lists + settings)
2. Evaluate smart list matchers against library
3. Generate list of matching comics
4. Apply device book limit
5. Compare with device's current library
6. Send new/updated comics, delete removed comics
7. Update sync history

### Device Registry (internal/device/registry.go)

**Purpose:** Track discovered devices and their state

**Device Info:**
```go
type Info struct {
    ID           string
    Name         string
    Model        string
    Manufacturer string
    Edition      EditionType
    Version      int
    Hash         string  // SHA1 validation
    Capabilities []string
}
```

**Device State:**
- Last seen timestamp
- Currently syncing flag
- Registration status (in-memory + config persistence)

### Library Management (internal/library/)

**Purpose:** Parse and query ComicDB.xml library

**Key Structures:**
```go
type ComicLibrary struct {
    Books      []ComicBook
    ComicLists []ComicListItem
}

type ComicBook struct {
    GUID      string
    Series    string
    Title     string
    Volume    int
    Publisher string
    // ... full metadata
}

type ComicListItem struct {
    ID          string
    Name        string
    Type        string  // "ComicSmartListItem"
    MatcherMode string  // "And", "Or"
    Matchers    []ComicBookMatcher
}
```

**Matcher Evaluation:**
- Field-based filtering (series, publisher, year, etc.)
- Operator support (equals, contains, greater than, etc.)
- Boolean logic (AND/OR combinations)

### REST API + WebSocket (internal/api/)

**Purpose:** Web UI backend and real-time updates

**REST Endpoints:**

**Core:**
- `GET /api/health` - Server health check
- `GET /api/version` - Build version info
- `GET /api/stats` - Server statistics

**Devices:**
- `GET /api/devices` - List discovered devices
- `POST /api/devices/register` - Register device
- `POST /api/devices/unregister` - Unregister device
- `GET /api/devices/config/{deviceId}` - Get device config

**Library:**
- `GET /api/library/lists` - List smart lists

**Device Lists:**
- `POST /api/devices/lists/{deviceId}` - Add list to device
- `DELETE /api/devices/lists/{deviceId}/{listId}` - Remove list

**Sync:**
- `GET /api/sync/status` - Active sync operations
- `GET /api/sync/history` - Sync history (paginated)

**WebSocket Events:**
- `device_discovered` - New device found
- `device_connected` - Device connected
- `device_disconnected` - Device disconnected
- `device_registered` - Device registered
- `device_unregistered` - Device unregistered
- `device_updated` - Device config changed
- `sync_started` - Sync operation started
- `sync_progress` - Sync progress update
- `sync_completed` - Sync completed
- `sync_failed` - Sync failed

## Communication Protocols

### Device Discovery Protocol

**Multicast Group:** 224.34.123.90:7615

**Message Format:**
```
ComicRack[Variant]:{device_guid}[:Sync]
```

**Examples:**
```
ComicRackAndroid:a1b2c3d4-e5f6-7890-abcd-ef1234567890
ComicRackiOS:b2c3d4e5-f6a7-8901-bcde-f12345678901:Sync
```

### Device Communication Protocol

**Port:** TCP 7614

**Command Structure:**
```
[Command Code: INT] [Parameters...]
```

**Common Commands:**

**ReadFile:**
```
[0x01] [filename: STRING] → [data: DATA]
```

**WriteFile:**
```
[0x02] [filename: STRING] [data: DATA] → [success: BOOL]
```

**CommandInfo (Get device info):**
```
[0x08] → [comicrack.ini: DATA]
```

## Migration Roadmap

### v0.7 (Current)

**Status:** ✅ Complete

**Features:**
- XML library parsing
- YAML configuration
- Device registration persistence
- Smart list assignment via Web UI
- Full sync engine
- Web dashboard

**Storage:**
- ComicDB.xml (read-only)
- config.yaml (read-write)

### v0.8 (Next Major Release)

**Target:** Q2 2025

**Features:**
- SQLite database introduction
- XML → SQLite import tool
- SQLite → XML export tool
- Dual-format library support
- Migration utilities

**Storage:**
- SQLite (primary for new installations)
- ComicDB.xml (legacy support)
- config.yaml (device settings migration to DB)

**Migration Path:**
1. User runs `comic-server migrate import`
2. Tool parses ComicDB.xml
3. Populates SQLite database
4. Validates migration
5. Server switches to SQLite mode

### v0.9 (Future)

**Target:** Q4 2025

**Features:**
- SQLite is default/primary
- XML support deprecated (warning only)
- Advanced querying capabilities
- Performance optimizations

### v1.0 (Long-term Goal)

**Target:** 2026

**Features:**
- Full library management in Web UI
- Comic metadata editing
- Smart list builder UI
- File organization tools
- Metadata API integrations (ComicVine, etc.)
- Multi-user support
- Role-based access control

**ComicRackCE Replacement:**
At v1.0, comic-server will be a complete replacement for ComicRackCE's library management features, offering:
- Headless operation
- Web-based interface
- Cross-platform support
- Modern database backend
- API-first architecture

## Design Principles

1. **Backward Compatibility:** Maintain ComicRack protocol compatibility
2. **Gradual Migration:** Phased approach to database migration
3. **API-First:** All features accessible via REST API
4. **Real-time Updates:** WebSocket for live dashboard updates
5. **Graceful Degradation:** Log errors, don't fail requests
6. **Database-Ready:** Config structures prepared for SQLite migration
7. **Stateless where possible:** Config/DB for persistence, not in-memory state

## Security Considerations

**Current (v0.7):**
- No authentication/authorization
- Local network only
- Rate limiting (IP + device)
- Connection limits

**Future (v0.8+):**
- Optional authentication
- Token-based API access
- User account system
- Role-based permissions
- HTTPS support

## Performance Characteristics

**Sync Performance:**
- Concurrent sync limit (default: 5)
- Per-device rate limiting (default: 100 req/min)
- Per-IP rate limiting (default: 10 conn/min)

**Library Loading:**
- Full XML parse on startup
- Cached in memory
- SIGHUP to reload without restart

**Database (v0.8+):**
- Indexed queries for fast lookups
- Connection pooling
- Prepared statements
- Transaction batching for bulk operations

## Monitoring & Observability

**Metrics (Prometheus):**
- Active sync count
- Sync success/failure rates
- Device connection counts
- API request rates
- Library size statistics

**Logging:**
- Structured logging (zerolog)
- Configurable levels (debug, info, warn, error)
- JSON or text output
- Per-request context

**Web Dashboard:**
- Real-time sync status
- Device list with status
- Sync history with pagination
- Server statistics

---

**Document Version:** 1.0
**Last Updated:** 2025-01-15
**Author:** Claude Code
