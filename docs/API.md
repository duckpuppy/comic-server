# API Reference

This document describes the REST API provided by comic-server for monitoring and management.

## Overview

The API server runs on port 7620 (configurable) and provides:
- Health checks and version information
- Device monitoring
- Sync status and history
- Server statistics
- Prometheus metrics

## Base URL

Default: `http://localhost:7620`

Custom port: Use `--api-port` flag or `COMIC_SERVER_API_PORT` environment variable.

## Authentication

Currently, the API does not require authentication. It is intended for local monitoring and should not be exposed to untrusted networks.

Future versions may add authentication options.

## Endpoints

### GET /api/health

Health check endpoint with version information.

**Response:**
```json
{
  "status": "healthy",
  "uptime": "2h34m12s",
  "version": "0.6.0",
  "git_commit": "abc1234",
  "build_date": "2025-01-15T10:30:00Z"
}
```

**Fields:**
- `status` (string) - Always "healthy" if server is running
- `uptime` (string) - Time since server start
- `version` (string) - Server version
- `git_commit` (string) - Git commit hash
- `build_date` (string) - Build timestamp (RFC3339)

**Status Codes:**
- `200 OK` - Server is healthy

**Example:**
```bash
curl http://localhost:7620/api/health
```

---

### GET /api/version

Server version information.

**Response:**
```json
{
  "version": "0.6.0",
  "git_commit": "abc1234",
  "build_date": "2025-01-15T10:30:00Z"
}
```

**Fields:**
- `version` (string) - Server version
- `git_commit` (string) - Git commit hash
- `build_date` (string) - Build timestamp (RFC3339)

**Status Codes:**
- `200 OK` - Success

**Example:**
```bash
curl http://localhost:7620/api/version
```

---

### GET /api/sync/status

Get currently active sync operations.

**Response:**
```json
{
  "active_syncs": [
    {
      "device_id": "SM-T970",
      "device_ip": "192.168.0.100",
      "device_name": "Samsung Galaxy Tab",
      "start_time": "2025-01-15T14:30:00Z",
      "end_time": null,
      "status": "in_progress",
      "progress": 45,
      "books_total": 100,
      "books_added": 30,
      "books_updated": 15,
      "books_deleted": 0,
      "error_count": 0
    }
  ],
  "active_count": 1
}
```

**Fields:**
- `active_syncs` (array) - List of active sync operations
  - `device_id` (string) - Unique device identifier
  - `device_ip` (string) - Device IP address
  - `device_name` (string) - Friendly device name (optional)
  - `start_time` (string) - Sync start time (RFC3339)
  - `end_time` (string|null) - Sync end time (null if ongoing)
  - `status` (string) - Sync status: "starting", "in_progress", "completed", "failed", "aborted"
  - `progress` (integer) - Completion percentage (0-100)
  - `books_total` (integer) - Total books to process
  - `books_added` (integer) - Books added to device
  - `books_updated` (integer) - Books updated on device
  - `books_deleted` (integer) - Books deleted from device
  - `error_count` (integer) - Number of errors encountered
  - `error_message` (string) - Error details (only present if failed)
- `active_count` (integer) - Number of active syncs

**Status Codes:**
- `200 OK` - Success

**Example:**
```bash
curl http://localhost:7620/api/sync/status
```

---

### GET /api/sync/history

Get sync operation history with optional pagination.

#### Legacy Mode (without pagination)

**Query Parameters:**
- `limit` (integer, optional) - Maximum number of entries to return (default: 20, max: 100)

**Response:**
```json
{
  "history": [
    {
      "device_id": "SM-T970",
      "device_ip": "192.168.0.100",
      "device_name": "Samsung Galaxy Tab",
      "start_time": "2025-01-15T14:00:00Z",
      "end_time": "2025-01-15T14:15:23Z",
      "status": "completed",
      "progress": 100,
      "books_total": 50,
      "books_added": 25,
      "books_updated": 20,
      "books_deleted": 5,
      "error_count": 0
    }
  ],
  "count": 1
}
```

**Example:**
```bash
# Get last 20 syncs
curl http://localhost:7620/api/sync/history

# Get last 50 syncs
curl http://localhost:7620/api/sync/history?limit=50
```

#### Paginated Mode (with offset)

**Query Parameters:**
- `limit` (integer, optional) - Page size (default: 20, max: 100)
- `offset` (integer, required) - Number of entries to skip (0 = most recent)

**Response:**
```json
{
  "history": [
    {
      "device_id": "SM-T970",
      "device_ip": "192.168.0.100",
      "device_name": "Samsung Galaxy Tab",
      "start_time": "2025-01-15T14:00:00Z",
      "end_time": "2025-01-15T14:15:23Z",
      "status": "completed",
      "progress": 100,
      "books_total": 50,
      "books_added": 25,
      "books_updated": 20,
      "books_deleted": 5,
      "error_count": 0
    }
  ],
  "total": 150,
  "offset": 0,
  "limit": 20,
  "has_more": true,
  "next_offset": 20
}
```

