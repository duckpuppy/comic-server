# Troubleshooting Guide

This guide covers common issues and their solutions when running comic-server.

## Quick Diagnostics

Before diving into specific issues, run these diagnostic checks:

```bash
# Check if server is running
curl http://localhost:7620/api/health

# Check server logs (systemd)
sudo journalctl -u comic-server -n 50

# Test device discovery
comic-server discover

# Enable debug logging
comic-server server --library /path/to/library.xml --log-level debug
```

## Common Issues

### Device Discovery Problems

#### Devices Not Being Discovered

**Symptoms:**
- `comic-server discover` shows no devices
- Devices don't appear in `/api/devices`

**Causes & Solutions:**

1. **Multicast not working**

   Check if multicast is enabled on your network interface:
   ```bash
   # Linux
   ip link show | grep MULTICAST

   # Enable multicast
   sudo ip link set dev eth0 multicast on
   ```

2. **Firewall blocking UDP port 7615**

   ```bash
   # Linux (ufw)
   sudo ufw allow 7615/udp

   # Linux (firewalld)
   sudo firewall-cmd --permanent --add-port=7615/udp
   sudo firewall-cmd --reload

   # Check if port is listening
   sudo ss -ulnp | grep 7615
   ```

3. **IGMP not enabled on router**

   - Log into your router's admin interface
   - Enable IGMP snooping
   - Ensure multicast traffic is allowed between VLANs

4. **WSL2 networking limitations**

   WSL2 has limited multicast support. Try:
   - Running on native Linux instead
   - Using WSL1 (no multicast in WSL2)
   - Running on Windows natively

5. **Device and server on different subnets**

   Multicast discovery (224.34.123.90) doesn't cross subnets by default. Solutions:
   - Put devices and server on same subnet
   - Configure multicast routing on your router
   - Use IGMP proxy

**Testing multicast:**
```bash
# Terminal 1 - Listen for multicast
socat UDP4-RECVFROM:7615,ip-add-membership=224.34.123.90:eth0,fork -

# Terminal 2 - Send test packet
echo "ComicRack:test-device" | socat - UDP4-DATAGRAM:224.34.123.90:7615,broadcast
```

#### Devices Discovered But Can't Connect

**Symptoms:**
- Devices appear in `/api/devices`
- Connection fails when attempting sync
- Error: "connection refused" or "timeout"

**Causes & Solutions:**

1. **Firewall blocking TCP port 7614**

   ```bash
   # Allow port 7614
   sudo ufw allow 7614/tcp

   # Verify server is listening
   sudo ss -tlnp | grep 7614
   ```

2. **Server not listening on correct interface**

   Check server logs for bind errors:
   ```bash
   sudo journalctl -u comic-server | grep "bind\|listen"
   ```

3. **Network connectivity issues**

   Test connectivity:
   ```bash
   # From server to device
   ping 192.168.0.100

   # From device to server
   # Use a network tool app on the device
   ```

### Sync Problems

#### Sync Fails Immediately

**Symptoms:**
- Sync starts but fails within seconds
- Error in logs about library or file access

**Causes & Solutions:**

1. **Library file not found or not readable**

   ```bash
   # Check file exists and is readable
   ls -l /path/to/ComicDb.xml

   # Fix permissions
   chmod 644 /path/to/ComicDb.xml

   # Check SELinux context (if applicable)
   ls -Z /path/to/ComicDb.xml
   ```

2. **Invalid library XML**

   ```bash
   # Validate XML structure
   xmllint --noout /path/to/ComicDb.xml

   # Check for encoding issues
   file /path/to/ComicDb.xml
   ```

3. **Comic files not accessible**

   The library XML contains paths to comic files. Ensure:
   - Comic files exist at the specified paths
   - Server has read access to comic directories
   - Paths are absolute, not relative

   ```bash
   # Find broken links in library
   grep -o 'File="[^"]*"' /path/to/ComicDb.xml | \
     sed 's/File="\(.*\)"/\1/' | \
     while read file; do
       [ ! -f "$file" ] && echo "Missing: $file"
     done
   ```

#### Sync Gets Stuck or Never Completes

