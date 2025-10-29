# Installation Guide

This guide covers installing and setting up comic-server on various platforms.

## Prerequisites

- ComicRackCE installed with an existing comic library (ComicDb.xml)
- Go 1.25.1 or later (for building from source)
- Network connectivity between server and devices

## Quick Start (Recommended)

Using mise for tool management and just for task running:

```bash
# Install mise
curl https://mise.jdx.dev/install.sh | sh

# Activate mise in your shell
mise activate bash >> ~/.bashrc  # or ~/.zshrc for zsh
source ~/.bashrc

# Clone and install
git clone https://github.com/duckpuppy/comic-server.git
cd comic-server
mise install

# Build
just build

# Run
./comic-server server --library ~/.local/share/ComicRack/ComicDb.xml
```

## Platform-Specific Installation

### Linux

#### Method 1: Binary Installation (Easiest)

Download the latest release from GitHub:

```bash
# Download binary (replace VERSION with actual version)
wget https://github.com/duckpuppy/comic-server/releases/download/vVERSION/comic-server-linux-amd64

# Make executable
chmod +x comic-server-linux-amd64
sudo mv comic-server-linux-amd64 /usr/local/bin/comic-server

# Verify installation
comic-server version
```

#### Method 2: Build from Source

```bash
# Install Go 1.25.1+
wget https://go.dev/dl/go1.25.1.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.1.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Clone and build
git clone https://github.com/duckpuppy/comic-server.git
cd comic-server
go build -o comic-server

# Install
sudo mv comic-server /usr/local/bin/
```

#### systemd Service Installation

Create a systemd service for automatic startup:

```bash
# Create service file
sudo tee /etc/systemd/system/comic-server.service > /dev/null <<EOF
[Unit]
Description=Comic Server - Wireless Sync for ComicRack
After=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=$HOME
ExecStart=/usr/local/bin/comic-server server --library $HOME/.local/share/ComicRack/ComicDb.xml
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=10s

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=$HOME/.local/share/ComicRack

[Install]
WantedBy=multi-user.target
EOF

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable comic-server
sudo systemctl start comic-server

# Check status
sudo systemctl status comic-server

# View logs
sudo journalctl -u comic-server -f
```

### macOS

#### Method 1: Binary Installation

```bash
# Download binary (replace VERSION and ARCH)
# For Intel Macs: comic-server-darwin-amd64
# For Apple Silicon: comic-server-darwin-arm64
curl -L -o comic-server https://github.com/duckpuppy/comic-server/releases/download/vVERSION/comic-server-darwin-ARCH

# Make executable
chmod +x comic-server
sudo mv comic-server /usr/local/bin/

# Verify installation
comic-server version
```

#### Method 2: Build from Source

```bash
# Install Go (using Homebrew)
brew install go

# Clone and build
git clone https://github.com/duckpuppy/comic-server.git
cd comic-server
go build -o comic-server

# Install
sudo mv comic-server /usr/local/bin/
```

#### launchd Service Installation

Create a launch agent for automatic startup:

```bash
# Determine library path (adjust as needed)
LIBRARY_PATH="$HOME/Library/Application Support/ComicRack/ComicDb.xml"

# Create launch agent
tee ~/Library/LaunchAgents/com.comic-server.plist > /dev/null <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.comic-server</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/comic-server</string>
        <string>server</string>
        <string>--library</string>
        <string>$LIBRARY_PATH</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$HOME/Library/Logs/comic-server.log</string>
    <key>StandardErrorPath</key>
    <string>$HOME/Library/Logs/comic-server-error.log</string>
</dict>
</plist>
EOF

# Load and start service
launchctl load ~/Library/LaunchAgents/com.comic-server.plist

# Check status
launchctl list | grep comic-server

# View logs
tail -f ~/Library/Logs/comic-server.log
```

### Windows

#### Method 1: Binary Installation

1. Download `comic-server-windows-amd64.exe` from [GitHub Releases](https://github.com/duckpuppy/comic-server/releases)
2. Rename to `comic-server.exe`
3. Move to `C:\Program Files\comic-server\`
4. Add to PATH or run from that directory

```powershell
# Verify installation
comic-server.exe version
```

#### Method 2: Build from Source

```powershell
# Install Go (download from https://go.dev/dl/)
# Or use Chocolatey:
choco install golang

# Clone and build
git clone https://github.com/duckpuppy/comic-server.git
cd comic-server
go build -o comic-server.exe

# Install
mkdir "C:\Program Files\comic-server"
move comic-server.exe "C:\Program Files\comic-server\"
```

#### Windows Service Installation

Using NSSM (Non-Sucking Service Manager):

```powershell
# Download and install NSSM
choco install nssm

# Install service
nssm install comic-server "C:\Program Files\comic-server\comic-server.exe" server --library "C:\Users\YourUser\AppData\Roaming\ComicRack\ComicDb.xml"

# Configure service
nssm set comic-server AppDirectory "C:\Program Files\comic-server"
nssm set comic-server DisplayName "Comic Server"
nssm set comic-server Description "Wireless sync server for ComicRack devices"
nssm set comic-server Start SERVICE_AUTO_START

# Start service
nssm start comic-server

# Check status
nssm status comic-server
```

## Docker Installation

Run comic-server in a Docker container:

```bash
# Create Dockerfile
cat > Dockerfile <<EOF
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o comic-server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/comic-server .
EXPOSE 7614 7615/udp 7620
ENTRYPOINT ["./comic-server"]
CMD ["server", "--library", "/data/ComicDb.xml"]
EOF

# Build image
docker build -t comic-server .

# Run container
docker run -d \
  --name comic-server \
  --network host \
  -v /path/to/comics:/comics:ro \
  -v /path/to/ComicDb.xml:/data/ComicDb.xml:ro \
  comic-server
```

Note: `--network host` is required for multicast discovery to work properly.

## Verifying Installation

After installation, verify the server is working:

```bash
# Check version
comic-server version

# Test device discovery (run in separate terminal)
comic-server discover

# Start server
comic-server server --library /path/to/ComicDb.xml

# Check API (in another terminal)
curl http://localhost:7620/api/health
```

## Network Configuration

### Firewall Rules

Allow the following ports:

**Linux (ufw):**
```bash
sudo ufw allow 7614/tcp comment "Comic Server - Device Communication"
sudo ufw allow 7615/udp comment "Comic Server - Device Discovery"
sudo ufw allow 7620/tcp comment "Comic Server - API"
sudo ufw enable
```

**Linux (firewalld):**
```bash
sudo firewall-cmd --permanent --add-port=7614/tcp
sudo firewall-cmd --permanent --add-port=7615/udp
sudo firewall-cmd --permanent --add-port=7620/tcp
sudo firewall-cmd --reload
```

**macOS:**
```bash
# Add to /etc/pf.conf
pass in proto tcp from any to any port 7614
pass in proto udp from any to any port 7615
pass in proto tcp from any to any port 7620

# Reload firewall
sudo pfctl -f /etc/pf.conf
```

**Windows:**
```powershell
# Allow ports in Windows Firewall
netsh advfirewall firewall add rule name="Comic Server TCP" dir=in action=allow protocol=TCP localport=7614,7620
netsh advfirewall firewall add rule name="Comic Server UDP" dir=in action=allow protocol=UDP localport=7615
```

### Multicast Configuration

Ensure IGMP (Internet Group Management Protocol) is enabled:

**Linux:**
```bash
# Check if multicast is enabled
ip link show | grep MULTICAST

# Enable if needed
sudo ip link set dev eth0 multicast on
```

**Router Configuration:**
- Enable IGMP snooping on your router
- Ensure multicast traffic is allowed between VLANs (if applicable)

## Troubleshooting Installation

### Command Not Found

If `comic-server` command is not found after installation:

```bash
# Check if binary is in PATH
which comic-server

# Add to PATH if needed (Linux/macOS)
export PATH=$PATH:/usr/local/bin

# Make permanent by adding to ~/.bashrc or ~/.zshrc
echo 'export PATH=$PATH:/usr/local/bin' >> ~/.bashrc
```

### Permission Denied

If you get permission errors when accessing the library:

```bash
# Check library file permissions
ls -l /path/to/ComicDb.xml

# Fix permissions
chmod 644 /path/to/ComicDb.xml
```

### Service Won't Start

Check service logs:

**Linux:**
```bash
sudo journalctl -u comic-server -n 50
```

**macOS:**
```bash
tail -f ~/Library/Logs/comic-server-error.log
```

**Windows:**
```powershell
nssm status comic-server
Get-EventLog -LogName Application -Source comic-server -Newest 10
```

## Next Steps

After installation:

1. Configure the server: See [CONFIGURATION.md](CONFIGURATION.md)
2. Set up device sync: See [README.md](../README.md#usage)
3. Monitor the server: See [API.md](API.md)
4. Troubleshoot issues: See [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## Uninstallation

### Linux

```bash
# Stop and disable service
sudo systemctl stop comic-server
sudo systemctl disable comic-server
sudo rm /etc/systemd/system/comic-server.service
sudo systemctl daemon-reload

# Remove binary
sudo rm /usr/local/bin/comic-server
```

### macOS

```bash
# Unload service
launchctl unload ~/Library/LaunchAgents/com.comic-server.plist
rm ~/Library/LaunchAgents/com.comic-server.plist

# Remove binary
sudo rm /usr/local/bin/comic-server
```

### Windows

```powershell
# Remove service
nssm stop comic-server
nssm remove comic-server confirm

# Remove binary
Remove-Item "C:\Program Files\comic-server" -Recurse
```
