package protocol

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/duckpuppy/comic-server/internal/log"
)

const (
	// Default timeout values from EngineConfiguration (from WIRELESS_SYNC_PROTOCOL.md)
	DefaultReceiveTimeout    = 5000 * time.Millisecond // 5 seconds
	DefaultSendTimeout       = 5000 * time.Millisecond // 5 seconds
	DefaultConnectionTimeout = 2500 * time.Millisecond // 2.5 seconds
	DefaultRetries           = 1

	// minTransferThroughput is a deliberately conservative assumption for
	// how slow the link to a device can get before a transfer is
	// reasonably considered stalled, not just slow - used to size the
	// deadline for operations moving real file data (WriteFile, ReadFile)
	// instead of applying receiveTimeout (meant for small protocol
	// messages: a command byte, a filename) to a multi-megabyte comic
	// book. ComicRackCE's own client sidesteps this differently - its
	// SendTimeout/ReceiveTimeout are PER-Send()/Receive()-call timeouts
	// on a chunked ~100KB-at-a-time transfer, not one deadline for the
	// whole file - but the same effect (a slow-but-alive transfer
	// shouldn't fail just because it's large) is reached here more simply
	// by sizing the one deadline to the payload. Found live 2026-08-27:
	// average book size in a real sync was ~90MB, nowhere close to
	// transferable in the flat 5s default, independent of any actual
	// WiFi flakiness - explains a large share of that session's i/o
	// timeout failures on WriteFile.
	minTransferThroughput = 1024 * 1024 // 1 MB/s
)

// Client represents a TCP client for communicating with ComicRack devices
type Client struct {
	deviceIP          string
	devicePort        int
	receiveTimeout    time.Duration
	sendTimeout       time.Duration
	connectionTimeout time.Duration
	retries           int
}

// NewClient creates a new protocol client for a device
func NewClient(deviceIP string, devicePort int) *Client {
	return &Client{
		deviceIP:          deviceIP,
		devicePort:        devicePort,
		receiveTimeout:    DefaultReceiveTimeout,
		sendTimeout:       DefaultSendTimeout,
		connectionTimeout: DefaultConnectionTimeout,
		retries:           DefaultRetries,
	}
}

// SetTimeouts allows customizing timeout values
func (c *Client) SetTimeouts(receive, send, connection time.Duration) {
	c.receiveTimeout = receive
	c.sendTimeout = send
	c.connectionTimeout = connection
}

// SetRetries sets the number of connection retry attempts
func (c *Client) SetRetries(retries int) {
	c.retries = retries
}

// dataTimeout returns the deadline duration for an operation moving
// dataSize bytes of real file data - at least c.receiveTimeout (the
// default, meant for small protocol messages), scaled up for anything
// large enough that minTransferThroughput would need longer. See
// minTransferThroughput's comment for why this exists.
func (c *Client) dataTimeout(dataSize int64) time.Duration {
	scaled := time.Duration(dataSize/minTransferThroughput) * time.Second
	if scaled > c.receiveTimeout {
		return scaled
	}
	return c.receiveTimeout
}

// dial opens a single TCP connection to the device - no retry, that's
// execute's job (see below).
func (c *Client) dial() (net.Conn, error) {
	address := net.JoinHostPort(c.deviceIP, strconv.Itoa(c.devicePort))
	return net.DialTimeout("tcp", address, c.connectionTimeout)
}

