package protocol

import (
	"fmt"
	"io"
	"net"
	"sync"
)

// MockDeviceServer simulates a ComicRack device for testing
// It listens on TCP port and responds to protocol commands
type MockDeviceServer struct {
	listener net.Listener
	port     int
	files    map[string][]byte // In-memory file system
	mu       sync.RWMutex
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewMockDeviceServer creates a new mock device server
// If port is 0, an available port will be assigned
func NewMockDeviceServer(port int) (*MockDeviceServer, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to start listener: %w", err)
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port

	return &MockDeviceServer{
		listener: listener,
		port:     actualPort,
		files:    make(map[string][]byte),
		stopChan: make(chan struct{}),
	}, nil
}

// Port returns the port the server is listening on
func (m *MockDeviceServer) Port() int {
	return m.port
}

// AddFile adds a file to the mock device's file system
func (m *MockDeviceServer) AddFile(filename string, content []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[filename] = content
}

// Start begins accepting connections
func (m *MockDeviceServer) Start() {
	m.wg.Add(1)
	go m.acceptLoop()
}

// Stop stops the server
func (m *MockDeviceServer) Stop() error {
	close(m.stopChan)
	err := m.listener.Close()
	m.wg.Wait()
	return err
}

// acceptLoop accepts and handles incoming connections
func (m *MockDeviceServer) acceptLoop() {
	defer m.wg.Done()

	for {
		conn, err := m.listener.Accept()
		if err != nil {
			// Check if we're shutting down
			select {
			case <-m.stopChan:
				return
			default:
				continue
			}
		}

		m.wg.Add(1)
		go m.handleConnection(conn)
	}
}

// handleConnection handles a single client connection
func (m *MockDeviceServer) handleConnection(conn net.Conn) {
	defer m.wg.Done()
	defer conn.Close()

	for {
		// Read command byte
		cmd, err := ReadByte(conn)
		if err != nil {
			if err != io.EOF {
				// Log error if needed
			}
			return
		}

		switch cmd {
		case CommandReadFile:
			m.handleReadFile(conn)
		case CommandFileExists:
			m.handleFileExists(conn)
		case CommandInfo:
			m.handleGetDeviceInfo(conn)
		default:
			// Unknown command, close connection
			return
		}
	}
}

// handleReadFile handles CommandReadFile
// Request: STRING(filename)
// Response: LONG(size) + BYTE[size](data) or error
func (m *MockDeviceServer) handleReadFile(conn net.Conn) {
	filename, err := ReadString(conn)
	if err != nil {
		return
	}

	m.mu.RLock()
	data, exists := m.files[filename]
	m.mu.RUnlock()

	if !exists {
		// File not found - send empty data
		WriteLong(conn, 0)
		return
	}

	// Send file size and data
	WriteLong(conn, int64(len(data)))
	conn.Write(data)
}

// handleFileExists handles CommandFileExists
// Request: STRING(filename)
// Response: BOOL(exists)
func (m *MockDeviceServer) handleFileExists(conn net.Conn) {
	filename, err := ReadString(conn)
	if err != nil {
		WriteBool(conn, false)
		return
	}

	m.mu.RLock()
	_, exists := m.files[filename]
	m.mu.RUnlock()

	WriteBool(conn, exists)
}

// handleGetDeviceInfo handles CommandInfo
// Response: BOOL(licensed) + INT(versionCode) + STRING(certificate) + BOOL(flag)
func (m *MockDeviceServer) handleGetDeviceInfo(conn net.Conn) {
	// Read protocol version first
	_, err := ReadInt(conn)
	if err != nil {
		return
	}

	// Send mock device info
	WriteBool(conn, true)                     // licensed
	WriteInt(conn, 123)                       // version code
	WriteString(conn, "MockCertificate")      // certificate
	WriteBool(conn, false)                    // additional flag
}
