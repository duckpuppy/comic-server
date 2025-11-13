package sync

import (
	"fmt"
	"sync"

	"github.com/duckpuppy/comic-server/internal/protocol"
)

// MockClient is a mock implementation of protocol.Client for testing
type MockClient struct {
	mu sync.Mutex

	// File system simulation
	files map[string][]byte

	// Command call tracking
	ReadFileCalls      []string
	WriteFileCalls     []WriteFileCall
	DeleteFileCalls    []string
	FileExistsCalls    []string
	ListFilesCalls     int
	ReadMultiFileCalls [][]string
	GetDeviceInfoCalls int
	SendStartCalls     int
	SendCompletedCalls int
	SendProgressCalls  []int
	GetFreeSpaceCalls  int
	CheckAbortCalls    int

	// Configurable responses
	DeviceInfo       *protocol.DeviceInfo
	FreeSpace        int64
	Aborted          bool
	ListFilesResult  string
	ReadFileError    error
	WriteFileError   error
	DeleteFileError  error
	FileExistsError  error
	ListFilesError   error
	GetInfoError     error
	SendStartError   error
	SendCompletedErr error
	ProgressError    error
	FreeSpaceError   error
	CheckAbortError  error
	ReadMultiError   error

	// Custom behavior callbacks
	CheckAbortFunc func() (bool, error)
}

// WriteFileCall tracks a WriteFile call
type WriteFileCall struct {
	Filename string
	Data     []byte
}

// NewMockClient creates a new mock client with default configuration
func NewMockClient() *MockClient {
	return &MockClient{
		files: make(map[string][]byte),
		DeviceInfo: &protocol.DeviceInfo{
			Licensed:    true,
			VersionCode: 100,
			Certificate: "test-cert",
			Flag:        true,
		},
		FreeSpace: 1024 * 1024 * 1024, // 1GB default
		Aborted:   false,
	}
}

// ReadFile simulates reading a file from the device
func (m *MockClient) ReadFile(filename string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ReadFileCalls = append(m.ReadFileCalls, filename)

	if m.ReadFileError != nil {
		return nil, m.ReadFileError
	}

	data, ok := m.files[filename]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", filename)
	}

	return data, nil
}

// WriteFile simulates writing a file to the device
func (m *MockClient) WriteFile(filename string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.WriteFileCalls = append(m.WriteFileCalls, WriteFileCall{
		Filename: filename,
		Data:     data,
	})

	if m.WriteFileError != nil {
		return m.WriteFileError
	}

	m.files[filename] = data
	return nil
}

// DeleteFile simulates deleting a file from the device
func (m *MockClient) DeleteFile(filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteFileCalls = append(m.DeleteFileCalls, filename)

	if m.DeleteFileError != nil {
		return m.DeleteFileError
	}

	delete(m.files, filename)
	return nil
}

// FileExists simulates checking if a file exists on the device
func (m *MockClient) FileExists(filename string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.FileExistsCalls = append(m.FileExistsCalls, filename)

	if m.FileExistsError != nil {
		return false, m.FileExistsError
	}

	_, exists := m.files[filename]
	return exists, nil
}

// ListFiles simulates listing files on the device
func (m *MockClient) ListFiles() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ListFilesCalls++

	if m.ListFilesError != nil {
		return "", m.ListFilesError
	}

	if m.ListFilesResult != "" {
		return m.ListFilesResult, nil
	}

	// Generate list from files map
	result := ""
	for filename := range m.files {
		if result != "" {
			result += "\n"
		}
		result += filename
	}

	return result, nil
}

// ReadMultiFile simulates reading multiple files from the device
func (m *MockClient) ReadMultiFile(filenames []string) (map[string][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ReadMultiFileCalls = append(m.ReadMultiFileCalls, filenames)

	if m.ReadMultiError != nil {
		return nil, m.ReadMultiError
	}

	result := make(map[string][]byte)
	for _, filename := range filenames {
		if data, ok := m.files[filename]; ok {
			result[filename] = data
		}
	}

	return result, nil
}

// GetDeviceInfo simulates getting device information
func (m *MockClient) GetDeviceInfo() (*protocol.DeviceInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetDeviceInfoCalls++

	if m.GetInfoError != nil {
		return nil, m.GetInfoError
	}

	return m.DeviceInfo, nil
}

// SendStart simulates sending sync start command
func (m *MockClient) SendStart(message ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SendStartCalls++

	if m.SendStartError != nil {
		return m.SendStartError
	}

	return nil
}

// SendCompleted simulates sending sync completed command
func (m *MockClient) SendCompleted() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SendCompletedCalls++

	if m.SendCompletedErr != nil {
		return m.SendCompletedErr
	}

	return nil
}

// SendProgressUpdate simulates sending progress update
func (m *MockClient) SendProgressUpdate(percent int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SendProgressCalls = append(m.SendProgressCalls, percent)

	if m.ProgressError != nil {
		return m.ProgressError
	}

	return nil
}

// GetFreeSpace simulates getting device free space
func (m *MockClient) GetFreeSpace() (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetFreeSpaceCalls++

	if m.FreeSpaceError != nil {
		return 0, m.FreeSpaceError
	}

	return m.FreeSpace, nil
}

// CheckAbort simulates checking if sync should abort
func (m *MockClient) CheckAbort() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CheckAbortCalls++

	if m.CheckAbortFunc != nil {
		// Unlock before calling custom function to avoid deadlock
		m.mu.Unlock()
		result, err := m.CheckAbortFunc()
		m.mu.Lock()
		return result, err
	}

	if m.CheckAbortError != nil {
		return false, m.CheckAbortError
	}

	return m.Aborted, nil
}

// AddFile adds a file to the mock device's file system
func (m *MockClient) AddFile(filename string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[filename] = data
}

// GetFile retrieves a file from the mock device's file system
func (m *MockClient) GetFile(filename string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[filename]
	return data, ok
}

// Reset clears all call tracking and resets to default state
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.files = make(map[string][]byte)
	m.ReadFileCalls = nil
	m.WriteFileCalls = nil
	m.DeleteFileCalls = nil
	m.FileExistsCalls = nil
	m.ListFilesCalls = 0
	m.ReadMultiFileCalls = nil
	m.GetDeviceInfoCalls = 0
	m.SendStartCalls = 0
	m.SendCompletedCalls = 0
	m.SendProgressCalls = nil
	m.GetFreeSpaceCalls = 0
	m.CheckAbortCalls = 0
}