// execute performs a command with automatic connection management,
// timeouts, and retries. On any failure - a failed connect, or the
// command itself failing partway through (e.g. a write that times out
// mid-transfer on flaky WiFi) - the WHOLE attempt is retried with a
// brand-new connection, up to c.retries additional times.
//
// This matches ComicRackCE's own WirelessSyncProvider.Communicate(),
// confirmed from its source: every command there opens its own
// short-lived socket and retries the entire operation (not just the
// connect) on any exception, using a fresh socket each time. comic-server
// used to only retry the connect phase - once connected, a command that
// failed partway through was never retried, it just failed that one
// operation outright. Against unstable WiFi (a connection that drops
// mid-transfer, not just a connect that never succeeds), that meant many
// avoidable failures that ComicRackCE's own client would have quietly
// recovered from. Found 2026-08-27 while comparing retry behavior
// against ComicRackCE's source during a real high-timeout-rate sync.
func (c *Client) execute(commandFunc func(conn net.Conn) error) error {
	var lastErr error

	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			// A retry after a mid-transfer failure reconnects and
			// re-sends the whole command from scratch (matches
			// ComicRackCE's own Communicate(), which does the same on
			// any exception - see comic-server-z0m). Neither client
			// sends any explicit "discard what you started" signal
			// first. If the device's per-write staging isn't reset on a
			// fresh CommandWriteFile for the same filename (e.g. it
			// appends instead of truncating), a retried WriteFile could
			// silently produce a corrupted, oversized archive - and
			// until this log line existed, a retry that eventually
			// succeeded was invisible: execute() only ever logged the
			// final outcome, never individual attempt failures. Added
			// live 2026-08-28 after finding two large (89MB, 127MB)
			// synced comics reported as corrupt on-device despite
			// verified-clean source files and error-free sync logs -
			// this is the only way to confirm or rule that out next
			// time it happens (comic-server-oqf).
			log.Warn().
				Int("attempt", attempt).
				Int("max_retries", c.retries).
				Err(lastErr).
				Msg("Retrying command after failure")
			time.Sleep(100 * time.Millisecond)
		}

		lastErr = c.executeOnce(commandFunc)
		if lastErr == nil {
			return nil
		}
	}

	return fmt.Errorf("failed after %d retries: %w", c.retries, lastErr)
}

// executeOnce is one full attempt: dial, set the deadline, run the
// command. No retry logic here - see execute.
func (c *Client) executeOnce(commandFunc func(conn net.Conn) error) error {
	conn, err := c.dial()
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	// Set timeouts on the connection
	if err := conn.SetDeadline(time.Now().Add(c.receiveTimeout)); err != nil {
		return fmt.Errorf("failed to set deadline: %w", err)
	}

	return commandFunc(conn)
}

// ReadFile sends CommandReadFile and returns the file contents
// Request: BYTE(1) + STRING(filename)
// Response: LONG(size) + BYTE[size](data)
func (c *Client) ReadFile(filename string) ([]byte, error) {
	var data []byte
	var readErr error

	err := c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandReadFile); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteString(conn, filename); err != nil {
			return fmt.Errorf("failed to write filename: %w", err)
		}

		// Read response - length first, then extend the deadline based
		// on the announced size before reading the (potentially large)
		// body itself, same reasoning as WriteFile.
		length, err := ReadDataLength(conn)
		if err != nil {
			return fmt.Errorf("failed to read file data length: %w", err)
		}
		if err := conn.SetDeadline(time.Now().Add(c.dataTimeout(length))); err != nil {
			return fmt.Errorf("failed to extend deadline for file data: %w", err)
		}
		fileData, err := ReadDataBody(conn, length)
		if err != nil {
			return fmt.Errorf("failed to read file data: %w", err)
		}

		data = fileData
		return nil
	})

	if err != nil {
		return nil, err
	}

	if readErr != nil {
		return nil, readErr
	}

	return data, nil
}

// FileExists sends CommandFileExists and returns whether the file exists
// Request: BYTE(3) + STRING(filename)
// Response: BOOL(exists)
func (c *Client) FileExists(filename string) (bool, error) {
	var exists bool

	err := c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandFileExists); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteString(conn, filename); err != nil {
			return fmt.Errorf("failed to write filename: %w", err)
		}

		// Read response
		result, err := ReadBool(conn)
		if err != nil {
			return fmt.Errorf("failed to read exists response: %w", err)
		}

		exists = result
		return nil
	})

	return exists, err
}

