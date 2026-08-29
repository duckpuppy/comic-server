package device

import (
	"net"
	"testing"
)

func TestNewDiscoveryListenerUsesConfiguredPort(t *testing.T) {
	// A non-default port must actually be bound, and two listeners on
	// different explicit ports must be able to run at the same time
	// without a bind conflict.
	l1, err := NewDiscoveryListener(17615)
	if err != nil {
		t.Fatalf("failed to create listener on 17615: %v", err)
	}
	defer l1.Stop()

	l2, err := NewDiscoveryListener(17616)
	if err != nil {
		t.Fatalf("failed to create listener on 17616: %v", err)
	}
	defer l2.Stop()

	if got := l1.conn.LocalAddr().(*net.UDPAddr).Port; got != 17615 {
		t.Errorf("expected listener 1 bound to port 17615, got %d", got)
	}
	if got := l2.conn.LocalAddr().(*net.UDPAddr).Port; got != 17616 {
		t.Errorf("expected listener 2 bound to port 17616, got %d", got)
	}
}

func TestNewDiscoveryListenerDefaultsToStandardPort(t *testing.T) {
	l, err := NewDiscoveryListener(0)
	if err != nil {
		t.Fatalf("failed to create listener with default port: %v", err)
	}
	defer l.Stop()

	if got := l.conn.LocalAddr().(*net.UDPAddr).Port; got != DiscoveryPort {
		t.Errorf("expected default port %d, got %d", DiscoveryPort, got)
	}
}

func TestParseDiscoveryMessage(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		sourceIP    string
		want        DiscoveredDevice
		expectError bool
	}{
		{
			name:     "basic device broadcast",
			message:  "ComicRack:abc123def456",
			sourceIP: "192.168.1.100",
			want: DiscoveredDevice{
				Key:       "abc123def456",
				IPAddress: "192.168.1.100",
				WantsSync: false,
			},
			expectError: false,
		},
		{
			name:     "device requesting sync",
			message:  "ComicRack:abc123def456:Sync",
			sourceIP: "192.168.1.101",
			want: DiscoveredDevice{
				Key:       "abc123def456",
				IPAddress: "192.168.1.101",
				WantsSync: true,
			},
			expectError: false,
		},
		{
			name:     "Android client broadcast",
			message:  "ComicRackAndroid:79ee27d04e51f774cfe525bb7ddf111d264a323b",
			sourceIP: "192.168.1.102",
			want: DiscoveredDevice{
				Key:       "79ee27d04e51f774cfe525bb7ddf111d264a323b",
				IPAddress: "192.168.1.102",
				WantsSync: false,
			},
			expectError: false,
		},
		{
			name:     "Android client requesting sync",
			message:  "ComicRackAndroid:79ee27d04e51f774cfe525bb7ddf111d264a323b:Sync",
			sourceIP: "192.168.1.103",
			want: DiscoveredDevice{
				Key:       "79ee27d04e51f774cfe525bb7ddf111d264a323b",
				IPAddress: "192.168.1.103",
				WantsSync: true,
			},
			expectError: false,
		},
		{
			name:     "iOS client broadcast (hypothetical)",
			message:  "ComicRackiOS:ioskey123",
			sourceIP: "192.168.1.104",
			want: DiscoveredDevice{
				Key:       "ioskey123",
				IPAddress: "192.168.1.104",
				WantsSync: false,
			},
			expectError: false,
		},
		{
			name:     "with whitespace",
			message:  "  ComicRack:xyz789  ",
			sourceIP: "192.168.1.105",
			want: DiscoveredDevice{
				Key:       "xyz789",
				IPAddress: "192.168.1.105",
				WantsSync: false,
			},
			expectError: false,
		},
		{
			name:     "with whitespace and sync",
			message:  "  ComicRack:xyz789:Sync  ",
			sourceIP: "192.168.1.106",
			want: DiscoveredDevice{
				Key:       "xyz789",
				IPAddress: "192.168.1.106",
				WantsSync: true,
			},
			expectError: false,
		},
		{
			name:        "invalid prefix",
			message:     "NotComicRack:abc123",
			sourceIP:    "192.168.1.100",
			expectError: true,
		},
		{
			name:        "empty key",
			message:     "ComicRack:",
			sourceIP:    "192.168.1.100",
			expectError: true,
		},
		{
			name:        "missing key",
			message:     "ComicRack",
			sourceIP:    "192.168.1.100",
			expectError: true,
		},
		{
			name:        "empty message",
			message:     "",
			sourceIP:    "192.168.1.100",
			expectError: true,
		},
		{
			name:        "random data",
			message:     "random data",
			sourceIP:    "192.168.1.100",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDiscoveryMessage(tt.message, tt.sourceIP)

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

			if got.Key != tt.want.Key {
				t.Errorf("Key = %q, want %q", got.Key, tt.want.Key)
			}
			if got.IPAddress != tt.want.IPAddress {
				t.Errorf("IPAddress = %q, want %q", got.IPAddress, tt.want.IPAddress)
			}
			if got.WantsSync != tt.want.WantsSync {
				t.Errorf("WantsSync = %v, want %v", got.WantsSync, tt.want.WantsSync)
			}
		})
	}
}
