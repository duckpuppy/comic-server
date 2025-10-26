# Security Documentation

This document describes the security measures implemented in comic-server to protect against common attack vectors.

## Security Model

comic-server implements a defense-in-depth approach with multiple security layers:

1. **Device Authentication**: Hash-based device validation
2. **Input Validation**: Protocol message size limits
3. **Trusted Data Sources**: File paths from server configuration only

## Phase 1: Core Security Controls (Completed)

### Device Authentication

All devices must authenticate before syncing by providing a valid device hash.

**Implementation**: `internal/device/info.go`

**Mechanism**: SHA1 hash verification
- Hash = `SHA1(Model + Manufacturer + Serial + Edition + Version)`
- Computed hash must match device-provided hash
- Enforced during device registration (`cmd/server.go:303`)

**Protection Against**:
- Unauthorized devices
- Device spoofing attacks

**Code Reference**:
```go
// info.Validate() performs full device validation including hash check
if err := info.Validate(); err != nil {
    logger.Error().Err(err).Msg("Device validation failed")
    return
}
```

### Protocol Input Validation

All protocol messages have enforced size limits to prevent memory exhaustion attacks.

**Implementation**: `internal/protocol/protocol.go`

**Limits**:
- `MaxStringLength = 1MB` - For filenames, paths, and metadata
- `MaxDataLength = 100MB` - For comic file transfers

**Protection Against**:
- Memory exhaustion DoS attacks
- Buffer overflow attempts
- Malicious clients sending extreme length values (e.g., 1GB, 1TB)

**Validation Logic**:
```go
// ReadString validates length before allocation
if length < 0 {
    return "", fmt.Errorf("invalid string length: %d (negative)", length)
}
if length > MaxStringLength {
    return "", fmt.Errorf("invalid string length: %d (exceeds maximum of %d bytes)",
        length, MaxStringLength)
}

// ReadData validates length before allocation
if length < 0 {
    return nil, fmt.Errorf("invalid data length: %d (negative)", length)
}
if length > MaxDataLength {
    return nil, fmt.Errorf("invalid data length: %d (exceeds maximum of %d bytes)",
        length, MaxDataLength)
}
```

**Testing**: Comprehensive security tests in `internal/protocol/security_test.go`:
- Tests valid edge cases (max length values)
- Tests invalid cases (exceeds max, negative lengths)
- Tests attack scenarios (1GB, 1TB length values)

### File Path Security

File paths are only read from trusted sources, preventing directory traversal attacks.

**Implementation**: `internal/sync/session.go`, `internal/library/library.go`

**Trust Model**:
- File paths originate from `ComicDB.xml` (server's own library file)
- Paths are NOT accepted from network clients
- Server reads its own library configuration and sends those files to devices

**Protection Against**:
- Directory traversal attacks (e.g., `../../etc/passwd`)
- Unauthorized file access
- Path injection attacks

**Data Flow**:
1. Server loads library from `ComicDB.xml` (trusted source)
2. Library contains `book.FilePath` for each comic
3. Sync operation reads from these trusted paths: `os.ReadFile(book.FilePath)`
4. Clients receive files but cannot specify arbitrary paths

## Security Testing

### Automated Tests

All security controls have comprehensive test coverage:

**Protocol Input Validation** (`internal/protocol/security_test.go`):
- `TestReadStringMaxLengthValidation` - String length limits
- `TestReadDataMaxLengthValidation` - Data length limits
- `TestReadStringNegativeLengthRejection` - Negative length rejection
- `TestReadDataNegativeLengthRejection` - Negative length rejection
- `TestSecurityLimitsConstants` - Constant value validation

**Device Authentication** (`internal/device/info_test.go`):
- `TestValidateHash` - Hash validation logic
- `TestComputeHash` - Hash computation consistency
- Integration tests for full device validation workflow

### Running Security Tests

```bash
# Run all security tests
just test

# Run specific security tests
go test -v ./internal/protocol -run Security
go test -v ./internal/device -run Validate
```

## Planned Enhancements (Phase 2 & 3)

### Phase 2: Rate Limiting and Resource Management
- Connection rate limiting per IP
- Maximum concurrent connections
- Request rate limiting per device
- Resource usage monitoring

### Phase 3: Advanced Security
- Security audit logging
- Penetration testing recommendations
- TLS/encryption for device communication (future consideration)
- Security update process documentation

## Reporting Security Issues

If you discover a security vulnerability, please:

1. **DO NOT** open a public issue
2. Report privately to the maintainers
3. Include:
   - Vulnerability description
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if available)

## Security Best Practices for Operators

### Network Security
- Run comic-server on trusted networks only
- Use firewall rules to restrict access to ports 7614, 7615, 7620
- Consider network segmentation for device sync traffic

### System Security
- Run comic-server as a non-root user
- Use restrictive file permissions on library files
- Keep system and dependencies updated
- Monitor logs for suspicious activity

### Configuration Security
- Use `--ignore-device` to block untrusted devices
- Verify device registrations in logs
- Review sync operations regularly
- Backup library files before major syncs

## Security Limitations

### Current Limitations
- No TLS/encryption for device communication (uses ComicRack's unencrypted protocol)
- No rate limiting (planned for Phase 2)
- No connection limits (planned for Phase 2)
- Limited to SHA1 hashes (protocol limitation from ComicRack)

### Accepted Risks
- **Unencrypted Protocol**: The ComicRack wireless sync protocol does not use encryption. This is acceptable for trusted home networks but should not be used over untrusted networks.
- **SHA1 Hashing**: While SHA1 is cryptographically weak, it's sufficient for device authentication in this context (not used for cryptographic purposes).

## References

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [CWE-400: Resource Exhaustion](https://cwe.mitre.org/data/definitions/400.html)
- [CWE-22: Path Traversal](https://cwe.mitre.org/data/definitions/22.html)
- Issue #10: Security hardening tracking issue