// GetDeviceInfo sends CommandInfo and returns device information
// Request: BYTE(9) + INT(protocol_version)
// Response: BOOL(licensed) + INT(versionCode) + STRING(certificate) + BOOL(flag)
func (c *Client) GetDeviceInfo() (*DeviceInfo, error) {
	var info *DeviceInfo

	err := c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandInfo); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteInt(conn, CurrentSyncVersion); err != nil {
			return fmt.Errorf("failed to write protocol version: %w", err)
		}

		// Read response
		licensed, err := ReadBool(conn)
		if err != nil {
			return fmt.Errorf("failed to read licensed flag: %w", err)
		}

		versionCode, err := ReadInt(conn)
		if err != nil {
			return fmt.Errorf("failed to read version code: %w", err)
		}

		certificate, err := ReadString(conn)
		if err != nil {
			return fmt.Errorf("failed to read certificate: %w", err)
		}

		additionalFlag, err := ReadBool(conn)
		if err != nil {
			return fmt.Errorf("failed to read additional flag: %w", err)
		}

		info = &DeviceInfo{
			Licensed:    licensed,
			VersionCode: int(versionCode),
			Certificate: certificate,
			Flag:        additionalFlag,
		}
		return nil
	})

	return info, err
}

// DeviceInfo contains information returned by CommandInfo
type DeviceInfo struct {
	Licensed    bool
	VersionCode int
	Certificate string
	Flag        bool
}

// SendStart sends CommandStart with a status message
// Request: BYTE(6) + STRING(message)
// Response: (none)
// If message is empty, defaults to "Start Synchronizing"
func (c *Client) SendStart(messages ...string) error {
	message := "Start Synchronizing"
	if len(messages) > 0 && messages[0] != "" {
		message = messages[0]
	}

	return c.execute(func(conn net.Conn) error {
		if err := WriteByte(conn, CommandStart); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteString(conn, message); err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}

		return nil
	})
}

// SendCompleted sends CommandCompleted to signal sync completion
// Request: BYTE(7) + STRING("Synchronization completed")
// Response: (none)
func (c *Client) SendCompleted() error {
	return c.execute(func(conn net.Conn) error {
		if err := WriteByte(conn, CommandCompleted); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteString(conn, "Synchronization completed"); err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}

		return nil
	})
}

// SendProgressUpdate sends CommandProgressUpdate with percentage
// Request: BYTE(8) + BYTE(percent 0-100)
// Response: (none)
func (c *Client) SendProgressUpdate(percent int) error {
	if percent < 0 || percent > 100 {
		return fmt.Errorf("invalid percent: %d (must be 0-100)", percent)
	}

	return c.execute(func(conn net.Conn) error {
		if err := WriteByte(conn, CommandProgressUpdate); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteByte(conn, byte(percent)); err != nil {
			return fmt.Errorf("failed to write percent: %w", err)
		}

		return nil
	})
}

// SendClientPong sends CommandClientPong to notify device that server is available
// This makes the sync button appear on the device.
// Request: BYTE(12)
// Response: (none)
func (c *Client) SendClientPong() error {
	return c.execute(func(conn net.Conn) error {
		if err := WriteByte(conn, CommandClientPong); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}
		return nil
	})
}

// ListFiles sends CommandListFiles and returns a newline-separated list of files
// Request: BYTE(0)
// Response: STRING(filelist)
func (c *Client) ListFiles() (string, error) {
	var fileList string

	err := c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandListFiles); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		// Read response
		files, err := ReadString(conn)
		if err != nil {
			return fmt.Errorf("failed to read file list: %w", err)
		}

		fileList = files
		return nil
	})

	return fileList, err
}

