package device_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/protocol"
)

// MockBroadcaster simulates a ComicRack device broadcasting discovery messages
type MockBroadcaster struct {
	conn      *net.UDPConn
	deviceKey string
	wantsSync bool
}

func NewMockBroadcaster(deviceKey string, wantsSync bool) (*MockBroadcaster, error) {
	// Connect to multicast group
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", device.MulticastGroup, device.DiscoveryPort))
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return nil, err
	}

	return &MockBroadcaster{
		conn:      conn,
		deviceKey: deviceKey,
		wantsSync: wantsSync,
	}, nil
}

func (m *MockBroadcaster) Broadcast() error {
	message := fmt.Sprintf("ComicRackAndroid:%s", m.deviceKey)
	if m.wantsSync {
		message += ":Sync"
	}

	_, err := m.conn.Write([]byte(message))
	return err
}

func (m *MockBroadcaster) Close() {
	m.conn.Close()
}

func TestDiscoveryListenerIntegration(t *testing.T) {
	// Create discovery listener
	listener, err := device.NewDiscoveryListener()
	if err != nil {
		t.Fatalf("Failed to create discovery listener: %v", err)
	}
	defer listener.Stop()

	deviceChan, errorChan := listener.Start()

	// Give listener time to start
	time.Sleep(100 * time.Millisecond)

	t.Run("DiscoverDevice", func(t *testing.T) {
		// Create mock broadcaster
		broadcaster, err := NewMockBroadcaster("test-device-001", false)
		if err != nil {
			t.Fatalf("Failed to create broadcaster: %v", err)
		}
		defer broadcaster.Close()

		// Broadcast discovery message
		if err := broadcaster.Broadcast(); err != nil {
			t.Fatalf("Failed to broadcast: %v", err)
		}

		// Wait for discovery
		select {
		case discovered := <-deviceChan:
			if discovered.Key != "test-device-001" {
				t.Errorf("Expected key 'test-device-001', got %q", discovered.Key)
			}
			if discovered.WantsSync {
				t.Error("Expected WantsSync=false")
			}
		case err := <-errorChan:
			t.Fatalf("Received error: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for device discovery")
		}
	})

	t.Run("DiscoverDeviceWithSync", func(t *testing.T) {
		broadcaster, err := NewMockBroadcaster("test-device-002", true)
		if err != nil {
			t.Fatalf("Failed to create broadcaster: %v", err)
		}
		defer broadcaster.Close()

		if err := broadcaster.Broadcast(); err != nil {
			t.Fatalf("Failed to broadcast: %v", err)
		}

		select {
		case discovered := <-deviceChan:
			if discovered.Key != "test-device-002" {
				t.Errorf("Expected key 'test-device-002', got %q", discovered.Key)
			}
			if !discovered.WantsSync {
				t.Error("Expected WantsSync=true")
			}
		case err := <-errorChan:
			t.Fatalf("Received error: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for device discovery")
		}
	})

	t.Run("DiscoveriOSDevice", func(t *testing.T) {
		// Test iOS variant
		addr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", device.MulticastGroup, device.DiscoveryPort))
		conn, _ := net.DialUDP("udp4", nil, addr)
		defer conn.Close()

		message := "ComicRackiOS:ios-device-001"
		conn.Write([]byte(message))

		select {
		case discovered := <-deviceChan:
			if discovered.Key != "ios-device-001" {
				t.Errorf("Expected key 'ios-device-001', got %q", discovered.Key)
			}
		case err := <-errorChan:
			t.Fatalf("Received error: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for device discovery")
		}
	})
}

