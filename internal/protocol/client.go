package protocol

import (
	"fmt"
	"net"
	"time"
)

const (
	// Default timeout values from EngineConfiguration (from WIRELESS_SYNC_PROTOCOL.md)
	DefaultReceiveTimeout    = 5000 * time.Millisecond // 5 seconds
	DefaultSendTimeout       = 5000 * time.Millisecond // 5 seconds
	DefaultConnectionTimeout = 2500 * time.Millisecond // 2.5 seconds
	DefaultRetries           = 1
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

// connect establishes a TCP connection to the device with retries
func (c *Client) connect() (net.Conn, error) {
	var conn net.Conn
	var err error

	for attempt := 0; attempt <= c.retries; attempt++ {
		address := fmt.Sprintf("%s:%d", c.deviceIP, c.devicePort)
		conn, err = net.DialTimeout("tcp", address, c.connectionTimeout)
		if err == nil {
			return conn, nil
		}

		// If this isn't the last attempt, wait a bit before retrying
		if attempt < c.retries {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil, fmt.Errorf("failed to connect after %d retries: %w", c.retries, err)
}

// execute performs a command with automatic connection management and timeouts
func (c *Client) execute(commandFunc func(conn net.Conn) error) error {
	conn, err := c.connect()
	if err != nil {
		return err
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

		// Read response
		fileData, err := ReadData(conn)
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

// SendStart sends CommandStart to begin synchronization
// Request: BYTE(6) + STRING("Start Synchronizing")
// Response: (none)
func (c *Client) SendStart() error {
	return c.execute(func(conn net.Conn) error {
		if err := WriteByte(conn, CommandStart); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteString(conn, "Start Synchronizing"); err != nil {
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
// Response: (none)
func (c *Client) WriteFile(filename string, data []byte) error {
	return c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandWriteFile); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteString(conn, filename); err != nil {
			return fmt.Errorf("failed to write filename: %w", err)
		}

		if err := WriteData(conn, data); err != nil {
			return fmt.Errorf("failed to write file data: %w", err)
		}

		return nil
	})
}

// DeleteFile sends CommandDeleteFile to delete a file from the device
// Request: BYTE(4) + STRING(filename)
// Response: (none)
func (c *Client) DeleteFile(filename string) error {
	return c.execute(func(conn net.Conn) error {
		// Send command
		if err := WriteByte(conn, CommandDeleteFile); err != nil {
			return fmt.Errorf("failed to write command: %w", err)
		}

		if err := WriteString(conn, filename); err != nil {
			return fmt.Errorf("failed to write filename: %w", err)
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
