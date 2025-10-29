# Multi-stage build for minimal image size
# Stage 1: Builder
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
# - CGO_ENABLED=0: Static binary (no C dependencies)
# - -ldflags: Strip debug info and inject version info
# - -trimpath: Remove file system paths from binary
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s \
    -X github.com/duckpuppy/comic-server/cmd.Version=${VERSION} \
    -X github.com/duckpuppy/comic-server/cmd.GitCommit=${GIT_COMMIT} \
    -X github.com/duckpuppy/comic-server/cmd.BuildDate=${BUILD_DATE}" \
    -trimpath \
    -o comic-server \
    .

# Stage 2: Runtime
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && addgroup -g 1000 comic \
    && adduser -D -u 1000 -G comic comic

# Copy binary from builder
COPY --from=builder /build/comic-server /usr/local/bin/comic-server

# Create directories for config and data
RUN mkdir -p /config /data /comics && \
    chown -R comic:comic /config /data /comics

# Switch to non-root user
USER comic

# Expose ports
# 7614: TCP device communication
# 7615: UDP multicast discovery
# 7620: REST API and metrics
EXPOSE 7614/tcp 7615/udp 7620/tcp

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/comic-server", "version"]

# Set default environment variables
ENV COMIC_SERVER_LOG_LEVEL=info \
    COMIC_SERVER_LOG_FORMAT=json

# Volume for config and data
VOLUME ["/config", "/data", "/comics"]

ENTRYPOINT ["/usr/local/bin/comic-server"]
CMD ["server", "--library", "/data/ComicDb.xml"]
