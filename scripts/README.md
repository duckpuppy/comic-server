# Service Installation

This directory contains service configuration files for running comic-server as a background daemon.

## Linux (systemd)

### Installation

1. **Copy the binary:**
   ```bash
   sudo cp comic-server /usr/local/bin/
   sudo chmod +x /usr/local/bin/comic-server
   ```

2. **Create service user (recommended for security):**
   ```bash
   sudo useradd -r -s /bin/false -d /var/lib/comic-server comic-server
   sudo mkdir -p /var/lib/comic-server
   sudo chown comic-server:comic-server /var/lib/comic-server
   ```

3. **Edit the service file:**
   ```bash
   # Copy the service file
   sudo cp scripts/comic-server.service /etc/systemd/system/

   # Edit to set your library path
   sudo nano /etc/systemd/system/comic-server.service
   # Change: ExecStart=/usr/local/bin/comic-server server --library /path/to/ComicDb.xml --auto-sync
   ```

4. **Enable and start the service:**
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable comic-server
   sudo systemctl start comic-server
   ```

### Management

```bash
# Check status
sudo systemctl status comic-server

# View logs
sudo journalctl -u comic-server -f

# Reload configuration (sends SIGHUP)
sudo systemctl reload comic-server

# Restart service
sudo systemctl restart comic-server

# Stop service
sudo systemctl stop comic-server
```

### Configuration

The service uses the configuration file at `~/.config/comic-server/config.yaml` by default.

To reload the configuration without restarting:
```bash
sudo systemctl reload comic-server
```

This sends a SIGHUP signal to the process, which reloads:
- Configuration file settings
- Device configurations
- Library (if path changed)
- Log level and format

---

## macOS (launchd)

### Installation

1. **Copy the binary:**
   ```bash
   sudo cp comic-server /usr/local/bin/
   sudo chmod +x /usr/local/bin/comic-server
   ```

2. **Edit the plist file:**
   ```bash
   # Edit the plist to set your library path
   nano scripts/com.comic-server.plist
   # Change the library path in ProgramArguments
   ```

3. **Install the plist:**
   ```bash
   # System-wide (runs as root)
   sudo cp scripts/com.comic-server.plist /Library/LaunchDaemons/
   sudo launchctl load /Library/LaunchDaemons/com.comic-server.plist

   # OR user-level (runs as current user)
   cp scripts/com.comic-server.plist ~/Library/LaunchAgents/
   launchctl load ~/Library/LaunchAgents/com.comic-server.plist
   ```

### Management

```bash
# System-wide service
sudo launchctl start com.comic-server
sudo launchctl stop com.comic-server
sudo launchctl unload /Library/LaunchDaemons/com.comic-server.plist

# User-level service
launchctl start com.comic-server
launchctl stop com.comic-server
launchctl unload ~/Library/LaunchAgents/com.comic-server.plist

# View logs
tail -f /usr/local/var/log/comic-server.log
tail -f /usr/local/var/log/comic-server.error.log
```

### Configuration Reload

To reload configuration on macOS:
```bash
# Send SIGHUP signal
kill -HUP $(pgrep comic-server)
```

---

## Manual Daemon Mode

If you prefer not to use systemd or launchd, you can run comic-server manually in the background:

```bash
# Start in background
nohup comic-server server --library /path/to/ComicDb.xml --auto-sync > /var/log/comic-server.log 2>&1 &

# Save PID for later
echo $! > /var/run/comic-server.pid

# Reload configuration
kill -HUP $(cat /var/run/comic-server.pid)

# Stop
kill $(cat /var/run/comic-server.pid)
```

---

## Security Considerations

### systemd Hardening

The provided systemd service file includes security hardening:
- `NoNewPrivileges=true` - Prevents privilege escalation
- `PrivateTmp=true` - Isolated /tmp directory
- `ProtectSystem=strict` - Read-only root filesystem
- `ProtectHome=read-only` - Read-only home directories
- `ReadWritePaths=/var/lib/comic-server` - Only this directory is writable

### File Permissions

Ensure proper permissions on configuration and library files:
```bash
# Config file should be readable by the service user
chmod 644 ~/.config/comic-server/config.yaml

# Library file should be readable
chmod 644 /path/to/ComicDb.xml
```

### Firewall

Comic-server uses these ports:
- **UDP 7615** - Device discovery (multicast)
- **TCP 7614** - Device communication
- **TCP 7620+** - Server control

Ensure your firewall allows:
```bash
# Linux (firewalld)
sudo firewall-cmd --permanent --add-port=7615/udp
sudo firewall-cmd --permanent --add-port=7614/tcp
sudo firewall-cmd --permanent --add-port=7620/tcp
sudo firewall-cmd --reload

# Linux (ufw)
sudo ufw allow 7615/udp
sudo ufw allow 7614/tcp
sudo ufw allow 7620/tcp

# macOS
# Add rules in System Preferences > Security & Privacy > Firewall > Firewall Options
```

---

## Troubleshooting

### Service won't start

1. Check logs:
   ```bash
   # systemd
   sudo journalctl -u comic-server -n 50

   # launchd
   tail -50 /usr/local/var/log/comic-server.error.log
   ```

2. Test manually:
   ```bash
   comic-server server --library /path/to/ComicDb.xml --log-level debug
   ```

3. Check file permissions:
   ```bash
   ls -l /usr/local/bin/comic-server
   ls -l /path/to/ComicDb.xml
   ```

### Configuration reload not working

1. Verify SIGHUP is being received:
   ```bash
   # systemd - should show "Received SIGHUP, reloading configuration"
   sudo journalctl -u comic-server -f

   # Then reload
   sudo systemctl reload comic-server
   ```

2. Check configuration file syntax:
   ```bash
   comic-server server --library /path/to/ComicDb.xml --dry-run
   ```

### Multicast discovery not working

1. Ensure IGMP is allowed through firewall
2. Check network interface supports multicast:
   ```bash
   ip maddr show  # Linux
   netstat -g     # macOS
   ```

3. Test discovery manually:
   ```bash
   comic-server discover
   ```
