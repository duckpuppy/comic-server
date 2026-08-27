package protocol_test

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/protocol"
)

func TestClientServerIntegration(t *testing.T) {
	// Start mock device server
	server, err := protocol.NewMockDeviceServer(0) // Use any available port
	if err != nil {
		t.Fatalf("Failed to create mock server: %v", err)
	}
	defer server.Stop()

	server.Start()
	time.Sleep(50 * time.Millisecond) // Give server time to start

	// Create client
	client := protocol.NewClient("127.0.0.1", server.Port())

	t.Run("ReadFile", func(t *testing.T) {
		// Add test file to server
		testContent := []byte("test file content")
		server.AddFile("test.txt", testContent)

		// Read file via client
		data, err := client.ReadFile("test.txt")
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}

		if string(data) != string(testContent) {
			t.Errorf("Expected %q, got %q", string(testContent), string(data))
		}
	})

	t.Run("ReadNonExistentFile", func(t *testing.T) {
		// Read non-existent file
		data, err := client.ReadFile("nonexistent.txt")
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}

		// Should return empty data for non-existent files
		if len(data) != 0 {
			t.Errorf("Expected empty data, got %d bytes", len(data))
		}
	})

	t.Run("FileExists", func(t *testing.T) {
		server.AddFile("exists.txt", []byte("content"))

		exists, err := client.FileExists("exists.txt")
		if err != nil {
			t.Fatalf("Failed to check file existence: %v", err)
		}

		if !exists {
			t.Error("Expected file to exist")
		}

		exists, err = client.FileExists("notexists.txt")
		if err != nil {
			t.Fatalf("Failed to check file existence: %v", err)
		}

		if exists {
			t.Error("Expected file to not exist")
		}
	})

	t.Run("GetDeviceInfo", func(t *testing.T) {
		info, err := client.GetDeviceInfo()
		if err != nil {
			t.Fatalf("Failed to get device info: %v", err)
		}

		// Mock server returns test values
		if !info.Licensed {
			t.Error("Expected licensed=true")
		}
		if info.VersionCode != 123 {
			t.Errorf("Expected version code 123, got %d", info.VersionCode)
		}
		if info.Certificate != "MockCertificate" {
			t.Errorf("Expected 'MockCertificate', got %q", info.Certificate)
		}
	})

	t.Run("LargeFileTransfer", func(t *testing.T) {
		// Test with a larger file (1MB)
		largeContent := make([]byte, 1024*1024)
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}

		server.AddFile("large.bin", largeContent)

		data, err := client.ReadFile("large.bin")
		if err != nil {
			t.Fatalf("Failed to read large file: %v", err)
		}

		if len(data) != len(largeContent) {
			t.Errorf("Expected %d bytes, got %d", len(largeContent), len(data))
		}

		// Verify content
		for i := range data {
			if data[i] != largeContent[i] {
				t.Errorf("Content mismatch at byte %d: expected %d, got %d", i, largeContent[i], data[i])
				break
			}
		}
	})
}

func TestClientConnectionFailure(t *testing.T) {
	// Create client to non-existent server
	client := protocol.NewClient("127.0.0.1", 65000) // Unlikely to be in use
	client.SetTimeouts(100*time.Millisecond, 100*time.Millisecond, 100*time.Millisecond)
	client.SetRetries(0)

	_, err := client.ReadFile("test.txt")
	if err == nil {
		t.Error("Expected connection error, got nil")
	}
}

func TestClientRetries(t *testing.T) {
	// Test that client retries on connection failure
	client := protocol.NewClient("127.0.0.1", 65001)
	client.SetTimeouts(50*time.Millisecond, 50*time.Millisecond, 50*time.Millisecond)
	client.SetRetries(2)

	start := time.Now()
	_, err := client.ReadFile("test.txt")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected connection error, got nil")
	}

	// Should have tried at least 3 times (initial + 2 retries)
	// With 50ms timeout each, should take at least 150ms
	if elapsed < 100*time.Millisecond {
		t.Errorf("Expected retries to take at least 100ms, took %v", elapsed)
	}
}

// TestClientRetriesFullOperationNotJustConnect is the regression test for
// a gap found 2026-08-27 comparing against ComicRackCE's own retry
// behavior: the client only retried the initial TCP connect, never an
// operation that failed AFTER connecting successfully (e.g. a write that
// drops mid-transfer on flaky WiFi) - that's exactly the failure mode
// unstable WiFi produces, and ComicRackCE's own client recovers from it
// by retrying the whole operation with a fresh connection.
//
// This listener accepts every connection (so dial always succeeds) but
// closes it immediately without responding, forcing the command itself
// to fail with a read error - and counts how many separate connections
// it received. A client that only retried the connect phase would open
// exactly one connection and give up; one that retries the whole
// operation should open several.
func TestClientRetriesFullOperationNotJustConnect(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	var connectionCount int64
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // listener closed
			}
			atomic.AddInt64(&connectionCount, 1)
			conn.Close() // drop immediately, before any response
		}
	}()

	addr := listener.Addr().(*net.TCPAddr)
	client := protocol.NewClient("127.0.0.1", addr.Port)
	client.SetTimeouts(200*time.Millisecond, 200*time.Millisecond, 200*time.Millisecond)
	client.SetRetries(2)

	_, err = client.ReadFile("test.txt")
	if err == nil {
		t.Error("expected an error since the server never responds")
	}

	got := atomic.LoadInt64(&connectionCount)
	if got < 2 {
		t.Errorf("expected at least 2 separate connection attempts (initial + retry after the command itself failed), got %d", got)
	}
}
