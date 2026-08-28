package protocol

import (
	"errors"
	"net"
	"os"
	"syscall"
)

// IsNetworkError returns true if the error is a network-related error
// indicating connection issues, timeouts, or disconnections.
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for common network error types
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true // Includes timeout, temporary errors
	}

	// Check for EOF (connection closed)
	if errors.Is(err, net.ErrClosed) {
		return true
	}

	// Check for syscall errors
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		// Check for specific syscall errors
		var sysErr syscall.Errno
		if errors.As(opErr.Err, &sysErr) {
			switch sysErr {
			case syscall.ECONNREFUSED,
				syscall.ECONNRESET,
				syscall.ECONNABORTED,
				syscall.EPIPE,
				syscall.ETIMEDOUT,
				syscall.EHOSTUNREACH,
				syscall.ENETUNREACH:
				return true
			}
		}

		// Check for other common OpError conditions
		if opErr.Timeout() {
			return true
		}
	}

	// Check for PathError (file operations on closed connections)
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return IsNetworkError(pathErr.Err)
	}

	return false
}

// IsConnectionRefused returns true if err is specifically a "connection
// refused" - the device has no process listening on the port yet (or no
// longer does), as opposed to other network failures (timeout, reset,
// unreachable). Callers use this to distinguish "the device's TCP
// listener just isn't up yet" (worth a longer grace period - the device
// is expected to start listening any second) from other failure shapes
// that don't carry that same "give it a bit more time" implication.
func IsConnectionRefused(err error) bool {
	if err == nil {
		return false
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr syscall.Errno
		if errors.As(opErr.Err, &sysErr) {
			return sysErr == syscall.ECONNREFUSED
		}
	}

	return false
}

// ErrorTypeString returns a human-readable description of the error type
func ErrorTypeString(err error) string {
	if err == nil {
		return "no error"
	}

	if IsNetworkError(err) {
		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() {
				return "connection timeout"
			}
			if netErr.Temporary() {
				return "temporary network error"
			}
		}

		var opErr *net.OpError
		if errors.As(err, &opErr) {
			var sysErr syscall.Errno
			if errors.As(opErr.Err, &sysErr) {
				switch sysErr {
				case syscall.ECONNREFUSED:
					return "connection refused"
				case syscall.ECONNRESET:
					return "connection reset by peer"
				case syscall.ECONNABORTED:
					return "connection aborted"
				case syscall.EPIPE:
					return "broken pipe"
				case syscall.ETIMEDOUT:
					return "connection timed out"
				case syscall.EHOSTUNREACH:
					return "host unreachable"
				case syscall.ENETUNREACH:
					return "network unreachable"
				}
			}
		}

		return "network error"
	}

	return "general error"
}
