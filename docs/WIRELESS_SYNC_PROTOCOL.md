# ComicRack Wireless Sync Protocol Specification

**Version:** 1.0
**Last Updated:** 2025-10-22
**Purpose:** Complete technical specification for implementing a headless ComicRack wireless sync server

---

## Table of Contents

1. [Overview](#overview)
2. [Network Architecture](#network-architecture)
3. [Device Discovery Protocol](#device-discovery-protocol)
4. [Binary Protocol Specification](#binary-protocol-specification)
5. [Command Reference](#command-reference)
6. [Device Information Format](#device-information-format)
7. [File Organization](#file-organization)
8. [Sync Flow](#sync-flow)
9. [Security & Validation](#security--validation)
10. [Error Handling](#error-handling)
11. [Configuration](#configuration)

---

## Overview

The ComicRack wireless sync protocol enables synchronization of comic books between a ComicRack server and Android/iOS client devices over TCP/IP networks. The protocol uses:

- **Binary protocol** over TCP for command/response communication
- **UDP multicast** for device discovery
- **INI file format** for device configuration
- **XML serialization** for metadata and reading lists

### Key Characteristics

- Protocol Version: `1` (CurrentSyncVersion)
- Big-endian byte order for integers and longs
- UTF-8 text encoding
- Stateless command/response model
- Each command is a separate TCP connection

---

## Network Architecture

### Port Assignments

| Port | Protocol | Direction | Purpose |
|------|----------|-----------|---------|
| **7614** | TCP | Server → Device | Device client socket (commands sent to device) |
| **7615** | UDP Multicast | Device → Server | Device discovery broadcasts (224.34.123.90) |
| **7620+** | TCP | Device → Server | Server control port (device control messages) |

### Communication Model

```
┌─────────────┐                    ┌─────────────┐
│   Server    │                    │   Device    │
│             │                    │             │
│             │  UDP Multicast     │             │
│             │◄───────────────────┤ Port 7615   │
│ Port 7615   │  "ComicRack:key"   │             │
│             │                    │             │
│             │  TCP Command       │             │
│ (client)    ├───────────────────►│ Port 7614   │
│             │  e.g. CommandStart │             │
│             │                    │             │
│ Port 7620+  │  TCP Control Msg   │             │
│             │◄───────────────────┤ (client)    │
│             │                    │             │
└─────────────┘                    └─────────────┘
```

---

## Device Discovery Protocol

### 1. Device Announces Availability (UDP Broadcast)

Devices send UDP multicast packets to `224.34.123.90:7615`:

**Format:**

```
"ComicRack:{device_key}"
```

Or when requesting sync:

```
"ComicRack:{device_key}:Sync"
```

**Example:**

```
"ComicRack:abc123def456"
"ComicRack:abc123def456:Sync"
```

### 2. Server Listens for Broadcasts

The server:

1. Binds to UDP port 7615 with `SO_REUSEADDR` option
2. Joins multicast group `224.34.123.90`
3. Listens for messages starting with `"ComicRack:"`
4. Extracts device key from message
5. Stores device IP address in internal registry

### 3. Server Notifies Devices

**Periodic Availability Notification:**

- Server sends every **10 seconds** to known device IPs
- TCP connection to device port **7614**
- Command: `CommandServerAvailable` (13)
- Payload: Server control port number (int)

**Command Sequence:**

```
BYTE: 13 (CommandServerAvailable)
INT:  7620 (control port number)
```

### 4. Device Pong Response

When device receives server availability:

- Device may respond with `CommandClientPong` (12)
- Server validates device is paired

---

## Binary Protocol Specification

### Data Type Encoding

All multi-byte values use **big-endian byte order** on little-endian systems (network byte order).

#### BYTE (1 byte)

```
Raw byte value (0-255)
```

#### BOOL (1 byte)

```
0x00 = false
0x01-0xFF = true
```

#### INT (4 bytes, big-endian)

```
[byte3][byte2][byte1][byte0]
```

**C# Encoding:**

```csharp
if (BitConverter.IsLittleEndian)
    value = value.EndianSwap();
byte[] bytes = BitConverter.GetBytes(value);
socket.Send(bytes);
```

#### LONG (8 bytes, big-endian)

```
[byte7][byte6][byte5][byte4][byte3][byte2][byte1][byte0]
```

**C# Encoding:**

```csharp
if (BitConverter.IsLittleEndian)
    value = value.EndianSwap();
socket.Send(BitConverter.GetBytes(value));
```

#### STRING (UTF-8)

```
INT:  length (number of UTF-8 bytes)
BYTE[length]: UTF-8 encoded bytes
```

**C# Encoding:**

```csharp
byte[] utf8Bytes = Encoding.UTF8.GetBytes(text);
SendInteger(socket, utf8Bytes.Length);
socket.Send(utf8Bytes);
```

**C# Decoding:**

```csharp
int length = ReadInteger(socket);
byte[] buffer = new byte[length];
ReadBlocking(socket, buffer);
string text = Encoding.UTF8.GetString(buffer);
```

#### DATA (raw bytes with length prefix)

```
LONG: data_length (number of bytes)
BYTE[data_length]: raw data bytes
```

**Used for:** File contents, serialized objects

---

## Command Reference

All commands follow the pattern:

1. Open TCP connection to device port 7614
2. Send command byte
3. Send/receive command-specific data
4. Close connection

### Command Summary

| Command | Value | Direction | Purpose |
|---------|-------|-----------|---------|
| CommandListFiles | 0 | Server → Device | Get list of files on device |
| CommandReadFile | 1 | Server → Device | Read single file from device |
| CommandFreeSpace | 2 | Server → Device | Get available storage space |
| CommandFileExists | 3 | Server → Device | Check if file exists |
| CommandDeleteFile | 4 | Server → Device | Delete file from device |
| CommandWriteFile | 5 | Server → Device | Write file to device |
| CommandStart | 6 | Server → Device | Start synchronization session |
| CommandCompleted | 7 | Server → Device | Synchronization completed |
| CommandProgressUpdate | 8 | Server → Device | Report progress percentage |
| CommandInfo | 9 | Server → Device | Get device info and validate |
| CommandReadMultiFile | 10 | Server → Device | Read multiple files efficiently |
| CommandCheckAbort | 11 | Server → Device | Check if user aborted sync |
| CommandClientPong | 12 | Server → Device | Respond to availability check |
| CommandServerAvailable | 13 | Server → Device | Notify device server is available |

---

### CommandStart (6)

Signals the start of a synchronization session.

**Request:**

```
BYTE: 6 (CommandStart)
STRING: "Start Synchronizing"
```

**Response:**

```
(None - connection closes)
```

---

### CommandInfo (9)

Retrieves device information and validates the device.

**Request:**

```
BYTE: 6 (CommandInfo)
INT: 1 (protocol version)
```

**Response:**

```
BOOL: licensed (is device licensed/full version)
INT: versionCode (app version number)
STRING: certificate_key (APK signing certificate - hex string)
BOOL: additional_flag (reserved)
```

**Validation Rules:**

- Android Full: certificate_key must match AndroidKey or AndroidDebugKey
- iOS: certificate validation skipped
- Version must match device.Edition minimum version
- License status must match expected value

**Certificate Keys:**

*AndroidDebugKey:*

```
3082030d308201f5a0030201020204494d03a7300d06092a864886f70d01010b05003037310b300906035504
06130255533110300e060355040a1307416e64726f6964311630140603550403130d416e64726f69642044
65627567301e170d3135303732323134353830375a170d3435303731343134353830375a3037310b300906
03550406130255533110300e060355040a1307416e64726f6964311630140603550403130d416e64726f69
6420446562756730820122300d06092a864886f70d01010105000382010f003082010a02820101009ebd1f
327aa7fd9d5c556df9e09ce4d7f091b04ffe649bf0c286fcd7d2efb24c485f02b4518d08227285d1758f0
e6ba44ec3d2dd16a53d34f790452c2b25166db2488ac8de275cbe575325a4f19a476e23cd0831e7b05bb7
28525500e516bb24b20444ce79ec5625cf5963e4b792f8c5017fc36b880b8b78750bdcace2d4e25aee155
aab3a4bb1c1c9a539a73edfc1057d77080e85c9506b033ffc72efe2c418f91171b78899be0d9fb04e5bef
d57cae955d7a81aff4573362d74571bd84d8ca5502cf99ad3124a6dfe45428b38c50300af28c13006803f
eadfc3aed84027b5fc4d0350ff41612652368f49b49c0461ff845b9c1e1bea440df4f65805f582b150203
010001a321301f301d0603551d0e0416041490edd92d5d54c07225eb32807acdd2bd57a169a4300d0609
2a864886f70d01010b050003820101000932afee5e88d0b1252c84d1d9b8533c5d57fbd0e4766c53ce0f
6565e04fe06c4c23687e109d9e23569736f3ee2105c57630b3b66229d25d910293c3d615e81290e932ec
f321cc59a2dbecb4acf89a811cd63f10611b01d546f2aea1a23f259bb7f4e833117396e2e62c28a331d5
d9a3fb625e199438635dd65e2da46fd4687aea161e9a490a597e264573be8821e2f3e7826df68dfee333
301d968d154636497e76851f838df16d9d428b390b5b19f7b6ddbdbb1e19395f349f169764f38c114a91
eaa831195a8e2c6217d1ac385ca4f6301f385fe44bdb82bbf6cd64033cd54e334500cec523e6d5abcc3f
6bed5538ad7830ec45a897b1752f3695063e4853
```

*AndroidKey (Release):*

```
3082019d30820106a00302010202044e7b1922300d06092a864886f70d010105050030133111300f060355
040a130863596f20536f6674301e170d3131303932323131313635305a170d33363039313531313136353
05a30133111300f060355040a130863596f20536f667430819f300d06092a864886f70d010101050003818
d00308189028181008d81ffb74008a048c517275a464db26461df06a3c85675b6ffa8bea15ec9288eec1e
f1bf7616d09b7265bf1c5666473342c2a96ca385769592d73a21595335e5173c69ae5bb7aebd29387e963
5ce30bdf11afff71145570b6577799ecac6100bcf0b2c4df6fe34fb8a418b5511c6a56c97b15c544269e9
1478ee24633ef063090203010001300d06092a864886f70d010105050003818100802cf8770c7af0744f96
80b54da88b56eb1d6a48e8d446ce746817fe959991dc1f882323c6015edd4d48f28cfe3a94e30b75c9285
5b01a8c48354aae8e13a0b949390133c07b09419ac73d0b0b3dc13b2838fe9eae4b171c8022cb47ead602
771560277cde7ad61e1a9ce5dee880d0226ed8cc71f36fb376d271a3cb61f1128b
```

---

### CommandListFiles (0)

Returns a newline-separated list of files on the device.

**Request:**

```
BYTE: 0 (CommandListFiles)
```

**Response:**

```
STRING: "file1.cbp\nfile2.cbp\nfile3.cbp"
```

**Notes:**

- Files are separated by `\n` (newline)
- Server filters for valid sync files (`.cbp` extension)
- Empty entries are removed

---

### CommandReadFile (1)

Reads a single file from the device.

**Request:**

```
BYTE: 1 (CommandReadFile)
STRING: filename (e.g., "comic.cbp.xml")
```

**Response:**

```
DATA: file contents
  LONG: file_size
  BYTE[file_size]: file data
```

**Example:**

```
Request:
  0x01
  0x00 0x00 0x00 0x0D
  "comic.cbp.xml"

Response:
  0x00 0x00 0x00 0x00 0x00 0x00 0x02 0x3A  (size: 570 bytes)
  [570 bytes of XML data]
```

---

### CommandFreeSpace (2)

Returns available storage space on device.

**Request:**

```
BYTE: 2 (CommandFreeSpace)
```

**Response:**

```
LONG: free_bytes
```

**Example:**

```
Response: 0x00 0x00 0x00 0x00 0x3B 0x9A 0xCA 0x00
         (1,000,000,000 bytes = ~954 MB)
```

---

### CommandFileExists (3)

Checks if a file exists on the device.

**Request:**

```
BYTE: 3 (CommandFileExists)
STRING: filename
```

**Response:**

```
BOOL: exists (0x00 = false, 0x01 = true)
```

---

### CommandDeleteFile (4)

Deletes a file from the device.

**Request:**

```
BYTE: 4 (CommandDeleteFile)
STRING: filename
```

**Response:**

```
BOOL: success (0x01 = deleted, 0x00 = failed)
```

**Notes:**

- Always delete both `.cbp` file and `.cbp.xml` sidecar
- Response indicates success/failure

---

### CommandWriteFile (5)

Writes a file to the device.

**Request:**

```
BYTE: 5 (CommandWriteFile)
STRING: filename
LONG: file_size
BYTE[file_size]: file_data
```

**Response:**

```
BOOL: success (0x01 = written, 0x00 = failed)
```

**Implementation Notes:**

- Send file data in chunks (e.g., 100,000 bytes)
- Device writes to storage
- Response sent after complete write

**Example:**

```
Request:
  0x05
  0x00 0x00 0x00 0x09
  "comic.cbp"
  0x00 0x00 0x00 0x00 0x00 0x0F 0x42 0x40  (1,000,000 bytes)
  [1,000,000 bytes of CBP data in 100KB chunks]

Response:
  0x01  (success)
```

---

### CommandCompleted (7)

Signals synchronization completion.

**Request:**

```
BYTE: 7 (CommandCompleted)
STRING: "Synchronization completed"
```

**Response:**

```
(None - connection closes)
```

**Side Effect:**

- Device updates marker file timestamp (comicrack.ini)

---

### CommandProgressUpdate (8)

Reports sync progress to device.

**Request:**

```
BYTE: 8 (CommandProgressUpdate)
BYTE: percent (0-100)
```

**Response:**

```
(None - connection closes)
```

**Example:**

```
0x08 0x32  (50% complete)
```

---

### CommandReadMultiFile (10)

Efficiently reads multiple files in a single command.

**Request:**

```
BYTE: 10 (CommandReadMultiFile)
INT: file_count
STRING: filename_1
STRING: filename_2
...
STRING: filename_n
```

**Response:**

```
DATA: file_1_contents
  LONG: file_1_size
  BYTE[file_1_size]: file_1_data
DATA: file_2_contents
  LONG: file_2_size
  BYTE[file_2_size]: file_2_data
...
DATA: file_n_contents
```

**Example:**

```
Request:
  0x0A
  0x00 0x00 0x00 0x02  (2 files)
  0x00 0x00 0x00 0x0F
  "comic1.cbp.xml"
  0x00 0x00 0x00 0x0F
  "comic2.cbp.xml"

Response:
  0x00 0x00 0x00 0x00 0x00 0x00 0x02 0x3A  (570 bytes)
  [570 bytes of comic1 XML]
  0x00 0x00 0x00 0x00 0x00 0x00 0x01 0xF4  (500 bytes)
  [500 bytes of comic2 XML]
```

**Notes:**

- Used for initial sync to retrieve all book metadata
- More efficient than multiple CommandReadFile calls
- Typically used to read `.cbp.xml` sidecar files

---

### CommandCheckAbort (11)

Checks if user requested sync cancellation.

**Request:**

```
BYTE: 11 (CommandCheckAbort)
```

**Response:**

```
BOOL: should_abort (0x00 = continue, 0x01 = abort)
```

**Usage:**

- Called periodically during sync (e.g., after each book)
- Server should abort sync if response is true

---

### CommandClientPong (12)

Responds to server availability check (device pong).

**Request:**

```
BYTE: 12 (CommandClientPong)
```

**Response:**

```
(None - connection closes)
```

**Usage:**

- Sent from server to device when device is found but not syncing
- Confirms device is alive and paired

---

### CommandServerAvailable (13)

Notifies device that server is available for sync.

**Request:**

```
BYTE: 13 (CommandServerAvailable)
INT: control_port (e.g., 7620)
```

**Response:**

```
(None - connection closes)
```

**Usage:**

- Sent periodically (every 10 seconds) to known devices
- Informs device of server's control port
- Allows device to initiate sync requests

---

## Device Information Format

### Marker File: `comicrack.ini`

The marker file identifies a valid ComicRack sync device and stores device metadata.

**Location:** Root of device sync storage
**Format:** INI file (key=value pairs)

#### Required Fields

```ini
Name=Galaxy Tab S7
Model=SM-T870
Manufacturer=Samsung
Serial=R52M123456A
ID=abc123def456789
Hash=1a2b3c4d5e6f7g8h9i0j
Version=123
Edition=Android Full
Screen=2560,1600,320.0,320.0
Capabilities=WEBP
```

#### Field Descriptions

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| Name | string | User-friendly device name | `Galaxy Tab S7` |
| Model | string | Device model identifier | `SM-T870` |
| Manufacturer | string | Device manufacturer | `Samsung` |
| Serial | string | Device serial number (unique) | `R52M123456A` |
| ID | string | Device key (used in discovery) | `abc123def456789` |
| Hash | string | SHA1 hash for validation | `1a2b3c4d5e6f7g8h9i0j` |
| Version | int | App version code | `123` |
| Edition | string | App edition (see below) | `Android Full` |
| Screen | string | `width,height,dpiX,dpiY` | `2560,1600,320.0,320.0` |
| Capabilities | string | Comma-separated capabilities | `WEBP` |

#### Edition Values

| Edition String | Enum Value | Book Limit | Min Version |
|---------------|------------|------------|-------------|
| `Android Free` | AndroidFree | 100 books | 100 |
| `Android Full` | AndroidFull | Unlimited | 89 |
| `iOS` | iOS | Unlimited | 1 |

#### Hash Calculation

The hash validates device authenticity:

```csharp
string concatenated = Model + Manufacturer + Serial + Edition + Version;
byte[] bytes = Encoding.UTF8.GetBytes(concatenated);
SHA1 sha1 = new SHA1Managed();
byte[] hash = sha1.ComputeHash(bytes);
string hashHex = BitConverter.ToString(hash).Replace("-", "").ToLower();
```

**Example:**

```
Input: "SM-T870" + "Samsung" + "R52M123456A" + "Android Full" + "123"
SHA1:  1a2b3c4d5e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0t
Hex:   1a2b3c4d5e6f7g8h9i0j
```

#### Capabilities

Currently supported:

- `WEBP` - Device supports WebP image format

Multiple capabilities separated by commas (future-proofing).

---

## File Organization

### File Types

| Extension | Type | Description |
|-----------|------|-------------|
| `.cbp` | Comic Book Package | Compressed comic book file |
| `.cbp.xml` | Sidecar Metadata | Serialized ComicBook object (XML) |
| `sync_information.xml` | Reading Lists | Synced reading lists and metadata |
| `comicrack.ini` | Device Marker | Device identification and capabilities |

### Directory Structure

```
[Device Root]/
├── comicrack.ini                  (marker file)
├── sync_information.xml           (reading lists)
├── Batman #1.cbp                  (comic file)
├── Batman #1.cbp.xml              (metadata sidecar)
├── Spider-Man #50.cbp
├── Spider-Man #50.cbp.xml
└── ...
```

### File Naming

- Comic files use `.cbp` extension (Comic Book Package)
- Sidecar files append `.xml` to the comic filename
- Duplicate filenames get numbered suffix: `filename (2).cbp`

### Sidecar Metadata (.cbp.xml)

Sidecar files contain serialized `ComicBook` objects with:

- Metadata (series, title, volume, number, etc.)
- Page information (count, types, bookmarks)
- Reading progress (last page read, opened count)
- Ratings and timestamps

**Format:** XML serialization of ComicBook class (not detailed here)

### Reading Lists (sync_information.xml)

Contains synced reading lists.

**Format:**

```xml
<?xml version="1.0"?>
<SyncInformation>
  <Name>ComicRack</Name>
  <Version>1</Version>
  <Lists>
    <List Name="To Read">
      <Description>Books I want to read</Description>
      <Books>
        <Id>12345678-1234-1234-1234-123456789abc</Id>
        <Id>87654321-4321-4321-4321-cba987654321</Id>
      </Books>
    </List>
    <List Name="Favorites">
      <Description>My favorite comics</Description>
      <Books>
        <Id>aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee</Id>
      </Books>
    </List>
  </Lists>
</SyncInformation>
```

**Fields:**

- `Name`: Always "ComicRack"
- `Version`: Format version (currently 1)
- `Lists`: Collection of reading lists
  - `List`: Individual reading list
    - `Name`: List display name
    - `Description`: Optional description
    - `Books`: Collection of comic GUIDs

---

## Sync Flow

### Initial Device Discovery

```
1. Device → Server (UDP multicast 224.34.123.90:7615)
   "ComicRack:abc123def456"

2. Server receives broadcast
   - Extracts device key: "abc123def456"
   - Stores device IP address
   - Attempts to connect to device port 7614

3. Server → Device (TCP port 7614)
   CommandReadFile: "comicrack.ini"

4. Device → Server
   Returns comicrack.ini contents

5. Server validates device
   - Parses INI file
   - Creates DeviceInfo object
   - Validates hash
   - Checks minimum version
   - Stores device in registry
```

### Sync Session

```
1. CommandStart
   Server → Device: Start sync session

2. CommandInfo
   Server → Device: Validate device
   - Send protocol version (1)
   - Receive license, version, certificate key
   - Validate against expected values

3. CommandFreeSpace
   Server → Device: Check available space
   - Ensure sufficient storage

4. CommandListFiles
   Server → Device: Get list of files
   - Returns: "comic1.cbp\ncomic2.cbp\n..."

5. CommandReadMultiFile
   Server → Device: Read all sidecar files
   - Send list of .cbp.xml files
   - Receive all metadata in single response
   - Deserialize ComicBook objects

6. Sync Logic (Server-side)
   - Compare server library with device books
   - Determine books to add, remove, update

7. For each book to delete:
   CommandDeleteFile: "oldbook.cbp"
   CommandDeleteFile: "oldbook.cbp.xml"

8. For each book to add:
   a. CommandFileExists: Check if file exists
   b. CommandWriteFile: Write .cbp file
   c. CommandWriteFile: Write .cbp.xml sidecar
   d. CommandProgressUpdate: Report progress %

9. CommandWriteFile: "sync_information.xml"
   - Write reading lists

10. CommandCompleted
    Server → Device: Sync complete

11. CommandProgressUpdate: 100
    Final progress report
```

### Incremental Updates

If a book already exists on the device but has updated metadata:

```
1. Server reads existing sidecar from device
2. Compares page structure (count, types, positions)
3. If pages match:
   - Only update sidecar metadata
   - Skip re-transferring comic file
4. If pages differ:
   - Delete old files
   - Write new files
```

---

## Security & Validation

### Certificate Validation

Android devices must provide valid APK signing certificate:

**Accepted Certificates:**

1. **Debug Key** (development builds)
2. **Release Key** (production builds)

Validation is **skipped in current implementation** but keys are still transmitted and validated for iOS.

### Device Hash Validation

Every connection validates device authenticity:

```csharp
string expected = ComputeSHA1(Model + Manufacturer + Serial + Edition + Version);
if (device.DeviceHash != expected)
    throw new InvalidDataException();
```

### Network Security

- **No encryption** in current protocol (TCP plaintext)
- **No authentication** beyond device hash
- **Local network only** (multicast discovery requires LAN)

**Recommendations for headless server:**

- Implement TLS/SSL wrapper for TCP connections
- Add password/token authentication
- Restrict to trusted network segments

### Version Enforcement

Minimum version requirements prevent incompatible devices:

| Edition | Minimum Version |
|---------|----------------|
| Android Free | 100 |
| Android Full | 89 |
| iOS | 1 |

---

## Error Handling

### Connection Timeouts

```csharp
WifiSyncReceiveTimeout = 5000ms      // Wait for data from device
WifiSyncSendTimeout = 5000ms         // Wait to send data to device
WifiSyncConnectionTimeout = 2500ms   // Wait for connection establishment
WifiSyncConnectionRetries = 1        // Number of retry attempts
```

### Error Types

#### FatalSyncException

- Invalid device hash
- Certificate validation failure
- Wrong protocol version
- Unrecoverable read/write errors

#### DeviceOutOfSpaceException

- Insufficient free space on device
- Thrown before writing files
- Requires: `free_bytes - reserved_bytes < file_size`
- Reserved: 128 MB (FreeDeviceMemoryMB)

#### IOException

- File write failures
- File read failures
- Network disconnection

### Retry Logic

For each command:

1. Attempt connection with timeout
2. If fails, retry N times (WifiSyncConnectionRetries)
3. If all retries fail, throw exception

### Abort Handling

User can cancel sync:

- Server calls CommandCheckAbort periodically
- If device returns true, server aborts
- No cleanup commands sent (device handles cleanup)

---

## Configuration

### Server Configuration

Key configuration values (from EngineConfiguration):

```ini
; Network Timeouts
WifiSyncReceiveTimeout=5000          ; Socket receive timeout (ms)
WifiSyncSendTimeout=5000             ; Socket send timeout (ms)
WifiSyncConnectionTimeout=2500       ; Connection timeout (ms)
WifiSyncConnectionRetries=1          ; Retry attempts

; Sync Settings
SyncQueueLength=50                   ; Max concurrent book conversions
FreeDeviceMemoryMB=128               ; Reserved space on device (MB)
ParallelConversions=32               ; Max parallel book processing threads

; Image Optimization
SyncOptimizeQuality=65               ; JPEG quality (1-100)
SyncOptimizeMaxHeight=1500           ; Max image height (pixels)
SyncOptimizeSharpen=false            ; Apply sharpening
SyncOptimizeWebP=true                ; Use WebP for optimized sync
SyncWebP=true                        ; Use WebP for normal sync
SyncCreateThumbnails=true            ; Generate thumbnails

; Other
SyncResamping=GdiPlus                ; Image resampling method
```

### Manual Device Addition

If multicast discovery fails, manually add device IPs:

```ini
ExtraWifiDeviceAddresses=192.168.1.100,192.168.1.101
```

Server will periodically send CommandServerAvailable to these IPs.

---

## Implementation Checklist

For building a headless server, implement:

### Core Protocol

- [ ] TCP server listening on device client port (7614)
- [ ] UDP multicast listener on discovery port (7615)
- [ ] TCP control server on dynamic port (7620+)
- [ ] Binary protocol encoder/decoder (big-endian)
- [ ] All 14 command handlers

### Device Management

- [ ] Device discovery handler
- [ ] Device registry (IP → DeviceInfo mapping)
- [ ] INI file parser for comicrack.ini
- [ ] Device hash validation
- [ ] Certificate validation (optional)

### File Operations

- [ ] Virtual file system for device storage
- [ ] `.cbp` file handling (comic packages)
- [ ] `.cbp.xml` sidecar generation
- [ ] `sync_information.xml` generation
- [ ] File existence checking
- [ ] File deletion

### Sync Logic

- [ ] Book comparison (device vs. library)
- [ ] Incremental update detection
- [ ] Book export/conversion pipeline
- [ ] Reading list synchronization
- [ ] Progress tracking
- [ ] Abort handling

### Configuration

- [ ] Timeout configuration
- [ ] Image optimization settings
- [ ] Device space management
- [ ] Manual device IP addition

### Error Handling

- [ ] Connection retry logic
- [ ] Space validation before writes
- [ ] Graceful abort handling
- [ ] Exception logging

---

## Protocol Reference Implementation

**Location:** `/home/duckpuppy/src/ComicRackCE/ComicRack.Engine/Sync/WirelessSyncProvider.cs`

Key methods:

- `StartListen()` - Initialize UDP/TCP listeners (line 461)
- `OnReceivedBroadcastData()` - Handle device broadcasts (line 564)
- `Communicate()` - Execute commands with retry (line 288)
- `SendString()`, `SendInteger()`, `SendLong()`, `SendByte()` - Encoders (lines 332-389)
- `ReadString()`, `ReadInteger()`, `ReadLong()`, `ReadBool()` - Decoders (lines 347-407)

---

## Appendix: Wire Format Examples

### Example 1: CommandFileExists

**Request (check if "Batman #1.cbp" exists):**

```
Offset  Hex                                      ASCII
------  ---------------------------------------  ----------------
0x0000  03                                       .                # Command byte
0x0001  00 00 00 0D                              ....             # String length (13)
0x0005  42 61 74 6D 61 6E 20 23 31 2E 63 62 70  Batman #1.cbp    # Filename
```

**Response (file exists):**

```
Offset  Hex  ASCII
------  ---  -----
0x0000  01   .     # Boolean true (exists)
```

### Example 2: CommandWriteFile

**Request (write "test.cbp" with 5 bytes of data):**

```
Offset  Hex                                      ASCII
------  ---------------------------------------  ----------------
0x0000  05                                       .                # Command byte
0x0001  00 00 00 08                              ....             # String length (8)
0x0005  74 65 73 74 2E 63 62 70                  test.cbp         # Filename
0x000D  00 00 00 00 00 00 00 05                  ........         # Data length (5 bytes)
0x0015  48 65 6C 6C 6F                           Hello            # File data
```

**Response (write succeeded):**

```
Offset  Hex  ASCII
------  ---  -----
0x0000  01   .     # Boolean true (success)
```

### Example 3: CommandInfo

**Request:**

```
Offset  Hex              ASCII
------  ---------------  --------
0x0000  09               .        # Command byte
0x0001  00 00 00 01      ....     # Protocol version (1)
```

**Response:**

```
Offset  Hex              ASCII
------  ---------------  --------
0x0000  01               .        # Licensed (true)
0x0001  00 00 00 7B      ...{     # Version code (123)
0x0005  00 00 04 00      ....     # Certificate length (1024)
0x0009  [1024 bytes of hex certificate]
0x0409  00               .        # Additional flag (false)
```

---

## Version History

**v1.0** (2025-10-22)

- Initial protocol specification
- Documented all 14 commands
- Added wire format examples
- Included security considerations

---

## Contact & Support

This specification is based on ComicRack CE codebase analysis.
For questions about this protocol document, refer to the source code in:

- `/ComicRack.Engine/Sync/WirelessSyncProvider.cs`
- `/ComicRack.Engine/Sync/SyncProviderBase.cs`
- `/ComicRack.Engine/Sync/DeviceInfo.cs`

---

**End of Specification**
