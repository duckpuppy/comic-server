package protocol

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "regular error",
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name:     "os error",
			err:      os.ErrNotExist,
			expected: false,
		},
		{
			name:     "timeout error",
			err:      &timeoutError{},
			expected: true,
		},
		{
			name:     "connection refused",
			err:      &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED},
			expected: true,
		},
		{
			name:     "connection reset",
			err:      &net.OpError{Op: "read", Err: syscall.ECONNRESET},
			expected: true,
		},
		{
			name:     "broken pipe",
			err:      &net.OpError{Op: "write", Err: syscall.EPIPE},
			expected: true,
		},
		{
			name:     "connection closed",
			err:      net.ErrClosed,
			expected: true,
		},
		{
			name:     "host unreachable",
			err:      &net.OpError{Op: "dial", Err: syscall.EHOSTUNREACH},
			expected: true,
		},
		{
			name:     "network unreachable",
			err:      &net.OpError{Op: "dial", Err: syscall.ENETUNREACH},
			expected: true,
		},
		{
			name:     "wrapped timeout error",
			err:      fmt.Errorf("context error: %w", &timeoutError{}),
			expected: true,
		},
		{
			name:     "wrapped connection refused",
			err:      fmt.Errorf("failed to connect: %w", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNetworkError(tt.err)
			if got != tt.expected {
				t.Errorf("IsNetworkError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestErrorTypeString(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "no error",
		},
		{
			name:     "regular error",
			err:      errors.New("some error"),
			expected: "general error",
		},
		{
			name:     "timeout error",
			err:      &timeoutError{},
			expected: "connection timeout",
		},
		{
			name:     "connection refused",
			err:      &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED},
			expected: "connection refused",
		},
		{
			name:     "connection reset",
			err:      &net.OpError{Op: "read", Err: syscall.ECONNRESET},
			expected: "connection reset by peer",
		},
		{
			name:     "broken pipe",
			err:      &net.OpError{Op: "write", Err: syscall.EPIPE},
			expected: "broken pipe",
		},
		{
			name:     "host unreachable",
			err:      &net.OpError{Op: "dial", Err: syscall.EHOSTUNREACH},
			expected: "host unreachable",
		},
		{
			name:     "network unreachable",
			err:      &net.OpError{Op: "dial", Err: syscall.ENETUNREACH},
			expected: "network unreachable",
		},
		{
			name:     "temporary error",
			err:      &temporaryError{},
			expected: "temporary network error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ErrorTypeString(tt.err)
			if got != tt.expected {
				t.Errorf("ErrorTypeString(%v) = %q, want %q", tt.err, got, tt.expected)
			}
		})
	}
}

// Mock error types for testing

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return false }

type temporaryError struct{}

func (e *temporaryError) Error() string   { return "temporary" }
func (e *temporaryError) Timeout() bool   { return false }
func (e *temporaryError) Temporary() bool { return true }

// TestRealNetworkError tests with actual network conditions
func TestRealNetworkError(t *testing.T) {
	// Try to connect to a non-existent local port
	conn, err := net.DialTimeout("tcp", "127.0.0.1:1", 100*time.Millisecond)
	if conn != nil {
		conn.Close()
	}

	if err == nil {
		t.Skip("Expected connection to fail, but it succeeded")
	}

	if !IsNetworkError(err) {
		t.Errorf("Real connection error should be detected as network error: %v", err)
	}

	errorType := ErrorTypeString(err)
	if errorType == "general error" {
		t.Errorf("Expected specific error type, got 'general error' for: %v", err)
	}

	t.Logf("Error type: %s", errorType)
}

// TestTimeoutError tests with actual timeout
func TestTimeoutError(t *testing.T) {
	// Create a listener but don't accept connections
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// Try to connect with very short timeout
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 1*time.Nanosecond)
	if conn != nil {
		conn.Close()
	}

	// On fast systems, connection might succeed, so we can't assert error
	if err != nil {
		if !IsNetworkError(err) {
			t.Errorf("Timeout error should be detected as network error: %v", err)
		}
		t.Logf("Error type: %s", ErrorTypeString(err))
	}
}

// TestIsConnectionRefused_RealDial exercises a genuine ECONNREFUSED from
// the OS (dialing a port nothing is listening on), the same shape
// comic-server-134's fix distinguishes from other network failures.
func TestIsConnectionRefused_RealDial(t *testing.T) {
	// Listen then immediately close, freeing a port the OS is very
	// unlikely to have anything else bound to before the dial below - a
	// closed TCP port on localhost reliably refuses rather than timing
	// out or being silently dropped, unlike a bare fixed port number.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate a port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Skip("expected dial to a closed port to fail, but it succeeded")
	}

	if !IsConnectionRefused(err) {
		t.Errorf("expected IsConnectionRefused(%v) = true", err)
	}
	if ErrorTypeString(err) != "connection refused" {
		t.Errorf("ErrorTypeString(%v) = %q, want \"connection refused\"", err, ErrorTypeString(err))
	}
}

func TestIsConnectionRefused_OtherErrorsAreFalse(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"plain text mentioning refused", fmt.Errorf("connection refused")},
		{"timeout", &timeoutError{}},
		{"reset", &net.OpError{Op: "read", Net: "tcp", Err: &os.SyscallError{Syscall: "read", Err: syscall.ECONNRESET}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsConnectionRefused(tt.err) {
				t.Errorf("IsConnectionRefused(%v) = true, want false", tt.err)
			}
		})
	}
}
