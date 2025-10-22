package device

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// Edition represents the device edition/version type
type Edition string

const (
	EditionAndroidFree Edition = "Android Free"
	EditionAndroidFull Edition = "Android Full"
	EditionIOS         Edition = "iOS"
)

// Info contains device information from comicrack.ini
type Info struct {
	Name         string   // User-friendly device name
	Model        string   // Device model identifier
	Manufacturer string   // Device manufacturer
	Serial       string   // Device serial number (unique)
	ID           string   // Device key (used in discovery)
	Hash         string   // SHA1 hash for validation
	Version      int      // App version code
	Edition      Edition  // App edition
	Screen       string   // Screen dimensions (width,height,dpiX,dpiY)
	Capabilities []string // Device capabilities (e.g., "WEBP")
}

// MinVersion returns the minimum version required for this edition
func (i *Info) MinVersion() int {
	switch i.Edition {
	case EditionAndroidFree:
		return 100
	case EditionAndroidFull:
		return 89
	case EditionIOS:
		return 1
	default:
		return 0
	}
}

// BookLimit returns the maximum number of books for this edition
// Returns -1 for unlimited
func (i *Info) BookLimit() int {
	switch i.Edition {
	case EditionAndroidFree:
		return 100
	case EditionAndroidFull:
		return -1 // Unlimited
	case EditionIOS:
		return -1 // Unlimited
	default:
		return 0
	}
}

// ParseINI parses a comicrack.ini file and returns device information
func ParseINI(data []byte) (*Info, error) {
	info := &Info{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Parse key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Map to struct fields
		switch key {
		case "Name":
			info.Name = value
		case "Model":
			info.Model = value
		case "Manufacturer":
			info.Manufacturer = value
		case "Serial":
			info.Serial = value
		case "ID":
			info.ID = value
		case "Hash":
			info.Hash = value
		case "Version":
			version, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid version number %q: %w", value, err)
			}
			info.Version = version
		case "Edition":
			info.Edition = Edition(value)
		case "Screen":
			info.Screen = value
		case "Capabilities":
			// Comma-separated list
			if value != "" {
				info.Capabilities = strings.Split(value, ",")
				// Trim whitespace from each capability
				for i := range info.Capabilities {
					info.Capabilities[i] = strings.TrimSpace(info.Capabilities[i])
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan INI data: %w", err)
	}

	// Validate required fields
	if info.Name == "" {
		return nil, fmt.Errorf("missing required field: Name")
	}
	if info.Model == "" {
		return nil, fmt.Errorf("missing required field: Model")
	}
	if info.Manufacturer == "" {
		return nil, fmt.Errorf("missing required field: Manufacturer")
	}
	if info.Serial == "" {
		return nil, fmt.Errorf("missing required field: Serial")
	}
	if info.ID == "" {
		return nil, fmt.Errorf("missing required field: ID")
	}
	if info.Hash == "" {
		return nil, fmt.Errorf("missing required field: Hash")
	}
	if info.Version == 0 {
		return nil, fmt.Errorf("missing or invalid required field: Version")
	}
	if info.Edition == "" {
		return nil, fmt.Errorf("missing required field: Edition")
	}

	return info, nil
}

// HasCapability checks if the device supports a specific capability
func (i *Info) HasCapability(capability string) bool {
	for _, cap := range i.Capabilities {
		if strings.EqualFold(cap, capability) {
			return true
		}
	}
	return false
}

// String returns a human-readable representation of the device info
func (i *Info) String() string {
	bookLimit := i.BookLimit()
	limitStr := "unlimited"
	if bookLimit > 0 {
		limitStr = fmt.Sprintf("%d", bookLimit)
	}

	return fmt.Sprintf("%s (%s %s) - ID: %s, Edition: %s, Version: %d, Books: %s",
		i.Name, i.Manufacturer, i.Model, i.ID, i.Edition, i.Version, limitStr)
}

// ComputeHash calculates the expected SHA1 hash for device validation
// Hash = SHA1(Model + Manufacturer + Serial + Edition + Version)
func (i *Info) ComputeHash() string {
	concatenated := fmt.Sprintf("%s%s%s%s%d",
		i.Model,
		i.Manufacturer,
		i.Serial,
		i.Edition,
		i.Version,
	)

	hash := sha1.Sum([]byte(concatenated))
	return hex.EncodeToString(hash[:])
}

// ValidateHash checks if the device hash matches the computed hash
func (i *Info) ValidateHash() error {
	expected := i.ComputeHash()

	// Normalize both hashes to lowercase for comparison
	actualHash := strings.ToLower(strings.TrimSpace(i.Hash))
	expectedHash := strings.ToLower(expected)

	if actualHash != expectedHash {
		return fmt.Errorf("device hash mismatch: got %q, expected %q", i.Hash, expected)
	}

	return nil
}

// Validate performs full device validation
func (i *Info) Validate() error {
	// Check hash
	if err := i.ValidateHash(); err != nil {
		return fmt.Errorf("hash validation failed: %w", err)
	}

	// Check minimum version
	minVersion := i.MinVersion()
	if i.Version < minVersion {
		return fmt.Errorf("version %d is below minimum %d for edition %s", i.Version, minVersion, i.Edition)
	}

	return nil
}