func TestDeviceRegistryIntegration(t *testing.T) {
	// Create registry
	registry := device.NewRegistry()

	// Create mock device server
	server, err := protocol.NewMockDeviceServer(device.DevicePort)
	if err != nil {
		t.Fatalf("Failed to create mock server: %v", err)
	}
	defer server.Stop()

	// Add comicrack.ini to mock server
	iniContent := []byte(`[Device]
Name=Test Tablet
Model=SM-T970
Manufacturer=Samsung
Serial=123456789
ID=test-device-001
Hash=69a8ba20bbf08f4f3e365a0da7e3de8d6207fd63
Version=123
Edition=Android Full
Capabilities=Sync,ReadLists
`)
	server.AddFile("comicrack.ini", iniContent)
	server.Start()

	time.Sleep(50 * time.Millisecond)

	t.Run("ValidateAndRegisterDevice", func(t *testing.T) {
		// Create client
		client := protocol.NewClient("127.0.0.1", server.Port())

		// Read device info
		iniData, err := client.ReadFile("comicrack.ini")
		if err != nil {
			t.Fatalf("Failed to read device info: %v", err)
		}

		// Parse device info
		info, err := device.ParseINI(iniData)
		if err != nil {
			t.Fatalf("Failed to parse device info: %v", err)
		}

		// Validate device
		if err := info.Validate(); err != nil {
			t.Fatalf("Device validation failed: %v", err)
		}

		// Add to registry
		registry.Add(info, "127.0.0.1")

		// Verify registration
		if registry.Count() != 1 {
			t.Errorf("Expected 1 device in registry, got %d", registry.Count())
		}

		// Get device from registry
		registeredInfo, ok := registry.Get("test-device-001")
		if !ok {
			t.Fatal("Device not found in registry")
		}

		if registeredInfo.Info.Name != "Test Tablet" {
			t.Errorf("Expected name 'Test Tablet', got %q", registeredInfo.Info.Name)
		}

		if registeredInfo.Info.Model != "SM-T970" {
			t.Errorf("Expected model 'SM-T970', got %q", registeredInfo.Info.Model)
		}
	})

	t.Run("UpdateLastSeen", func(t *testing.T) {
		// Get initial last seen time
		info1, _ := registry.Get("test-device-001")
		initialTime := info1.LastSeen

		// Wait a bit
		time.Sleep(10 * time.Millisecond)

		// Update last seen
		registry.UpdateLastSeen("test-device-001")

		// Verify time was updated
		info2, _ := registry.Get("test-device-001")
		if !info2.LastSeen.After(initialTime) {
			t.Error("LastSeen time should be updated")
		}
	})

	t.Run("ListDevices", func(t *testing.T) {
		devices := registry.List()
		if len(devices) != 1 {
			t.Errorf("Expected 1 device, got %d", len(devices))
		}

		if devices[0].Info.Name != "Test Tablet" {
			t.Errorf("Expected 'Test Tablet', got %q", devices[0].Info.Name)
		}
	})

	t.Run("RemoveDevice", func(t *testing.T) {
		registry.Remove("test-device-001")
		if registry.Count() != 0 {
			t.Errorf("Expected 0 devices after removal, got %d", registry.Count())
		}

		_, ok := registry.Get("test-device-001")
		if ok {
			t.Error("Device should not exist after removal")
		}
	})
}

func TestEndToEndDiscoveryAndRegistration(t *testing.T) {
	// This test simulates the full workflow:
	// 1. Device broadcasts discovery message
	// 2. Server discovers device
	// 3. Server connects to device and reads comicrack.ini
	// 4. Server validates and registers device

	// Create mock device server
	server, err := protocol.NewMockDeviceServer(device.DevicePort)
	if err != nil {
		t.Fatalf("Failed to create mock server: %v", err)
	}
	defer server.Stop()

	// Add comicrack.ini
	iniContent := []byte(`[Device]
Name=Integration Test Device
Model=Pixel 8
Manufacturer=Google
Serial=ABC123XYZ
ID=integration-test-001
Hash=6037f87e4c490a8231031141424a885122644652
Version=100
Edition=Android Full
`)
	server.AddFile("comicrack.ini", iniContent)
	server.Start()

	// Create discovery listener
	listener, err := device.NewDiscoveryListener()
	if err != nil {
		t.Fatalf("Failed to create discovery listener: %v", err)
	}
	defer listener.Stop()

	deviceChan, errorChan := listener.Start()
	time.Sleep(100 * time.Millisecond)

	// Create registry
	registry := device.NewRegistry()

	// Create mock broadcaster
	broadcaster, err := NewMockBroadcaster("integration-test-001", true)
	if err != nil {
		t.Fatalf("Failed to create broadcaster: %v", err)
	}
	defer broadcaster.Close()

	// Broadcast discovery message
	if err := broadcaster.Broadcast(); err != nil {
		t.Fatalf("Failed to broadcast: %v", err)
	}

	// Wait for discovery
	var discovered device.DiscoveredDevice
	select {
	case discovered = <-deviceChan:
	case err := <-errorChan:
		t.Fatalf("Received error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for device discovery")
	}

	// Verify discovery
	if discovered.Key != "integration-test-001" {
		t.Errorf("Expected key 'integration-test-001', got %q", discovered.Key)
	}

	if !discovered.WantsSync {
		t.Error("Expected WantsSync=true")
	}

	// Connect to device and read info
	client := protocol.NewClient("127.0.0.1", server.Port())
	iniData, err := client.ReadFile("comicrack.ini")
	if err != nil {
		t.Fatalf("Failed to read device info: %v", err)
	}

	// Parse and validate
	info, err := device.ParseINI(iniData)
	if err != nil {
		t.Fatalf("Failed to parse device info: %v", err)
	}

	if err := info.Validate(); err != nil {
		t.Fatalf("Device validation failed: %v", err)
	}

	// Register device
	registry.Add(info, discovered.IPAddress)

	// Verify registration
	if registry.Count() != 1 {
		t.Errorf("Expected 1 device in registry, got %d", registry.Count())
	}

	registeredInfo, ok := registry.Get(discovered.Key)
	if !ok {
		t.Fatal("Device not found in registry")
	}

	if registeredInfo.Info.Name != "Integration Test Device" {
		t.Errorf("Expected 'Integration Test Device', got %q", registeredInfo.Info.Name)
	}

	if registeredInfo.Info.Edition != "Android Full" {
		t.Errorf("Expected 'Android Full', got %q", registeredInfo.Info.Edition)
	}

	// Verify book limit is unlimited for "Android Full" (-1 means unlimited)
	if limit := registeredInfo.Info.BookLimit(); limit != -1 {
		t.Errorf("Expected unlimited books (-1), got %d", limit)
	}
}
