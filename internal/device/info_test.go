package device

import (
	"strings"
	"testing"
)

func TestParseINI(t *testing.T) {
	tests := []struct {
		name        string
		ini         string
		want        *Info
		expectError bool
	}{
		{
			name: "valid Android Full device",
			ini: `Name=Galaxy Tab S7
Model=SM-T870
Manufacturer=Samsung
Serial=R52M123456A
ID=abc123def456789
Hash=1a2b3c4d5e6f7g8h9i0j
Version=123
Edition=Android Full
Screen=2560,1600,320.0,320.0
Capabilities=WEBP`,
			want: &Info{
				Name:         "Galaxy Tab S7",
				Model:        "SM-T870",
				Manufacturer: "Samsung",
				Serial:       "R52M123456A",
				ID:           "abc123def456789",
				Hash:         "1a2b3c4d5e6f7g8h9i0j",
				Version:      123,
				Edition:      EditionAndroidFull,
				Screen:       "2560,1600,320.0,320.0",
				Capabilities: []string{"WEBP"},
			},
			expectError: false,
		},
		{
			name: "valid Android Free device",
			ini: `Name=Test Device
Model=Test-Model
Manufacturer=TestCo
Serial=TEST123
ID=testkey
Hash=testhash
Version=100
Edition=Android Free
Screen=1920,1080,240.0,240.0
Capabilities=`,
			want: &Info{
				Name:         "Test Device",
				Model:        "Test-Model",
				Manufacturer: "TestCo",
				Serial:       "TEST123",
				ID:           "testkey",
				Hash:         "testhash",
				Version:      100,
				Edition:      EditionAndroidFree,
				Screen:       "1920,1080,240.0,240.0",
				Capabilities: nil,
			},
			expectError: false,
		},
		{
			name: "valid iOS device",
			ini: `Name=iPad Pro
Model=iPad13,4
Manufacturer=Apple
Serial=FAKESERIAL
ID=iosdevicekey
Hash=ioshash
Version=50
Edition=iOS`,
			want: &Info{
				Name:         "iPad Pro",
				Model:        "iPad13,4",
				Manufacturer: "Apple",
				Serial:       "FAKESERIAL",
				ID:           "iosdevicekey",
				Hash:         "ioshash",
				Version:      50,
				Edition:      EditionIOS,
			},
			expectError: false,
		},
		{
			name: "with comments and whitespace",
			ini: `# This is a comment
Name=Test Device
  Model=Test-Model
Manufacturer=TestCo

; Another comment
Serial=TEST123
ID=testkey
Hash=testhash
Version=100
Edition=Android Free`,
			want: &Info{
				Name:         "Test Device",
				Model:        "Test-Model",
				Manufacturer: "TestCo",
				Serial:       "TEST123",
				ID:           "testkey",
				Hash:         "testhash",
				Version:      100,
				Edition:      EditionAndroidFree,
			},
			expectError: false,
		},
		{
			name: "multiple capabilities",
			ini: `Name=Test
Model=Test
Manufacturer=Test
Serial=TEST123
ID=test
Hash=test
Version=100
Edition=Android Full
Capabilities=WEBP, HEIF, AVIF`,
			want: &Info{
				Name:         "Test",
				Model:        "Test",
				Manufacturer: "Test",
				Serial:       "TEST123",
				ID:           "test",
				Hash:         "test",
				Version:      100,
				Edition:      EditionAndroidFull,
				Capabilities: []string{"WEBP", "HEIF", "AVIF"},
			},
			expectError: false,
		},
		{
			name: "missing Name field",
			ini: `Model=Test
Manufacturer=Test
Serial=TEST123
ID=test
Hash=test
Version=100
Edition=Android Full`,
			expectError: true,
		},
		{
			name: "missing Model field",
			ini: `Name=Test
Manufacturer=Test
Serial=TEST123
ID=test
Hash=test
Version=100
Edition=Android Full`,
			expectError: true,
		},
		{
			name: "invalid Version",
			ini: `Name=Test
Model=Test
Manufacturer=Test
Serial=TEST123
ID=test
Hash=test
Version=not_a_number
Edition=Android Full`,
			expectError: true,
		},
		{
			name:        "empty INI",
			ini:         "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseINI([]byte(tt.ini))

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Compare fields
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.Model != tt.want.Model {
				t.Errorf("Model = %q, want %q", got.Model, tt.want.Model)
			}
			if got.Manufacturer != tt.want.Manufacturer {
				t.Errorf("Manufacturer = %q, want %q", got.Manufacturer, tt.want.Manufacturer)
			}
			if got.Serial != tt.want.Serial {
				t.Errorf("Serial = %q, want %q", got.Serial, tt.want.Serial)
			}
			if got.ID != tt.want.ID {
				t.Errorf("ID = %q, want %q", got.ID, tt.want.ID)
			}
			if got.Hash != tt.want.Hash {
				t.Errorf("Hash = %q, want %q", got.Hash, tt.want.Hash)
			}
			if got.Version != tt.want.Version {
				t.Errorf("Version = %d, want %d", got.Version, tt.want.Version)
			}
			if got.Edition != tt.want.Edition {
				t.Errorf("Edition = %q, want %q", got.Edition, tt.want.Edition)
			}

			// Compare capabilities
			if len(got.Capabilities) != len(tt.want.Capabilities) {
				t.Errorf("Capabilities count = %d, want %d", len(got.Capabilities), len(tt.want.Capabilities))
			} else {
				for i := range got.Capabilities {
					if got.Capabilities[i] != tt.want.Capabilities[i] {
						t.Errorf("Capabilities[%d] = %q, want %q", i, got.Capabilities[i], tt.want.Capabilities[i])
					}
				}
			}
		})
	}
}

