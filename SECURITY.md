# Security

This document describes the security features and hardening measures implemented in comic-server.

## Security Hardening Phases

### Phase 1: Input Validation ✅ COMPLETED
- Device validation and authentication
- SHA1 hash verification for device info
- Device registry with tracked devices
- Configurable device ignore/filter lists

### Phase 2: Rate Limiting and Resource Management ✅ COMPLETED

Phase 2 implements comprehensive rate limiting to protect against denial-of-service attacks and resource exhaustion.

#### Phase 2a: Rate Limiting Infrastructure

**IP-based Rate Limiting** (`internal/ratelimit/ip_limiter.go`):
- Sliding window algorithm for tracking connection attempts per IP
- Prevents single IPs from overwhelming the server
- Configurable maximum attempts per time window
- Automatic cleanup of expired tracking data
- Thread-safe concurrent access

**Device-based Rate Limiting** (`internal/ratelimit/device_limiter.go`):
- Token bucket algorithm for smooth rate limiting
- Allows bursts up to capacity with steady refill rate
- Per-device request tracking
- Configurable refill rate and capacity
- Background cleanup of inactive devices

**Configuration** (`internal/config/config.go`):
- `MaxConcurrentConnections`: Maximum concurrent sync operations (default: 5)
- `MaxConnectionsPerIP`: Max connection attempts per IP per window (default: 10)
- `MaxRequestsPerDevice`: Max requests per device per window (default: 100)
- `RateLimitWindowSeconds`: Time window for rate limits (default: 60 seconds)

**Test Coverage**:
- 22 comprehensive tests covering:
  - Basic rate limiting behavior
  - Multi-IP/multi-device scenarios
  - Token refill mechanics
  - Burst behavior
  - Concurrent access
  - Cleanup mechanisms
  - Edge cases (zero limits, large windows)

#### Phase 2b: Server Integration

**Rate Limiter Integration** (`cmd/server.go`):

1. **IP Rate Limiting at Discovery**:
   - Applied at device discovery (earliest possible point)
   - Rejects connections before any processing
   - Logs rate limit violations with current attempt counts

2. **Connection Limiting via Semaphores**:
   - Replaced simple mutex with buffered channel semaphore
   - Configurable concurrent connection limit
   - Graceful rejection when limit reached
   - Backward compatible (nil check for disabled mode)

3. **Device Rate Limiting for Sync**:
   - Applied before sync operations
   - Token bucket allows bursts with smooth rate limiting
   - Detailed logging of available tokens

**CLI Flags**:
```bash
--max-concurrent int            Maximum concurrent connections (0 = unlimited, default: 5)
--max-connections-per-ip int    Max connection attempts per IP per window (0 = unlimited, default: 10)
--max-requests-per-device int   Max requests per device per window (0 = unlimited, default: 100)
--rate-limit-window int         Rate limit window in seconds (default: 60)
```

**Configuration File** (YAML/TOML):
```yaml
server:
  max_concurrent_connections: 5
  max_connections_per_ip: 10
  max_requests_per_device: 100
  rate_limit_window_seconds: 60
```

**Priority**: CLI flags > Config file > Default values

**Resource Cleanup**:
- Rate limiters properly stopped with deferred `Stop()` calls
- Background goroutines cleaned up on shutdown
- No resource leaks

### Phase 3: Advanced Security (Planned)
- TLS/SSL encryption for device communication
- Certificate-based device authentication
- Enhanced logging and audit trails
- Intrusion detection capabilities

## Security Best Practices

### Deployment Recommendations

1. **Network Security**:
   - Use firewall rules to restrict access to ports 7614-7620
   - Consider running behind a reverse proxy for additional protection
   - Use separate network segments for device communication

2. **Rate Limiting**:
   - Adjust rate limits based on your deployment size and usage patterns
   - Monitor rate limit violations in logs
   - Use stricter limits in hostile network environments

3. **Device Management**:
   - Use device ignore lists to block unauthorized devices
   - Regularly review registered devices
   - Remove inactive devices from registry

4. **Monitoring**:
   - Monitor connection attempts and rate limit violations
   - Set up alerts for suspicious patterns
   - Review logs regularly for security events

### Configuration Examples

**Home Network** (lenient):
```yaml
server:
  max_concurrent_connections: 10
  max_connections_per_ip: 20
  max_requests_per_device: 200
  rate_limit_window_seconds: 60
```

**Public/Hostile Network** (strict):
```yaml
server:
  max_concurrent_connections: 3
  max_connections_per_ip: 5
  max_requests_per_device: 50
  rate_limit_window_seconds: 60
```

**Development** (unlimited):
```yaml
server:
  max_concurrent_connections: 0
  max_connections_per_ip: 0
  max_requests_per_device: 0
```

## Reporting Security Issues

If you discover a security vulnerability, please email security@[domain] or create a private security advisory on GitHub.

**Please do not**:
- Create public GitHub issues for security vulnerabilities
- Discuss security issues in public forums
- Attempt to exploit vulnerabilities in production systems

## Security Roadmap

- [x] Phase 1: Input Validation (v0.1)
- [x] Phase 2a: Rate Limiting Infrastructure (v0.2)
- [x] Phase 2b: Server Integration (v0.2)
- [ ] Phase 3: TLS/SSL Encryption (v0.3)
- [ ] Phase 4: Certificate Authentication (v0.4)
- [ ] Phase 5: Advanced Monitoring (v0.5)