**Symptoms:**
- Sync shows progress but never reaches 100%
- Progress stops at specific percentage
- Server becomes unresponsive

**Causes & Solutions:**

1. **Large files causing timeout**

   Increase timeout in configuration:
   ```yaml
   server:
     connection_timeout: 300  # 5 minutes
   ```

2. **Network instability**

   Check server logs for network errors:
   ```bash
   sudo journalctl -u comic-server | grep "network\|timeout\|disconnect"
   ```

   Check device WiFi signal strength and stability.

3. **Device storage full**

   Check device storage via device settings or app.

4. **Rate limiting**

   Check if rate limits are being hit:
   ```bash
   curl http://localhost:7620/api/stats
   ```

   Adjust limits:
   ```yaml
   server:
     max_requests_per_device: 200  # Increase limit
   ```

#### Sync Reports Errors

**Symptoms:**
- Sync completes but with error_count > 0
- Some books fail to sync

**Debugging:**

1. **Enable debug logging**

   ```bash
   comic-server server --library /path/to/library.xml --log-level debug
   ```

2. **Check sync history**

   ```bash
   curl http://localhost:7620/api/sync/history | jq '.history[] | select(.error_count > 0)'
   ```

3. **Common error causes:**
   - Corrupted comic files (check file integrity)
   - Invalid file formats (only CBZ, CBR, PDF supported)
   - Permissions issues
   - Disk space on device

### Device Disconnect Issues

#### Devices Disconnecting During Sync

**Symptoms:**
- Sync starts but device disconnects midway
- Error: "connection reset by peer" or "broken pipe"

**Causes & Solutions:**

1. **WiFi power saving**

   On device:
   - Disable battery optimization for ComicRack app
   - Keep screen on during sync
   - Disable aggressive power saving modes

2. **Router disconnecting idle connections**

   - Increase router's connection timeout
   - Enable keep-alive in server config (planned feature)

3. **Network instability**

   ```bash
   # Monitor connection quality
   ping -i 1 192.168.0.100

   # Check for packet loss
   mtr 192.168.0.100
   ```

#### Device Appears Multiple Times

**Symptoms:**
- Same device listed multiple times in `/api/devices`
- Different IP addresses or IDs for same physical device

**Causes:**
- Device is getting different IPs (DHCP)
- Device reconnects with different ID

**Solutions:**

1. **Assign static IP or DHCP reservation**

   In your router:
   - Find device's MAC address
   - Create DHCP reservation
   - Assign consistent IP

2. **Clean up device registry**

   Devices with old last_seen timestamps can be ignored:
   ```bash
   # Show devices not seen in 24 hours
   curl "http://localhost:7620/api/devices?last_seen_after=$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)"
   ```

### Configuration Issues

#### Configuration Not Loading

**Symptoms:**
- Server uses default values despite configuration file
- Changes to config file have no effect

**Debugging:**

1. **Check configuration file location**

   ```bash
   # Server looks in these locations (in order):
   ls -l ./config.yaml
   ls -l ~/.config/comic-server/config.yaml
   ls -l /etc/comic-server/config.yaml
   ```

2. **Validate YAML syntax**

   ```bash
   # Install yamllint
   pip install yamllint

   # Validate config
   yamllint ~/.config/comic-server/config.yaml
   ```

3. **Check precedence**

   Remember: CLI flags > Environment variables > Config file

   ```bash
   # Use debug logging to see loaded config
   comic-server server --library /path/to/library.xml --log-level debug
   ```

4. **Reload configuration**

   After changing config, send SIGHUP:
   ```bash
   kill -HUP $(pgrep comic-server)
   # or
   sudo systemctl reload comic-server
   ```

#### Ignored Devices Still Being Synced

**Symptoms:**
- Device in `ignore_devices` list still gets synced

**Causes & Solutions:**

1. **Identifier mismatch**

   Check what you're using to ignore:
   ```bash
   # Get actual device identifiers
   curl http://localhost:7620/api/devices | jq '.devices[] | {id, name, ip}'
   ```

   Ensure `ignore_devices` uses correct identifier:
   ```yaml
   server:
     ignore_devices:
       - "192.168.0.24"      # IP address
       - "SM-T970"           # Device ID
       - "Production Tablet" # Device name
   ```