**Pagination Fields:**
- `total` (integer) - Total number of history entries
- `offset` (integer) - Current offset
- `limit` (integer) - Current page size
- `has_more` (boolean) - Whether more entries exist
- `next_offset` (integer|null) - Offset for next page (null if no more)

**Status Codes:**
- `200 OK` - Success

**Examples:**
```bash
# First page (most recent 20 entries)
curl http://localhost:7620/api/sync/history?offset=0&limit=20

# Next page
curl http://localhost:7620/api/sync/history?offset=20&limit=20

# Custom page size
curl http://localhost:7620/api/sync/history?offset=0&limit=50
```

**Pagination Example:**
```bash
#!/bin/bash
# Fetch all history entries
offset=0
limit=20

while true; do
  response=$(curl -s "http://localhost:7620/api/sync/history?offset=$offset&limit=$limit")
  has_more=$(echo "$response" | jq -r '.has_more')

  # Process entries
  echo "$response" | jq '.history[]'

  if [ "$has_more" != "true" ]; then
    break
  fi

  next_offset=$(echo "$response" | jq -r '.next_offset')
  offset=$next_offset
done
```

---

### GET /api/devices

Get list of discovered devices with optional filtering.

**Query Parameters:**
- `edition` (string, optional) - Filter by device edition: "Android Full", "Android Free", "iOS"
- `syncing` (boolean, optional) - Filter by sync status: "true" or "false"
- `last_seen_after` (string, optional) - Filter by last seen timestamp (RFC3339 format)

**Response:**
```json
{
  "devices": [
    {
      "id": "SM-T970",
      "ip": "192.168.0.100",
      "name": "Samsung Galaxy Tab",
      "model": "SM-T970",
      "manufacturer": "Samsung",
      "edition": "Android Full",
      "last_seen": "2025-01-15T14:30:00Z",
      "is_syncing": false
    }
  ],
  "count": 1
}
```

**Fields:**
- `devices` (array) - List of devices
  - `id` (string) - Unique device identifier
  - `ip` (string) - Device IP address
  - `name` (string) - Friendly device name
  - `model` (string) - Device model
  - `manufacturer` (string) - Device manufacturer
  - `edition` (string) - ComicRack edition: "Android Full", "Android Free", "iOS"
  - `last_seen` (string) - Last discovery time (RFC3339)
  - `is_syncing` (boolean) - Whether device is currently syncing
- `count` (integer) - Number of devices returned

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid filter parameters

**Examples:**
```bash
# All devices
curl http://localhost:7620/api/devices

# Filter by edition (note: use URL encoding for spaces)
curl http://localhost:7620/api/devices?edition=Android+Full
curl http://localhost:7620/api/devices?edition=Android+Free
curl http://localhost:7620/api/devices?edition=iOS

# Filter by sync status
curl http://localhost:7620/api/devices?syncing=true
curl http://localhost:7620/api/devices?syncing=false

# Filter by last seen (RFC3339 format)
curl "http://localhost:7620/api/devices?last_seen_after=2025-01-15T00:00:00Z"

# Combine filters (AND logic)
curl "http://localhost:7620/api/devices?edition=iOS&syncing=false&last_seen_after=2025-01-01T00:00:00Z"

# Using jq for pretty output
curl http://localhost:7620/api/devices | jq '.devices[] | {name, ip, edition, is_syncing}'
```

---

### GET /api/stats

Get server statistics and configuration.

**Response:**
```json
{
  "uptime": "2h34m12s",
  "active_syncs": 1,
  "registered_devices": 5,
  "max_concurrent_connections": 10,
  "rate_limiting_enabled": true,
  "max_connections_per_ip": 3,
  "max_requests_per_device": 100
}
```

**Fields:**
- `uptime` (string) - Server uptime
- `active_syncs` (integer) - Number of currently active syncs
- `registered_devices` (integer) - Total number of discovered devices
- `max_concurrent_connections` (integer) - Maximum concurrent connections allowed
- `rate_limiting_enabled` (boolean) - Whether rate limiting is enabled
- `max_connections_per_ip` (integer) - Maximum connections per IP (only if rate limiting enabled)
- `max_requests_per_device` (integer) - Maximum requests per device (only if rate limiting enabled)

**Status Codes:**
- `200 OK` - Success

**Example:**
```bash
curl http://localhost:7620/api/stats
```

---

### GET /metrics

Prometheus metrics endpoint.

**Response Format:** Prometheus exposition format (text/plain)

**Available Metrics:**

#### comic_server_syncs_total
Counter - Total number of sync operations by status.

