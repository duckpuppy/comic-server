package device

import (
	"fmt"
	"net"
	"strings"

	"github.com/duckpuppy/comic-server/internal/log"
	"golang.org/x/net/ipv4"
)

const (
	// MulticastGroup is the UDP multicast address for device discovery
	MulticastGroup = "224.34.123.90"

	// DiscoveryPort is the UDP port for device broadcasts
	DiscoveryPort = 7615

	// DevicePort is the TCP port where devices listen for commands
	DevicePort = 7614
)

// DiscoveredDevice represents a device discovered via UDP multicast
type DiscoveredDevice struct {
	Key       string // Device key from broadcast message
	IPAddress string // Source IP address
	WantsSync bool   // True if broadcast included ":Sync" suffix
}

// DiscoveryListener listens for ComicRack device broadcasts on UDP multicast
type DiscoveryListener struct {
	conn     *net.UDPConn
	devices  chan DiscoveredDevice
	errors   chan error
	stopChan chan struct{}
}

// NewDiscoveryListener creates a new UDP multicast listener on the given
// port. A port of 0 falls back to the default DiscoveryPort (7615).
func NewDiscoveryListener(port int) (*DiscoveryListener, error) {
	if port == 0 {
		port = DiscoveryPort
	}

	// Parse multicast group address
	groupAddr := net.ParseIP(MulticastGroup)
	if groupAddr == nil {
		return nil, fmt.Errorf("invalid multicast address: %s", MulticastGroup)
	}

	// Get the default network interface
	iface, err := getDefaultInterface()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interface: %w", err)
	}

	// Bind to the port on all interfaces (0.0.0.0:<port>)
	listenAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve listen address: %w", err)
	}

	// Create UDP connection
	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on UDP: %w", err)
	}

	// Create IPv4 packet conn for multicast control
	p := ipv4.NewPacketConn(conn)

	// Join the multicast group on the specific interface
	if err := p.JoinGroup(iface, &net.UDPAddr{IP: groupAddr}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to join multicast group: %w", err)
	}

	// Set multicast interface
	if err := p.SetMulticastInterface(iface); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set multicast interface: %w", err)
	}

	// Enable multicast loopback
	if err := p.SetMulticastLoopback(true); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set multicast loopback: %w", err)
	}

	// Set socket options for better multicast support
	if err := conn.SetReadBuffer(1024 * 1024); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set read buffer: %w", err)
	}

	return &DiscoveryListener{
		conn:     conn,
		devices:  make(chan DiscoveredDevice, 10),
		errors:   make(chan error, 10),
		stopChan: make(chan struct{}),
	}, nil
}

// Start begins listening for device broadcasts
// Returns channels for discovered devices and errors
func (d *DiscoveryListener) Start() (<-chan DiscoveredDevice, <-chan error) {
	go d.listen()
	return d.devices, d.errors
}

// Stop stops the discovery listener
func (d *DiscoveryListener) Stop() error {
	close(d.stopChan)
	return d.conn.Close()
}

// listen is the main loop that receives and parses UDP broadcasts
func (d *DiscoveryListener) listen() {
	defer close(d.devices)
	defer close(d.errors)

	buf := make([]byte, 1024)

	for {
		select {
		case <-d.stopChan:
			return
		default:
			// Read from UDP socket
			n, remoteAddr, err := d.conn.ReadFromUDP(buf)
			if err != nil {
				// Check if we're shutting down
				select {
				case <-d.stopChan:
					return
				default:
					d.errors <- fmt.Errorf("failed to read from UDP: %w", err)
					continue
				}
			}

			// Parse the broadcast message
			message := string(buf[:n])
			log.Debug().
				Str("from", remoteAddr.IP.String()).
				Str("message", message).
				Msg("Received UDP packet on discovery port")

			device, err := parseDiscoveryMessage(message, remoteAddr.IP.String())
			if err != nil {
				// Not a valid ComicRack broadcast, ignore
				log.Debug().
					Str("from", remoteAddr.IP.String()).
					Str("message", message).
					Err(err).
					Msg("Ignoring invalid discovery message")
				continue
			}

			// Send discovered device to channel
			select {
			case d.devices <- device:
			case <-d.stopChan:
				return
			}
		}
	}
}

// parseDiscoveryMessage parses a ComicRack device broadcast message
// Format: "ComicRack*:{device_key}" or "ComicRack*:{device_key}:Sync"
// where * can be empty, "Android", "iOS", etc.
// Matches the original ComicRackCE behavior using StartsWith("ComicRack")
func parseDiscoveryMessage(message string, sourceIP string) (DiscoveredDevice, error) {
	// Trim whitespace
	message = strings.TrimSpace(message)

	// Split by colon to parse the message
	// Expected format: "ComicRack[Variant]:key[:Sync]"
	parts := strings.Split(message, ":")
	if len(parts) < 2 {
		return DiscoveredDevice{}, fmt.Errorf("invalid message format")
	}

	// Check if the first part starts with "ComicRack" (matches any variant)
	if !strings.HasPrefix(parts[0], "ComicRack") {
		return DiscoveredDevice{}, fmt.Errorf("not a ComicRack broadcast")
	}

	// Extract device key (second part)
	deviceKey := strings.TrimSpace(parts[1])
	if deviceKey == "" {
		return DiscoveredDevice{}, fmt.Errorf("empty device key")
	}

	// Check for ":Sync" suffix (third part)
	wantsSync := len(parts) > 2 && parts[2] == "Sync"

	return DiscoveredDevice{
		Key:       deviceKey,
		IPAddress: sourceIP,
		WantsSync: wantsSync,
	}, nil
}

// getDefaultInterface returns the network interface with the default route
func getDefaultInterface() (*net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %w", err)
	}

	// Try to find an interface that is up and has multicast support
	for _, iface := range interfaces {
		// Skip loopback and interfaces that are down
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Check if interface supports multicast
		if iface.Flags&net.FlagMulticast == 0 {
			continue
		}

		// Check if interface has an IPv4 address
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				// Found a suitable interface
				return &iface, nil
			}
		}
	}

	return nil, fmt.Errorf("no suitable network interface found")
}
