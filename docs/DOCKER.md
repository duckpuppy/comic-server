# Docker Deployment Guide

This guide covers running comic-server in Docker containers.

## Quick Start

Pull and run the latest image:

```bash
docker pull ghcr.io/duckpuppy/comic-server:latest

docker run -d \
  --name comic-server \
  --network host \
  -v ~/.local/share/ComicRack/ComicDb.xml:/data/ComicDb.xml:ro \
  -v ~/Comics:/comics:ro \
  -e COMIC_SERVER_LOG_LEVEL=info \
  ghcr.io/duckpuppy/comic-server:latest
```

## Image Details

### Available Images

Images are hosted on GitHub Container Registry (ghcr.io):

**Repository:** `ghcr.io/duckpuppy/comic-server`

**Available tags:**
- `latest` - Latest build from master/main branch
- `vX.Y.Z` - Specific release versions (e.g., `v0.6.0`)
- `X.Y` - Major.minor version (e.g., `0.6`)
- `X` - Major version (e.g., `0`)
- `master-<sha>` - Specific commit from master branch

**Platforms:**
- `linux/amd64` - x86_64 architecture
- `linux/arm64` - ARM64 architecture (Raspberry Pi 4+, Apple Silicon, etc.)

### Image Size

The image is optimized for minimal size using multi-stage builds:

- **Base image:** Alpine Linux (~5 MB)
- **Binary size:** ~15-20 MB (stripped)
- **Total image size:** ~25-30 MB

## Using docker run

### Basic Usage

```bash
docker run -d \
  --name comic-server \
  --network host \
  -v /path/to/ComicDb.xml:/data/ComicDb.xml:ro \
  -v /path/to/comics:/comics:ro \
  ghcr.io/duckpuppy/comic-server:latest
```

**Important:** `--network host` is required for multicast device discovery to work properly.

### With Environment Variables

```bash
docker run -d \
  --name comic-server \
  --network host \
  -v ~/.local/share/ComicRack/ComicDb.xml:/data/ComicDb.xml:ro \
  -v ~/Comics:/comics:ro \
  -e COMIC_SERVER_LOG_LEVEL=debug \
  -e COMIC_SERVER_LOG_FORMAT=json \
  -e COMIC_SERVER_AUTO_SYNC=true \
  -e COMIC_SERVER_IGNORE_DEVICES=192.168.0.24,SM-T970 \
  ghcr.io/duckpuppy/comic-server:latest
```

### With Configuration File

```bash
# Create config directory
mkdir -p ~/comic-server/config

# Copy your config file
cp config.yaml ~/comic-server/config/

# Run with config volume
docker run -d \
  --name comic-server \
  --network host \
  -v ~/comic-server/config:/config \
  -v ~/.local/share/ComicRack/ComicDb.xml:/data/ComicDb.xml:ro \
  -v ~/Comics:/comics:ro \
  ghcr.io/duckpuppy/comic-server:latest
```

### Custom Command

Override the default command:

```bash
# Run discovery only
docker run --rm --network host \
  ghcr.io/duckpuppy/comic-server:latest \
  discover

# Show version
docker run --rm \
  ghcr.io/duckpuppy/comic-server:latest \
  version

# Run with custom library path
docker run -d \
  --name comic-server \
  --network host \
  -v /custom/path/library.xml:/data/library.xml:ro \
  -v ~/Comics:/comics:ro \
  ghcr.io/duckpuppy/comic-server:latest \
  server --library /data/library.xml
```

## Using docker-compose

### Basic compose.yml

Create a `docker-compose.yml` file:

```yaml
version: '3.8'

services:
  comic-server:
    image: ghcr.io/duckpuppy/comic-server:latest
    container_name: comic-server
    network_mode: host
    restart: unless-stopped

    environment:
      - COMIC_SERVER_LOG_LEVEL=info
      - COMIC_SERVER_LOG_FORMAT=json
      - COMIC_SERVER_AUTO_SYNC=true

    volumes:
      # Mount library file (read-only)
      - ~/.local/share/ComicRack/ComicDb.xml:/data/ComicDb.xml:ro
      # Mount comics directory (read-only)
      - ~/Comics:/comics:ro
      # Optional: config directory
      - ./config:/config

    healthcheck:
      test: ["CMD", "/usr/local/bin/comic-server", "version"]
      interval: 30s
      timeout: 3s
      retries: 3
```

### Using Environment File

Create `.env` file:

```bash
# Library and comics paths
LIBRARY_PATH=/home/user/.local/share/ComicRack/ComicDb.xml
COMICS_PATH=/home/user/Comics

# Server settings
COMIC_SERVER_LOG_LEVEL=info
COMIC_SERVER_LOG_FORMAT=json
COMIC_SERVER_AUTO_SYNC=true

# Optional: ignore devices
COMIC_SERVER_IGNORE_DEVICES=192.168.0.24,Production Tablet
```