2. **Configuration not reloaded**

   Send SIGHUP after config change:
   ```bash
   sudo systemctl reload comic-server
   ```

### Performance Issues

#### Server Using Too Much Memory

**Symptoms:**
- High memory usage
- System becomes slow
- OOM killer terminates server

**Causes & Solutions:**

1. **Large library file**

   The entire library is loaded into memory. For very large libraries:
   ```bash
   # Check library size
   ls -lh /path/to/ComicDb.xml

   # Check memory usage
   ps aux | grep comic-server
   ```

   Solutions:
   - Use 64-bit system with adequate RAM
   - Consider pruning unused entries from library

2. **Too many concurrent connections**

   ```bash
   # Check active connections
   sudo ss -tn | grep :7614 | wc -l
   ```

   Reduce limit:
   ```yaml
   server:
     max_concurrent_connections: 5
   ```

3. **Memory leak**

   If memory grows over time:
   - Check server version (may be fixed in newer version)
   - Monitor with: `watch -n 5 'ps aux | grep comic-server'`
   - Report issue on GitHub

#### Sync is Very Slow

**Symptoms:**
- Sync takes hours for small libraries
- Progress increases very slowly

**Causes & Solutions:**

1. **Network bandwidth**

   ```bash
   # Test bandwidth to device
   iperf3 -s  # On server
   # Run iperf3 client on device or another machine
   ```

2. **Server CPU usage**

   ```bash
   # Check CPU usage
   top -p $(pgrep comic-server)
   ```

   If high:
   - Reduce concurrent connections
   - Check for I/O wait (slow disk)

3. **Large comic files**

   ```bash
   # Find largest comics
   grep -o 'File="[^"]*"' /path/to/ComicDb.xml | \
     sed 's/File="\(.*\)"/\1/' | \
     xargs -I {} du -h {} | \
     sort -rh | head -20
   ```

   Consider:
   - Compressing very large files
   - Limiting sync to smaller subset

### API Issues

#### API Not Responding

**Symptoms:**
- `curl http://localhost:7620/api/health` fails
- API endpoints return errors

**Causes & Solutions:**

1. **Server not running**

   ```bash
   # Check process
   pgrep comic-server

   # Check systemd status
   sudo systemctl status comic-server
   ```

2. **Wrong port**

   ```bash
   # Check configured port
   sudo ss -tlnp | grep comic-server

   # If using custom port
   curl http://localhost:8620/api/health
   ```

3. **Firewall blocking API port**

   ```bash
   sudo ufw allow 7620/tcp
   ```

#### Metrics Not Available

**Symptoms:**
- `/metrics` endpoint returns 404
- Prometheus scraping fails

**Solution:**

Ensure you're using GET request:
```bash
curl http://localhost:7620/metrics
```

Check Prometheus logs for scrape errors:
```bash
# In Prometheus container/system
grep "comic-server" /var/log/prometheus.log
```

### Service/Daemon Issues

#### Service Won't Start

**Linux (systemd):**

```bash
# Check service status
sudo systemctl status comic-server

# Check for errors
sudo journalctl -u comic-server -n 50

# Common issues:
# - Binary path wrong in service file
# - Library path not accessible
# - Permissions issues
```

**macOS (launchd):**

```bash
# Check if loaded
launchctl list | grep comic-server

# Check logs
tail -f ~/Library/Logs/comic-server-error.log

# Common issues:
# - Incorrect plist XML
# - Binary not executable
# - Path issues in plist
```

**Windows (NSSM):**

```powershell
# Check service status
nssm status comic-server

# Check logs
Get-EventLog -LogName Application -Source comic-server -Newest 10
```

#### Service Starts But Stops Immediately

**Debugging:**

1. **Run manually to see errors**

   ```bash
   # Stop service
   sudo systemctl stop comic-server

   # Run manually
   /usr/local/bin/comic-server server --library /path/to/library.xml --log-level debug
   ```

2. **Check for library file access**

   ```bash
   # Service may run as different user
   sudo -u comic-server cat /path/to/library.xml
   ```

