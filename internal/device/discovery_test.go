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

func ipv4Net(ip string) net.Addr {
	return &net.IPNet{IP: net.ParseIP(ip), Mask: net.CIDRMask(24, 32)}
}

func TestPickInterface(t *testing.T) {
	up := net.FlagUp | net.FlagMulticast
	wifi := ifaceInfo{iface: net.Interface{Index: 2, Name: "wlan0", Flags: up}, addrs: []net.Addr{ipv4Net("10.0.0.5")}}
	cable := ifaceInfo{iface: net.Interface{Index: 3, Name: "enp4s0", Flags: up}, addrs: []net.Addr{ipv4Net("192.168.0.95")}}
	lo := ifaceInfo{iface: net.Interface{Index: 1, Name: "lo", Flags: up | net.FlagLoopback}, addrs: []net.Addr{ipv4Net("127.0.0.1")}}
	down := ifaceInfo{iface: net.Interface{Index: 4, Name: "eth9", Flags: net.FlagMulticast}, addrs: []net.Addr{ipv4Net("192.168.0.96")}}
	noMulti := ifaceInfo{iface: net.Interface{Index: 5, Name: "tun0", Flags: net.FlagUp}, addrs: []net.Addr{ipv4Net("192.168.0.97")}}
	v6only := ifaceInfo{iface: net.Interface{Index: 6, Name: "v6", Flags: up}, addrs: []net.Addr{&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)}}}

	tests := []struct {
		name    string
		infos   []ifaceInfo
		route   net.IP
		want    string
		wantErr bool
	}{
		{"default route beats a lower index", []ifaceInfo{lo, wifi, cable}, net.ParseIP("192.168.0.95"), "enp4s0", false},
		{"no default route falls back to first eligible", []ifaceInfo{lo, wifi, cable}, nil, "wlan0", false},
		{"route address matches nothing falls back", []ifaceInfo{wifi, cable}, net.ParseIP("172.16.0.1"), "wlan0", false},
		{"skips loopback down no-multicast and IPv6-only", []ifaceInfo{lo, down, noMulti, v6only, cable}, nil, "enp4s0", false},
		{"route address on an ineligible interface is ignored", []ifaceInfo{noMulti, wifi}, net.ParseIP("192.168.0.97"), "wlan0", false},
		{"nothing eligible", []ifaceInfo{lo, down, noMulti, v6only}, nil, "", true},
		{"empty list", nil, nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pickInterface(tt.infos, tt.route)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("pickInterface: %v", err)
			}
			if got.Name != tt.want {
				t.Errorf("got %s, want %s", got.Name, tt.want)
			}
		})
	}
}