Update `docker-compose.yml`:

```yaml
version: '3.8'

services:
  comic-server:
    image: ghcr.io/duckpuppy/comic-server:latest
    container_name: comic-server
    network_mode: host
    restart: unless-stopped

    env_file:
      - .env

    volumes:
      - ${LIBRARY_PATH}:/data/ComicDb.xml:ro
      - ${COMICS_PATH}:/comics:ro
      - ./config:/config
```

### Running with docker-compose

```bash
# Start service
docker-compose up -d

# View logs
docker-compose logs -f

# Stop service
docker-compose down

# Restart service
docker-compose restart

# Pull latest image and restart
docker-compose pull && docker-compose up -d
```

### With Monitoring Stack

Example with Prometheus and Grafana:

```yaml
version: '3.8'

services:
  comic-server:
    image: ghcr.io/duckpuppy/comic-server:latest
    container_name: comic-server
    network_mode: host
    restart: unless-stopped
    environment:
      - COMIC_SERVER_LOG_LEVEL=info
      - COMIC_SERVER_LOG_FORMAT=json
    volumes:
      - ~/.local/share/ComicRack/ComicDb.xml:/data/ComicDb.xml:ro
      - ~/Comics:/comics:ro

  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    restart: unless-stopped
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'

  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-data:/var/lib/grafana

volumes:
  prometheus-data:
  grafana-data:
```

Create `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'comic-server'
    static_configs:
      - targets: ['host.docker.internal:7620']
```

## Building Custom Images

### Build Locally

```bash
# Build for current platform
docker build -t comic-server:local .

# Build with version info
docker build \
  --build-arg VERSION=0.6.0 \
  --build-arg GIT_COMMIT=$(git rev-parse HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t comic-server:0.6.0 \
  .

# Build for multiple platforms
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t comic-server:multi \
  .
```

### Using Buildx for Multi-Platform

```bash
# Create buildx builder
docker buildx create --name comic-builder --use

# Build and push
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=0.6.0 \
  --build-arg GIT_COMMIT=$(git rev-parse HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t ghcr.io/yourusername/comic-server:0.6.0 \
  --push \
  .
```

### Custom Dockerfile

You can customize the Dockerfile for specific needs:

```dockerfile
# Example: Using Ubuntu instead of Alpine
FROM golang:1.25 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o comic-server .

FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /build/comic-server /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/comic-server"]
CMD ["server", "--library", "/data/ComicDb.xml"]
```

## Environment Variables

All comic-server environment variables are supported:

| Variable | Default | Description |
|----------|---------|-------------|
| `COMIC_SERVER_LIBRARY_PATH` | - | Path to ComicDb.xml (not used if --library flag provided) |
| `COMIC_SERVER_LOG_LEVEL` | `info` | Log level: debug, info, warn, error |
| `COMIC_SERVER_LOG_FORMAT` | `json` | Log format: text, json |
| `COMIC_SERVER_AUTO_SYNC` | `false` | Auto-sync on device discovery |
| `COMIC_SERVER_DISCOVERY_PORT` | `7615` | UDP discovery port |
| `COMIC_SERVER_COMMAND_PORT` | `7614` | TCP command port |
| `COMIC_SERVER_API_PORT` | `7620` | REST API port |
| `COMIC_SERVER_IGNORE_DEVICES` | - | Comma-separated list of devices to ignore |
| `COMIC_SERVER_MAX_CONCURRENT` | `10` | Max concurrent connections |
| `COMIC_SERVER_MAX_PER_IP` | `3` | Max connections per IP |
| `COMIC_SERVER_MAX_PER_DEVICE` | `100` | Max requests per device |

## Volume Mounts

The container expects these volumes:

| Mount Point | Purpose | Access | Required |
|-------------|---------|--------|----------|
| `/data/ComicDb.xml` | ComicRack library file | Read-only | Yes |
| `/comics` | Comics directory | Read-only | Yes |
| `/config` | Configuration directory | Read-write | Optional |

## Networking

### Host Network Mode (Recommended)

```bash
docker run --network host ...
```

**Pros:**
- Multicast discovery works out of the box
- No port mapping needed
- Best performance

**Cons:**
- Container shares host's network stack
- Less isolation

### Bridge Network Mode

```bash
docker run -p 7614:7614 -p 7615:7615/udp -p 7620:7620 ...
```

**Note:** Multicast discovery may not work in bridge mode. You'll need to manually configure device connections or use network=host.

### Custom Network

```bash
# Create custom network
docker network create comic-net

# Run container
docker run --network comic-net ...
```

## Health Checks