// WriteFile sends CommandWriteFile to write a file to the device
// Request: BYTE(5) + STRING(filename) + DATA(contents)
// Response: BOOL(success)
//
// The device stages an incoming write and only commits/renames it to the
// requested filename once it finishes processing the transfer - signaled
// by this BOOL. comic-server used to close the connection immediately
// after writing the last byte of data without reading anything back
// (matching ComicRackCE's WirelessSyncProvider.WriteFile only up through
// the data send, missing its trailing `if (!ReadBool(s)) throw new
// IOException();`). Closing the socket before the device sends that
// acknowledgment can race its finalize step, leaving the data stranded
// under an internal staging name instead of the real filename - found
// live 2026-08-28 via files named e.g. "sync4779799853534301468" instead
// of a real comic filename on a real device, with no comic body written
// under its real name since long before this entire debugging session
// (comic-server-*, see the writeFile-ack bd issue).
func (c *Client) WriteFile(filename string, data []byte) error {
	return c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandWriteFile); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteString(conn, filename); err != nil {
			return fmt.Errorf("failed to write filename: %w", err)
		}

		// Extend the deadline for the file body itself - see
		// dataTimeout. A real comic book can be tens to hundreds of MB;
		// the default receiveTimeout (sized for a command byte and a
		// filename) isn't remotely enough to move that much data, on any
		// WiFi, regardless of actual flakiness. The same extended
		// deadline covers the trailing BOOL read below too - the device
		// can't finalize the write until the whole body has arrived, so
		// the ack won't come back any faster than the transfer itself.
		if err := conn.SetDeadline(time.Now().Add(c.dataTimeout(int64(len(data))))); err != nil {
			return fmt.Errorf("failed to extend deadline for file data: %w", err)
		}
		if err := WriteData(conn, data); err != nil {
			return fmt.Errorf("failed to write file data: %w", err)
		}

		success, err := ReadBool(conn)
		if err != nil {
			return fmt.Errorf("failed to read write acknowledgment: %w", err)
		}
		if !success {
			return fmt.Errorf("device reported write failure for %q", filename)
		}

		return nil
	})
}

// DeleteFile sends CommandDeleteFile to delete a file from the device
// Request: BYTE(4) + STRING(filename)
// Response: BOOL(success)
//
// ComicRackCE's own client reads this and discards it (no error check) -
// matched here for the same reason: draining it before closing the
// connection, rather than leaving an unread byte on a socket the device
// may still be writing to when we close (see WriteFile's ack for the
// case where NOT reading this actually mattered).
func (c *Client) DeleteFile(filename string) error {
	return c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandDeleteFile); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteString(conn, filename); err != nil {
			return fmt.Errorf("failed to write filename: %w", err)
		}

		if _, err := ReadBool(conn); err != nil {
			return fmt.Errorf("failed to read delete acknowledgment: %w", err)
		}

		return nil
	})
}

// GetFreeSpace sends CommandFreeSpace and returns available storage in bytes
// Request: BYTE(2)
// Response: LONG(bytes)
func (c *Client) GetFreeSpace() (int64, error) {
	var freeSpace int64

	err := c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandFreeSpace); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		// Read response
		space, err := ReadLong(conn)
		if err != nil {
			return fmt.Errorf("failed to read free space: %w", err)
		}

		freeSpace = space
		return nil
	})

	return freeSpace, err
}

// ReadMultiFile sends CommandReadMultiFile and returns multiple file contents
// Request: BYTE(10) + STRING(filelist with newlines)
// Response: INT(count) + [STRING(filename) + DATA(contents)] repeated
func (c *Client) ReadMultiFile(filenames []string) (map[string][]byte, error) {
	files := make(map[string][]byte)

	err := c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandReadMultiFile); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		// Join filenames with newlines
		fileList := ""
		for i, filename := range filenames {
			if i > 0 {
				fileList += "\n"
			}
			fileList += filename
		}

		if err := WriteString(conn, fileList); err != nil {
			return fmt.Errorf("failed to write file list: %w", err)
		}

		// Read response - count of files
		count, err := ReadInt(conn)
		if err != nil {
			return fmt.Errorf("failed to read file count: %w", err)
		}

		// Read each file
		for i := int32(0); i < count; i++ {
			filename, err := ReadString(conn)
			if err != nil {
				return fmt.Errorf("failed to read filename %d: %w", i, err)
			}

			data, err := ReadData(conn)
			if err != nil {
				return fmt.Errorf("failed to read data for %s: %w", filename, err)
			}

			files[filename] = data
		}

		return nil
	})

	return files, err
}

// CheckAbort sends CommandCheckAbort to check if user aborted sync
// Request: BYTE(11)
// Response: BOOL(aborted)
func (c *Client) CheckAbort() (bool, error) {
	var aborted bool

	err := c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandCheckAbort); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		// Read response
		result, err := ReadBool(conn)
		if err != nil {
			return fmt.Errorf("failed to read abort status: %w", err)
		}

		aborted = result
		return nil
	})

	return aborted, err
}
