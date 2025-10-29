# Comic Server Monitoring with Prometheus and Grafana

This directory contains example configuration files for monitoring comic-server using Prometheus and Grafana.

## Prerequisites

- [Prometheus](https://prometheus.io/download/)
- [Grafana](https://grafana.com/grafana/download)
- comic-server running with metrics endpoint enabled (default port 7620)

## Quick Start

### 1. Start Prometheus

```bash
# Update prometheus.yml with your comic-server address
# Default: localhost:7620

prometheus --config.file=examples/monitoring/prometheus.yml
```

Prometheus UI will be available at: http://localhost:9090

### 2. Start Grafana

```bash
# Start Grafana (default port: 3000)
grafana-server
```

Grafana UI will be available at: http://localhost:3000 (default credentials: admin/admin)

### 3. Configure Grafana

1. Add Prometheus as a data source:
   - Go to Configuration → Data Sources
   - Click "Add data source"
   - Select "Prometheus"
   - Set URL to `http://localhost:9090`
   - Click "Save & Test"

2. Import the dashboard:
   - Go to Dashboards → Import
   - Upload `grafana-dashboard.json`
   - Select your Prometheus data source
   - Click "Import"

## Available Metrics

### Sync Metrics

- **comic_server_syncs_total** (Counter)
  - Labels: `status` (starting, completed, failed, aborted)
  - Tracks total number of sync operations by status

- **comic_server_active_syncs** (Gauge)
  - Current number of active sync operations
  - Useful for monitoring concurrent sync load

- **comic_server_sync_duration_seconds** (Histogram)
  - Distribution of sync operation durations
  - Buckets: 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, +Inf
  - Calculate percentiles: `histogram_quantile(0.95, rate(comic_server_sync_duration_seconds_bucket[5m]))`

### Book Processing Metrics

- **comic_server_books_processed_total** (Counter)
  - Labels: `operation` (added, updated, deleted)
  - Tracks total books processed by operation type

### Go Runtime Metrics

Standard Go metrics are automatically included:

- `go_goroutines` - Number of goroutines
- `go_memstats_*` - Memory statistics
- `go_gc_*` - Garbage collection metrics
- `process_*` - Process statistics (CPU, memory, file descriptors)

## Dashboard Panels

The included Grafana dashboard provides:

1. **Active Syncs Gauge** - Real-time view of concurrent syncs
2. **Sync Rate by Status** - Sync operations per second, color-coded by status
3. **Sync Duration (p50, p95)** - Performance percentiles over time
4. **Books Processed Rate** - Books per second by operation (added/updated/deleted)

## Example Queries

### Prometheus Queries

```promql
# Total syncs in the last hour
increase(comic_server_syncs_total[1h])

# Success rate (completed / total syncs)
rate(comic_server_syncs_total{status="completed"}[5m]) /
rate(comic_server_syncs_total[5m])

# Average sync duration
rate(comic_server_sync_duration_seconds_sum[5m]) /
rate(comic_server_sync_duration_seconds_count[5m])

# Books processed per sync
rate(comic_server_books_processed_total[5m]) /
rate(comic_server_syncs_total{status="completed"}[5m])

# Failed syncs in the last 15 minutes
increase(comic_server_syncs_total{status="failed"}[15m])
```

## Docker Compose Example

For a complete monitoring stack with Docker:

```yaml
version: '3'
services:
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-storage:/var/lib/grafana

  comic-server:
    image: comic-server:latest
    ports:
      - "7614:7614"  # Device communication
      - "7615:7615/udp"  # Discovery
      - "7620:7620"  # REST API & metrics
    volumes:
      - ./ComicDB.xml:/data/ComicDB.xml

volumes:
  grafana-storage:
```

## Alerting (Optional)

Create `alerts.yml` for Prometheus alerting:

```yaml
groups:
  - name: comic_server
    interval: 30s
    rules:
      - alert: HighFailureRate
        expr: rate(comic_server_syncs_total{status="failed"}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High sync failure rate detected"
          description: "More than 10% of syncs are failing ({{ $value }} per second)"

      - alert: LongSyncDuration
        expr: histogram_quantile(0.95, rate(comic_server_sync_duration_seconds_bucket[5m])) > 300
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Sync operations taking too long"
          description: "95th percentile sync duration is {{ $value }}s"

      - alert: NoActiveSyncs
        expr: sum(increase(comic_server_syncs_total[1h])) == 0
        for: 2h
        labels:
          severity: info
        annotations:
          summary: "No sync activity detected"
          description: "No syncs have occurred in the last hour"
```

Uncomment the `rule_files` section in `prometheus.yml` to enable alerting.

## Troubleshooting

**Prometheus can't scrape metrics:**
- Verify comic-server is running and `/metrics` endpoint is accessible
- Check firewall rules for port 7620
- Verify the `targets` address in `prometheus.yml`

**Grafana dashboard shows "No data":**
- Verify Prometheus data source is configured and working
- Check that Prometheus is successfully scraping comic-server
- Ensure comic-server has had some sync activity to generate metrics

**Metrics seem stale:**
- Check the `scrape_interval` in `prometheus.yml` (default: 10s)
- Verify the time range in Grafana dashboard (default: last 1 hour)
- Ensure Prometheus is running and not experiencing errors

## Additional Resources

- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [PromQL Basics](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [Grafana Dashboard Best Practices](https://grafana.com/docs/grafana/latest/best-practices/)