3. **Check working directory**

   Service may expect specific working directory:
   ```ini
   [Service]
   WorkingDirectory=/home/user
   ```

### Logging Issues

#### No Logs Appearing

**Causes & Solutions:**

1. **Log level too high**

   ```bash
   # Use debug level
   comic-server server --library /path/to/library.xml --log-level debug
   ```

2. **Logs going to wrong location**

   ```bash
   # systemd logs
   sudo journalctl -u comic-server -f

   # launchd logs
   tail -f ~/Library/Logs/comic-server.log

   # Docker logs
   docker logs comic-server -f
   ```

3. **Log rotation**

   Old logs may have been rotated. Check:
   ```bash
   # Linux
   ls -l /var/log/comic-server*
   zcat /var/log/comic-server.log.1.gz
   ```

#### Logs Too Verbose

**Solution:**

Reduce log level:
```bash
comic-server server --library /path/to/library.xml --log-level warn
```

Or in config:
```yaml
server:
  log_level: "warn"
```

## Getting Help

If you're still having issues:

1. **Check GitHub Issues**

   Search existing issues: https://github.com/duckpuppy/comic-server/issues

2. **Gather Debug Information**

   When reporting issues, include:

   ```bash
   # Version info
   comic-server version

   # System info
   uname -a
   go version

   # Debug logs (sanitize sensitive data)
   comic-server server --library /path/to/library.xml --log-level debug 2>&1 | tee debug.log

   # API status
   curl http://localhost:7620/api/health
   curl http://localhost:7620/api/stats
   curl http://localhost:7620/api/devices
   ```

3. **Create GitHub Issue**

   File a new issue with:
   - Clear description of the problem
   - Steps to reproduce
   - Debug information from above
   - Expected vs. actual behavior

4. **Community Support**

   - GitHub Discussions
   - ComicRack community forums

## Advanced Debugging

### Network Traffic Analysis

Capture and analyze protocol traffic:

```bash
# Capture UDP discovery packets
sudo tcpdump -i any -n udp port 7615 -A

# Capture TCP command traffic
sudo tcpdump -i any -n tcp port 7614 -w comic-server.pcap

# Analyze with Wireshark
wireshark comic-server.pcap
```

### Enable Go pprof Profiling

For performance debugging:

```go
// Add to main.go (development builds only)
import _ "net/http/pprof"

// Access profiles
go tool pprof http://localhost:7620/debug/pprof/heap
go tool pprof http://localhost:7620/debug/pprof/profile
```

### Strace System Calls

Debug file access and syscalls:

```bash
# Linux
sudo strace -f -e trace=open,read,write,connect -p $(pgrep comic-server)

# macOS
sudo dtruss -p $(pgrep comic-server)
```

### Database Corruption Recovery

If library XML is corrupted:

```bash
# Backup
cp ComicDb.xml ComicDb.xml.backup

# Try to fix XML
xmllint --recover ComicDb.xml > ComicDb.xml.fixed

# If unfixable, restore from ComicRack desktop app
# or rebuild library
```

## Prevention Best Practices

To avoid common issues:

1. **Regular backups**
   ```bash
   # Backup library
   cp ~/.local/share/ComicRack/ComicDb.xml ~/backups/ComicDb-$(date +%Y%m%d).xml
   ```

2. **Monitor server health**
   ```bash
   # Add to cron
   */5 * * * * curl -sf http://localhost:7620/api/health || systemctl restart comic-server
   ```

3. **Keep software updated**
   ```bash
   # Check for updates
   comic-server version
   # Compare with latest release on GitHub
   ```

4. **Use stable network configuration**
   - Static IPs or DHCP reservations
   - Quality WiFi equipment
   - Avoid VLANs unless properly configured

5. **Monitor logs regularly**
   ```bash
   # Watch for warnings
   sudo journalctl -u comic-server --since today | grep -i warn
   ```

## See Also

- [Installation Guide](INSTALLATION.md) - Installation instructions
- [Configuration Reference](CONFIGURATION.md) - Configuration options
- [API Reference](API.md) - API documentation
- [Protocol Specification](../WIRELESS_SYNC_PROTOCOL.md) - Protocol details