The image includes a health check that runs `comic-server version`:

```bash
# Check container health
docker ps
docker inspect --format='{{.State.Health.Status}}' comic-server

# View health check logs
docker inspect --format='{{range .State.Health.Log}}{{.Output}}{{end}}' comic-server
```

Customize health check in docker-compose:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:7620/api/health"]
  interval: 30s
  timeout: 3s
  retries: 3
  start_period: 5s
```

## Resource Limits

Set resource limits to prevent container from consuming too much:

**docker run:**
```bash
docker run \
  --cpus="1.0" \
  --memory="512m" \
  --memory-swap="512m" \
  ...
```

**docker-compose:**
```yaml
services:
  comic-server:
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 128M
```

## Logging

### View Logs

```bash
# Follow logs
docker logs -f comic-server

# Last 100 lines
docker logs --tail 100 comic-server

# With timestamps
docker logs -t comic-server

# Since specific time
docker logs --since 2024-01-01T00:00:00 comic-server
```

### Configure Log Driver

**docker run:**
```bash
docker run \
  --log-driver json-file \
  --log-opt max-size=10m \
  --log-opt max-file=3 \
  ...
```

**docker-compose:**
```yaml
services:
  comic-server:
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

### Send Logs to External System

**Syslog:**
```yaml
logging:
  driver: syslog
  options:
    syslog-address: "tcp://192.168.1.100:514"
    tag: "comic-server"
```

**Fluentd:**
```yaml
logging:
  driver: fluentd
  options:
    fluentd-address: localhost:24224
    tag: comic-server
```

## Security

### Run as Non-Root User

The image already runs as non-root user (UID 1000). Verify:

```bash
docker exec comic-server whoami
# Output: comic
```

### Read-Only Root Filesystem

```bash
docker run --read-only --tmpfs /tmp ...
```

### Security Options

```bash
docker run \
  --security-opt="no-new-privileges:true" \
  --cap-drop=ALL \
  ...
```

### Scan Image for Vulnerabilities

```bash
# Using Docker Scout
docker scout cves ghcr.io/duckpuppy/comic-server:latest

# Using Trivy
trivy image ghcr.io/duckpuppy/comic-server:latest
```

## Troubleshooting

### Container Won't Start

Check logs:
```bash
docker logs comic-server
```

Common issues:
- Library file not accessible: Check volume mount
- Permissions: Ensure files are readable by UID 1000

### Devices Not Discovered

1. **Check network mode:**
   ```bash
   docker inspect comic-server | grep NetworkMode
   ```
   Should be `host`.

2. **Test from container:**
   ```bash
   docker exec -it comic-server sh
   # Inside container
   /usr/local/bin/comic-server discover
   ```

3. **Check firewall:**
   ```bash
   # Allow UDP 7615 on host
   sudo ufw allow 7615/udp
   ```

### High Memory Usage

Set memory limits:
```bash
docker update --memory 512m comic-server
```

Monitor usage:
```bash
docker stats comic-server
```

### Container Keeps Restarting

Check restart policy:
```bash
docker inspect comic-server | grep RestartPolicy
```

Remove restart policy temporarily:
```bash
docker update --restart=no comic-server
```

## Maintenance

### Update Image

```bash
# Pull latest
docker pull ghcr.io/duckpuppy/comic-server:latest

# Stop and remove old container
docker stop comic-server
docker rm comic-server

# Start with new image
docker run -d --name comic-server ...
```

### With docker-compose

```bash
docker-compose pull
docker-compose up -d
```

### Backup Configuration

```bash
# Backup config volume
docker run --rm \
  -v comic-server-config:/config \
  -v $(pwd):/backup \
  alpine tar czf /backup/config-backup.tar.gz -C /config .

# Restore
docker run --rm \
  -v comic-server-config:/config \
  -v $(pwd):/backup \
  alpine tar xzf /backup/config-backup.tar.gz -C /config
```

### Prune Old Images

```bash
# Remove unused images
docker image prune

# Remove specific old tags
docker rmi ghcr.io/duckpuppy/comic-server:old-tag
```

## GitHub Actions Integration

Images are automatically built and published via GitHub Actions. See `.github/workflows/docker-publish.yml` for details.

**Triggers:**
- Push to master/main: Builds and tags as `latest`
- New tag `vX.Y.Z`: Builds and tags with version
- Pull requests: Builds but doesn't push

**Features:**
- Multi-platform builds (amd64, arm64)
- Automatic pruning of old dev images
- Keeps tagged releases indefinitely
- Stays under GitHub's 1GB free tier limit

## See Also

- [Installation Guide](INSTALLATION.md) - Other installation methods
- [Configuration Reference](CONFIGURATION.md) - Configuration options
- [API Reference](API.md) - Monitoring and management
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues
