# Test Client

A test client that simulates a ComicRack Android device for testing comic-server without physical hardware.

## Features

- **UDP Discovery Broadcasting**: Broadcasts device discovery messages every 5 seconds on the multicast group
- **TCP Command Server**: Listens on port 7614 for server commands
- **Full Protocol Support**: Implements all ComicRack wireless sync protocol commands
- **File Storage**: Saves received comic files and metadata to local storage
- **Metadata Display**: Parses and displays comic book metadata from sidecar XML files

## Usage

### Build

```bash
go build -o testclient ./cmd/testclient
```

Or use the justfile:

```bash
just build-testclient
```

### Basic Usage

```bash
# Start test client (broadcasts discovery, doesn't request sync)
./testclient

# Start with custom device name
./testclient --name "My Test Tablet"

# Request automatic sync when discovered
./testclient --sync

# Use custom storage directory
./testclient --storage /tmp/test-comics
```

### Flags

- `--device-id STRING` - Device ID (default: "test-device-123")
- `--name STRING` - Device display name (default: "Test Device")
- `--storage PATH` - Directory to store synced files (default: "./test-storage")
- `--sync` - Request sync on discovery (adds ":Sync" to broadcast message)
- `--multicast-ip STRING` - Multicast group IP (default: "224.34.123.90")
- `--multicast-port INT` - Multicast port (default: 7615)
- `--device-port INT` - Device TCP port (default: 7614)

## Testing Sync Flow

### 1. Start the test client

```bash
./testclient --sync --storage ./test-comics
```

You should see:
```
🤖 ComicRack Test Client
   Device ID: test-device-123
   Device Name: Test Device
   Storage: ./test-comics
   Request Sync: true

📡 Listening for commands on port 7614

📡 Broadcasting discovery: ComicRackAndroid:test-device-123:Sync
   (Press Ctrl+C to stop)
```

### 2. Start the server

In another terminal:

```bash
./comic-server server --library ~/.local/share/ComicRack/ComicDb.xml --auto-sync
```

### 3. Watch the sync happen

The test client will display all received commands:

```
📥 CommandInfo - Sending device info
📥 CommandStart - Sync session starting
📥 CommandFreeSpace - Reporting storage
📥 CommandListFiles - Listing files
   Found 0 files
📥 CommandWriteFile - Writing abc123.cbp (15234567 bytes)
   💾 Comic file saved
📥 CommandWriteFile - Writing abc123.cbp.xml (8456 bytes)
   📖 Book: Amazing Spider-Man #1 (Amazing Spider-Man #1)
📊 Progress: 50%
...
📥 CommandCompleted - Sync session complete
```

### 4. Check synced files

```bash
ls -lh ./test-comics/
```

You should see `.cbp` comic files and `.cbp.xml` sidecar metadata files.

## Protocol Commands Supported

- **CommandInfo** (9) - Returns device information (licensed, version code)
- **CommandStart** (6) - Acknowledges sync session start
- **CommandCompleted** (7) - Acknowledges sync session completion
- **CommandFreeSpace** (2) - Reports 10GB free storage
- **CommandListFiles** (0) - Returns list of files in storage directory
- **CommandReadFile** (1) - Reads and returns file contents
- **CommandWriteFile** (5) - Saves received file to storage
- **CommandDeleteFile** (4) - Deletes file from storage
- **CommandProgressUpdate** (8) - Displays sync progress percentage
- **CommandCheckAbort** (11) - Always returns false (never abort)
- **CommandReadMultiFile** (10) - Reads and returns multiple files

## File Storage

Files are stored in the storage directory with the same structure as on a real device:

```
test-storage/
├── abc123.cbp          # Comic book file
├── abc123.cbp.xml      # Metadata sidecar
├── def456.cbp
├── def456.cbp.xml
└── sync_information.xml  # Reading lists
```

## Debugging

### Enable verbose output

The test client already prints all commands received. To see more details, check the server logs.

### Check network connectivity

```bash
# Verify multicast messages are being sent
sudo tcpdump -i any -n udp port 7615

# Verify TCP connections
sudo tcpdump -i any -n tcp port 7614
```

### Inspect received files

```bash
# Check comic metadata
cat test-storage/*.xml

# Verify comic files
file test-storage/*.cbp
```

## Limitations

- **Storage**: Always reports 10GB free (hardcoded)
- **Device Info**: Simulates a basic unlicensed device
- **No comicrack.ini**: Unlike real devices, this doesn't have an actual INI file
  (the server reads it via CommandReadFile, which returns empty data)

## Use Cases

1. **Development**: Test sync logic without physical devices
2. **CI/CD**: Automated integration testing
3. **Protocol Debugging**: See exactly what commands the server sends
4. **Performance Testing**: Run multiple clients to test concurrent sync

## Example: Multiple Devices

```bash
# Terminal 1
./testclient --device-id tablet-1 --name "Tablet 1" --storage ./tablet1 --device-port 7614

# Terminal 2
./testclient --device-id tablet-2 --name "Tablet 2" --storage ./tablet2 --device-port 8614

# Update server to listen on both ports...
```

Note: Full multi-device testing requires server changes (currently only supports one device port).