**Labels:**
- `status` - Sync status: "starting", "completed", "failed", "aborted"

**Example:**
```
comic_server_syncs_total{status="completed"} 42
comic_server_syncs_total{status="failed"} 2
```

#### comic_server_active_syncs
Gauge - Current number of active sync operations.

**Example:**
```
comic_server_active_syncs 1
```

#### comic_server_books_processed_total
Counter - Total number of books processed by operation type.

**Labels:**
- `operation` - Operation type: "added", "updated", "deleted"

**Example:**
```
comic_server_books_processed_total{operation="added"} 1500
comic_server_books_processed_total{operation="updated"} 800
comic_server_books_processed_total{operation="deleted"} 200
```

#### comic_server_sync_duration_seconds
Histogram - Duration of sync operations in seconds.

**Buckets:** 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10

**Example:**
```
comic_server_sync_duration_seconds_bucket{le="0.005"} 0
comic_server_sync_duration_seconds_bucket{le="0.01"} 0
comic_server_sync_duration_seconds_bucket{le="10"} 42
comic_server_sync_duration_seconds_sum 315.6
comic_server_sync_duration_seconds_count 42
```

**Status Codes:**
- `200 OK` - Success

**Example:**
```bash
curl http://localhost:7620/metrics
```

**Prometheus Configuration:**
```yaml
scrape_configs:
  - job_name: 'comic-server'
    static_configs:
      - targets: ['localhost:7620']
    scrape_interval: 15s
```

## Error Responses

All endpoints return JSON error responses for failures:

```json
{
  "error": "error message here"
}
```

**Common Status Codes:**
- `400 Bad Request` - Invalid request parameters
- `404 Not Found` - Endpoint not found
- `405 Method Not Allowed` - Wrong HTTP method
- `500 Internal Server Error` - Server error

## Rate Limiting

The API itself is not rate limited. However, the server has global rate limiting for device connections:

- Maximum concurrent connections (default: 10)
- Maximum connections per IP (default: 3)
- Maximum requests per device (default: 100/minute)

These limits can be configured via command-line flags or configuration file.

## CORS

The API does not currently support CORS. For web-based monitoring tools, consider using a reverse proxy with CORS support.

## Monitoring Examples

### Bash Script Monitoring

```bash
#!/bin/bash
# Monitor active syncs

while true; do
  active=$(curl -s http://localhost:7620/api/sync/status | jq '.active_count')
  echo "$(date): $active active syncs"
  sleep 10
done
```

### Python Monitoring Script

```python
#!/usr/bin/env python3
import requests
import time

API_BASE = "http://localhost:7620/api"

def monitor_syncs():
    while True:
        response = requests.get(f"{API_BASE}/sync/status")
        data = response.json()

        print(f"Active syncs: {data['active_count']}")
        for sync in data['active_syncs']:
            print(f"  - {sync['device_name']}: {sync['progress']}%")

        time.sleep(10)

if __name__ == "__main__":
    monitor_syncs()
```

### Grafana Dashboard

Example Prometheus queries for Grafana:

**Active syncs over time:**
```promql
comic_server_active_syncs
```

**Sync success rate:**
```promql
rate(comic_server_syncs_total{status="completed"}[5m])
/
rate(comic_server_syncs_total[5m])
```

**Books processed per hour:**
```promql
rate(comic_server_books_processed_total[1h]) * 3600
```

**Average sync duration:**
```promql
rate(comic_server_sync_duration_seconds_sum[5m])
/
rate(comic_server_sync_duration_seconds_count[5m])
```

**95th percentile sync duration:**
```promql
histogram_quantile(0.95, rate(comic_server_sync_duration_seconds_bucket[5m]))
```

## Integration Examples

### Health Check with systemd

```ini
[Service]
ExecStart=/usr/local/bin/comic-server server --library /path/to/library.xml
ExecStartPost=/bin/sleep 2
ExecStartPost=/bin/curl -f http://localhost:7620/api/health || exit 1
```

### Monitoring with cron

```bash
# Add to crontab
*/5 * * * * curl -sf http://localhost:7620/api/health || systemctl restart comic-server
```

### Docker Health Check

```dockerfile
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:7620/api/health || exit 1
```

### Kubernetes Liveness Probe

```yaml
livenessProbe:
  httpGet:
    path: /api/health
    port: 7620
  initialDelaySeconds: 10
  periodSeconds: 30
```

## WebSocket Support

WebSocket support is planned for future versions to enable real-time sync progress updates.

## API Versioning

The API is currently unversioned. Breaking changes will be avoided, but if necessary, they will be announced in release notes.

Future versions may add API versioning (e.g., `/api/v1/health`).

## See Also

- [Installation Guide](INSTALLATION.md) - Installing and setting up the server
- [Configuration Reference](CONFIGURATION.md) - Server configuration options
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions
