package protocol

import (
	"bytes"
	"testing"
)

// TestReadStringMaxLengthValidation tests that ReadString rejects strings exceeding MaxStringLength
func TestReadStringMaxLengthValidation(t *testing.T) {
	tests := []struct {
		name        string
		length      int32
		expectError bool
	}{
		{
			name:        "valid small string",
			length:      100,
			expectError: false,
		},
		{
			name:        "valid max string",
			length:      MaxStringLength,
			expectError: false,
		},
		{
			name:        "invalid - exceeds max",
			length:      MaxStringLength + 1,
			expectError: true,
		},
		{
			name:        "invalid - negative length",
			length:      -1,
			expectError: true,
		},
		{
			name:        "invalid - large attack",
			length:      1000000000, // 1GB - would cause memory exhaustion
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer with INT(length) using our WriteInt function
			buf := new(bytes.Buffer)
			if err := WriteInt(buf, tt.length); err != nil {
				t.Fatalf("failed to write length: %v", err)
			}

			// Append some data if length is positive and within max limit
			// For valid cases, we need to provide the actual data
			if tt.length > 0 && tt.length <= MaxStringLength {
				buf.Write(make([]byte, tt.length))
			}

			// Try to read the string
			_, err := ReadString(buf)

			if tt.expectError && err == nil {
				t.Errorf("expected error for length %d, got nil", tt.length)
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error for length %d: %v", tt.length, err)
			}
		})
	}
}

// TestReadDataMaxLengthValidation tests that ReadData rejects data exceeding MaxDataLength
func TestReadDataMaxLengthValidation(t *testing.T) {
	tests := []struct {
		name        string
		length      int64
		expectError bool
	}{
		{
			name:        "valid small data",
			length:      1000,
			expectError: false,
		},
		{
			name:        "valid 10MB data",
			length:      10 * 1024 * 1024,
			expectError: false,
		},
		{
			name:        "valid max data",
			length:      MaxDataLength,
			expectError: false,
		},
		{
			name:        "invalid - exceeds max",
			length:      MaxDataLength + 1,
			expectError: true,
		},
		{
			name:        "invalid - negative length",
			length:      -1,
			expectError: true,
		},
		{
			name:        "invalid - extreme attack",
			length:      1000000000000, // 1TB - would cause memory exhaustion
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer with LONG(length) using our WriteLong function
			buf := new(bytes.Buffer)
			if err := WriteLong(buf, tt.length); err != nil {
				t.Fatalf("failed to write length: %v", err)
			}

			// Append some data if length is positive and reasonable (< 100MB for test performance)
			if tt.length > 0 && tt.length <= 100*1024*1024 {
				// For large valid lengths, we need to actually provide the data
				// or the read will fail with EOF
				data := make([]byte, tt.length)
				buf.Write(data)
			}

			// Try to read the data
			_, err := ReadData(buf)

			if tt.expectError && err == nil {
				t.Errorf("expected error for length %d, got nil", tt.length)
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error for length %d: %v", tt.length, err)
			}
		})
	}
}

// TestReadStringNegativeLengthRejection specifically tests negative length handling
func TestReadStringNegativeLengthRejection(t *testing.T) {
	// Create buffer with negative length
	buf := new(bytes.Buffer)
	if err := WriteInt(buf, -100); err != nil {
		t.Fatalf("failed to write test data: %v", err)
	}

	// Should reject negative length
	_, err := ReadString(buf)
	if err == nil {
		t.Error("expected error for negative string length, got nil")
	}
	if err != nil && !bytes.Contains([]byte(err.Error()), []byte("negative")) {
		t.Errorf("expected error message to mention 'negative', got: %v", err)
	}
}

// TestReadDataNegativeLengthRejection specifically tests negative length handling
func TestReadDataNegativeLengthRejection(t *testing.T) {
	// Create buffer with negative length
	buf := new(bytes.Buffer)
	if err := WriteLong(buf, -100); err != nil {
		t.Fatalf("failed to write test data: %v", err)
	}

	// Should reject negative length
	_, err := ReadData(buf)
	if err == nil {
		t.Error("expected error for negative data length, got nil")
	}
	if err != nil && !bytes.Contains([]byte(err.Error()), []byte("negative")) {
		t.Errorf("expected error message to mention 'negative', got: %v", err)
	}
}

// TestSecurityLimitsConstants verifies the security limit constants are reasonable
func TestSecurityLimitsConstants(t *testing.T) {
	// MaxStringLength should be reasonable (1MB)
	if MaxStringLength != 1*1024*1024 {
		t.Errorf("MaxStringLength should be 1MB, got %d", MaxStringLength)
	}

	// MaxDataLength should allow large comics but prevent DoS (100MB)
	if MaxDataLength != 100*1024*1024 {
		t.Errorf("MaxDataLength should be 100MB, got %d", MaxDataLength)
	}

	// MaxDataLength should be larger than MaxStringLength
	if MaxDataLength <= MaxStringLength {
		t.Error("MaxDataLength should be larger than MaxStringLength")
	}
}