func TestDeviceInfo_MinVersion(t *testing.T) {
	tests := []struct {
		edition Edition
		want    int
	}{
		{EditionAndroidFree, 100},
		{EditionAndroidFull, 89},
		{EditionIOS, 1},
	}

	for _, tt := range tests {
		t.Run(string(tt.edition), func(t *testing.T) {
			info := &Info{Edition: tt.edition}
			got := info.MinVersion()
			if got != tt.want {
				t.Errorf("MinVersion() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDeviceInfo_BookLimit(t *testing.T) {
	tests := []struct {
		edition Edition
		want    int
	}{
		{EditionAndroidFree, 100},
		{EditionAndroidFull, -1},
		{EditionIOS, -1},
	}

	for _, tt := range tests {
		t.Run(string(tt.edition), func(t *testing.T) {
			info := &Info{Edition: tt.edition}
			got := info.BookLimit()
			if got != tt.want {
				t.Errorf("BookLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDeviceInfo_HasCapability(t *testing.T) {
	info := &Info{
		Capabilities: []string{"WEBP", "HEIF"},
	}

	tests := []struct {
		capability string
		want       bool
	}{
		{"WEBP", true},
		{"webp", true}, // Case insensitive
		{"HEIF", true},
		{"AVIF", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.capability, func(t *testing.T) {
			got := info.HasCapability(tt.capability)
			if got != tt.want {
				t.Errorf("HasCapability(%q) = %v, want %v", tt.capability, got, tt.want)
			}
		})
	}
}

func TestDeviceInfo_ComputeHash(t *testing.T) {
	info := &Info{
		Model:        "SM-T870",
		Manufacturer: "Samsung",
		Serial:       "R52M123456A",
		Edition:      EditionAndroidFull,
		Version:      123,
	}

	// Compute hash
	hash := info.ComputeHash()

	// Hash should be 40 characters (SHA1 hex)
	if len(hash) != 40 {
		t.Errorf("Hash length = %d, want 40", len(hash))
	}

	// Hash should be deterministic
	hash2 := info.ComputeHash()
	if hash != hash2 {
		t.Errorf("Hash is not deterministic: %q != %q", hash, hash2)
	}

	// Different inputs should produce different hashes
	info2 := *info
	info2.Serial = "DIFFERENT"
	hash3 := info2.ComputeHash()
	if hash == hash3 {
		t.Errorf("Different inputs produced same hash")
	}
}

func TestDeviceInfo_ValidateHash(t *testing.T) {
	tests := []struct {
		name        string
		info        *Info
		expectError bool
	}{
		{
			name: "valid hash",
			info: &Info{
				Model:        "Test",
				Manufacturer: "TestCo",
				Serial:       "123",
				Edition:      EditionAndroidFull,
				Version:      100,
				Hash:         "", // Will be computed
			},
			expectError: false,
		},
		{
			name: "invalid hash",
			info: &Info{
				Model:        "Test",
				Manufacturer: "TestCo",
				Serial:       "123",
				Edition:      EditionAndroidFull,
				Version:      100,
				Hash:         "wronghash",
			},
			expectError: true,
		},
		{
			name: "case insensitive hash",
			info: &Info{
				Model:        "Test",
				Manufacturer: "TestCo",
				Serial:       "123",
				Edition:      EditionAndroidFull,
				Version:      100,
				Hash:         "", // Will be set to uppercase
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set correct hash for "valid" test
			if tt.name == "valid hash" {
				tt.info.Hash = tt.info.ComputeHash()
			} else if tt.name == "case insensitive hash" {
				tt.info.Hash = strings.ToUpper(tt.info.ComputeHash())
			}

			err := tt.info.ValidateHash()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestDeviceInfo_Validate(t *testing.T) {
	tests := []struct {
		name        string
		info        *Info
		expectError bool
	}{
		{
			name: "valid Android Full device",
			info: &Info{
				Model:        "Test",
				Manufacturer: "TestCo",
				Serial:       "123",
				Edition:      EditionAndroidFull,
				Version:      89, // Minimum for Android Full
				Hash:         "", // Will be computed
			},
			expectError: false,
		},
		{
			name: "version too low",
			info: &Info{
				Model:        "Test",
				Manufacturer: "TestCo",
				Serial:       "123",
				Edition:      EditionAndroidFull,
				Version:      50, // Below minimum of 89
				Hash:         "", // Will be computed
			},
			expectError: true,
		},
		{
			name: "invalid hash",
			info: &Info{
				Model:        "Test",
				Manufacturer: "TestCo",
				Serial:       "123",
				Edition:      EditionAndroidFull,
				Version:      100,
				Hash:         "invalid",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compute correct hash if empty
			if tt.info.Hash == "" {
				tt.info.Hash = tt.info.ComputeHash()
			}

			err := tt.info.Validate()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
