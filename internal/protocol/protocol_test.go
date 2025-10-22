package protocol

import (
	"bytes"
	"testing"
)

func TestByte(t *testing.T) {
	tests := []byte{0x00, 0x01, 0xFF, 0x42}

	for _, expected := range tests {
		var buf bytes.Buffer

		if err := WriteByte(&buf, expected); err != nil {
			t.Fatalf("WriteByte failed: %v", err)
		}

		got, err := ReadByte(&buf)
		if err != nil {
			t.Fatalf("ReadByte failed: %v", err)
		}

		if got != expected {
			t.Errorf("Byte mismatch: got %02x, want %02x", got, expected)
		}
	}
}

func TestBool(t *testing.T) {
	tests := []bool{true, false}

	for _, expected := range tests {
		var buf bytes.Buffer

		if err := WriteBool(&buf, expected); err != nil {
			t.Fatalf("WriteBool failed: %v", err)
		}

		got, err := ReadBool(&buf)
		if err != nil {
			t.Fatalf("ReadBool failed: %v", err)
		}

		if got != expected {
			t.Errorf("Bool mismatch: got %v, want %v", got, expected)
		}
	}
}

func TestInt(t *testing.T) {
	tests := []int32{0, 1, -1, 123, 7620, 2147483647, -2147483648}

	for _, expected := range tests {
		var buf bytes.Buffer

		if err := WriteInt(&buf, expected); err != nil {
			t.Fatalf("WriteInt failed: %v", err)
		}

		got, err := ReadInt(&buf)
		if err != nil {
			t.Fatalf("ReadInt failed: %v", err)
		}

		if got != expected {
			t.Errorf("Int mismatch: got %d, want %d", got, expected)
		}
	}
}

func TestIntBigEndian(t *testing.T) {
	// Test that 123 is encoded as big-endian
	var buf bytes.Buffer
	if err := WriteInt(&buf, 123); err != nil {
		t.Fatalf("WriteInt failed: %v", err)
	}

	bytes := buf.Bytes()
	expected := []byte{0x00, 0x00, 0x00, 0x7B}

	if !bytesEqual(bytes, expected) {
		t.Errorf("Int big-endian encoding incorrect: got %v, want %v", bytes, expected)
	}
}

func TestLong(t *testing.T) {
	tests := []int64{0, 1, -1, 1000000000, 9223372036854775807, -9223372036854775808}

	for _, expected := range tests {
		var buf bytes.Buffer

		if err := WriteLong(&buf, expected); err != nil {
			t.Fatalf("WriteLong failed: %v", err)
		}

		got, err := ReadLong(&buf)
		if err != nil {
			t.Fatalf("ReadLong failed: %v", err)
		}

		if got != expected {
			t.Errorf("Long mismatch: got %d, want %d", got, expected)
		}
	}
}

func TestLongBigEndian(t *testing.T) {
	// Test that 570 is encoded as big-endian (from protocol spec example)
	var buf bytes.Buffer
	if err := WriteLong(&buf, 570); err != nil {
		t.Fatalf("WriteLong failed: %v", err)
	}

	bytes := buf.Bytes()
	expected := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x3A}

	if !bytesEqual(bytes, expected) {
		t.Errorf("Long big-endian encoding incorrect: got %v, want %v", bytes, expected)
	}
}

func TestString(t *testing.T) {
	tests := []string{
		"",
		"test.cbp",
		"Batman #1.cbp",
		"comic.cbp.xml",
		"Start Synchronizing",
		"你好", // UTF-8 multibyte characters
	}

	for _, expected := range tests {
		var buf bytes.Buffer

		if err := WriteString(&buf, expected); err != nil {
			t.Fatalf("WriteString failed for %q: %v", expected, err)
		}

		got, err := ReadString(&buf)
		if err != nil {
			t.Fatalf("ReadString failed for %q: %v", expected, err)
		}

		if got != expected {
			t.Errorf("String mismatch: got %q, want %q", got, expected)
		}
	}
}

func TestStringFormat(t *testing.T) {
	// Test that "test.cbp" is encoded as INT(length) + UTF-8 bytes
	var buf bytes.Buffer
	if err := WriteString(&buf, "test.cbp"); err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}

	bytes := buf.Bytes()
	// Should be: 0x00000008 (length 8) + "test.cbp"
	expected := []byte{
		0x00, 0x00, 0x00, 0x08, // INT(8)
		0x74, 0x65, 0x73, 0x74, 0x2E, 0x63, 0x62, 0x70, // "test.cbp"
	}

	if !bytesEqual(bytes, expected) {
		t.Errorf("String format incorrect: got %v, want %v", bytes, expected)
	}
}

func TestData(t *testing.T) {
	tests := [][]byte{
		{},
		{0x00},
		{0x48, 0x65, 0x6C, 0x6C, 0x6F}, // "Hello"
		make([]byte, 1000),              // Large data
	}

	for _, expected := range tests {
		var buf bytes.Buffer

		if err := WriteData(&buf, expected); err != nil {
			t.Fatalf("WriteData failed: %v", err)
		}

		got, err := ReadData(&buf)
		if err != nil {
			t.Fatalf("ReadData failed: %v", err)
		}

		if !bytesEqual(got, expected) {
			t.Errorf("Data mismatch: got %d bytes, want %d bytes", len(got), len(expected))
		}
	}
}

func TestDataFormat(t *testing.T) {
	// Test that 5 bytes "Hello" is encoded as LONG(5) + data
	var buf bytes.Buffer
	data := []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F}
	if err := WriteData(&buf, data); err != nil {
		t.Fatalf("WriteData failed: %v", err)
	}

	bytes := buf.Bytes()
	// Should be: 0x0000000000000005 (LONG 5) + "Hello"
	expected := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05, // LONG(5)
		0x48, 0x65, 0x6C, 0x6C, 0x6F, // "Hello"
	}

	if !bytesEqual(bytes, expected) {
		t.Errorf("Data format incorrect: got %v, want %v", bytes, expected)
	}
}

// Helper function to compare byte slices
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
