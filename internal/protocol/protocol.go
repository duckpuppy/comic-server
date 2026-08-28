package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Protocol command bytes (from WIRELESS_SYNC_PROTOCOL.md)
const (
	CommandListFiles       byte = 0
	CommandReadFile        byte = 1
	CommandFreeSpace       byte = 2
	CommandFileExists      byte = 3
	CommandDeleteFile      byte = 4
	CommandWriteFile       byte = 5
	CommandStart           byte = 6
	CommandCompleted       byte = 7
	CommandProgressUpdate  byte = 8
	CommandInfo            byte = 9
	CommandReadMultiFile   byte = 10
	CommandCheckAbort      byte = 11
	CommandClientPong      byte = 12
	CommandServerAvailable byte = 13
)

// Protocol version
const CurrentSyncVersion = 1

// Security limits to prevent memory exhaustion and DoS attacks
const (
	// MaxStringLength is the maximum allowed string length (1MB)
	// Reasonable for filenames, paths, and metadata fields
	MaxStringLength = 1 * 1024 * 1024 // 1MB

	// MaxDataLength is the maximum allowed data payload (100MB)
	// Allows for large comic files while preventing extreme memory exhaustion
	MaxDataLength = 100 * 1024 * 1024 // 100MB
)

// WriteByte writes a single byte to the writer
func WriteByte(w io.Writer, b byte) error {
	_, err := w.Write([]byte{b})
	return err
}

// ReadByte reads a single byte from the reader
func ReadByte(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

// WriteBool writes a boolean as a byte (0x00 = false, 0x01 = true)
func WriteBool(w io.Writer, b bool) error {
	if b {
		return WriteByte(w, 0x01)
	}
	return WriteByte(w, 0x00)
}

// ReadBool reads a boolean from a byte (0x00 = false, anything else = true)
func ReadBool(r io.Reader) (bool, error) {
	b, err := ReadByte(r)
	if err != nil {
		return false, err
	}
	return b != 0x00, nil
}

// WriteInt writes a 32-bit integer in big-endian byte order
func WriteInt(w io.Writer, i int32) error {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(i))
	_, err := w.Write(buf)
	return err
}

// ReadInt reads a 32-bit integer in big-endian byte order
func ReadInt(r io.Reader) (int32, error) {
	buf := make([]byte, 4)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(buf)), nil
}

// WriteLong writes a 64-bit long in big-endian byte order
func WriteLong(w io.Writer, l int64) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(l))
	_, err := w.Write(buf)
	return err
}

// ReadLong reads a 64-bit long in big-endian byte order
func ReadLong(r io.Reader) (int64, error) {
	buf := make([]byte, 8)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(buf)), nil
}

// WriteString writes a length-prefixed UTF-8 string
// Format: INT(length) + BYTE[length](UTF-8 data)
func WriteString(w io.Writer, s string) error {
	data := []byte(s)
	if err := WriteInt(w, int32(len(data))); err != nil {
		return fmt.Errorf("failed to write string length: %w", err)
	}
	_, err := w.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write string data: %w", err)
	}
	return nil
}

// ReadString reads a length-prefixed UTF-8 string
// Format: INT(length) + BYTE[length](UTF-8 data)
func ReadString(r io.Reader) (string, error) {
	length, err := ReadInt(r)
	if err != nil {
		return "", fmt.Errorf("failed to read string length: %w", err)
	}

	if length < 0 {
		return "", fmt.Errorf("invalid string length: %d (negative)", length)
	}

	if length > MaxStringLength {
		return "", fmt.Errorf("invalid string length: %d (exceeds maximum of %d bytes)", length, MaxStringLength)
	}

	if length == 0 {
		return "", nil
	}

	buf := make([]byte, length)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return "", fmt.Errorf("failed to read string data: %w", err)
	}

	return string(buf), nil
}

// WriteData writes length-prefixed binary data
// Format: LONG(length) + BYTE[length](data)
func WriteData(w io.Writer, data []byte) error {
	if err := WriteLong(w, int64(len(data))); err != nil {
		return fmt.Errorf("failed to write data length: %w", err)
	}
	_, err := w.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}
	return nil
}

// ReadData reads length-prefixed binary data
// Format: LONG(length) + BYTE[length](data)
func ReadData(r io.Reader) ([]byte, error) {
	length, err := ReadDataLength(r)
	if err != nil {
		return nil, err
	}
	return ReadDataBody(r, length)
}

// ReadDataLength reads and validates just the LONG(length) prefix of a
// length-prefixed data block, without reading the body. Split out from
// ReadData so a caller expecting a potentially large body (e.g.
// Client.ReadFile) can extend its read deadline based on the announced
// length before reading the body itself - see
// internal/protocol.Client.dataTimeout.
func ReadDataLength(r io.Reader) (int64, error) {
	length, err := ReadLong(r)
	if err != nil {
		return 0, fmt.Errorf("failed to read data length: %w", err)
	}

	if length < 0 {
		return 0, fmt.Errorf("invalid data length: %d (negative)", length)
	}

	if length > MaxDataLength {
		return 0, fmt.Errorf("invalid data length: %d (exceeds maximum of %d bytes)", length, MaxDataLength)
	}

	return length, nil
}

// ReadDataBody reads exactly length bytes - the body half of ReadData,
// see ReadDataLength.
func ReadDataBody(r io.Reader, length int64) ([]byte, error) {
	if length == 0 {
		return []byte{}, nil
	}

	buf := make([]byte, length)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	return buf, nil
}
