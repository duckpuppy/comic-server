package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/protocol"
	csync "github.com/duckpuppy/comic-server/internal/sync"
	"github.com/spf13/cobra"
)

var (
	serverPort    int
	discoveryPort int
	libraryPath   string
	ignoreDevices []string
)

// syncMutex ensures only one device syncs at a time (v0.2 limitation)
var syncMutex sync.Mutex

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the wireless sync server",
	Long: `Start the comic-server wireless sync server.

This will start listening for device discovery broadcasts and handle
sync requests from ComicRack Android/iOS clients.`,
	RunE: runServer,
}

func runServer(cmd *cobra.Command, args []string) error {
	fmt.Println("🚀 Starting comic-server...")
	fmt.Printf("   Server control port: %d+\n", serverPort)
	fmt.Printf("   Discovery port: %d (UDP multicast %s)\n", discoveryPort, device.MulticastGroup)
	fmt.Printf("   Library path: %s\n", libraryPath)

	// Load config
	configPath, err := GetConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("   Config: %s\n", configPath)
	fmt.Printf("   Configured devices: %d\n", len(cfg.Devices))
	fmt.Println()

	// Load library
	lib, err := library.LoadLibrary(libraryPath)
	if err != nil {
		return fmt.Errorf("failed to load library: %w", err)
	}

	fmt.Printf("📚 Library loaded: %d books, %d lists\n", len(lib.Books), len(lib.ComicLists))
	fmt.Println()

	// Create device registry
	registry := device.NewRegistry()

	// Start UDP multicast listener
	listener, err := device.NewDiscoveryListener()
	if err != nil {
		return fmt.Errorf("failed to start discovery listener: %w", err)
	}
	defer listener.Stop()

	fmt.Printf("🔍 Listening for device broadcasts on %s:%d...\n", device.MulticastGroup, device.DiscoveryPort)
	fmt.Println("   Press Ctrl+C to stop")
	fmt.Println()

	// Start listening
	deviceChan, errorChan := listener.Start()

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Main loop
	for {
		select {
		case <-sigChan:
			fmt.Println("\n\n👋 Shutting down server...")
			return nil

		case err := <-errorChan:
			fmt.Printf("⚠️  Error: %v\n", err)

		case discovered := <-deviceChan:
			handleDiscoveredDevice(discovered, registry)
		}
	}
}

func shouldIgnoreDevice(ipAddress, deviceID, deviceName string) bool {
	for _, ignore := range ignoreDevices {
		if ignore == ipAddress || ignore == deviceID || ignore == deviceName {
			return true
		}
	}
	return false
}

func handleDiscoveredDevice(discovered device.DiscoveredDevice, registry *device.Registry) {
	// Check if this device should be ignored by IP (early check)
	if shouldIgnoreDevice(discovered.IPAddress, "", "") {
		return
	}

	// Check if we already know this device
	if _, ok := registry.Get(discovered.Key); ok {
		// Device already registered, just update last seen timestamp
		// DEBUG: Uncomment to see repeated discoveries
		// fmt.Printf("   Device %s already known\n", discovered.Key)
		registry.UpdateLastSeen(discovered.Key)
		return
	}

	fmt.Printf("📱 Device discovered: %s", discovered.IPAddress)
	if discovered.WantsSync {
		fmt.Printf(" (requesting sync)")
	}
	fmt.Println()

	// Connect to device and read comicrack.ini
	client := protocol.NewClient(discovered.IPAddress, device.DevicePort)

	fmt.Printf("   Reading device info...\n")
	iniData, err := client.ReadFile("comicrack.ini")
	if err != nil {
		fmt.Printf("   ❌ Failed to read device info: %v\n\n", err)
		return
	}

	// Parse device info
	info, err := device.ParseINI(iniData)
	if err != nil {
		fmt.Printf("   ❌ Failed to parse device info: %v\n\n", err)
		return
	}

	// Validate device
	if err := info.Validate(); err != nil {
		fmt.Printf("   ❌ Device validation failed: %v\n\n", err)
		return
	}

	// Check if device should be ignored by ID or name
	if shouldIgnoreDevice(discovered.IPAddress, info.ID, info.Name) {
		fmt.Printf("   ⏭️  Ignoring device (matches ignore filter)\n\n")
		return
	}

	// Add to registry
	registry.Add(info, discovered.IPAddress)

	// Display device information
	fmt.Printf("   ✅ %s\n", info.Name)
	fmt.Printf("      Model: %s %s\n", info.Manufacturer, info.Model)
	fmt.Printf("      Edition: %s (v%d)\n", info.Edition, info.Version)
	fmt.Printf("      ID: %s\n", info.ID)

	bookLimit := info.BookLimit()
	if bookLimit > 0 {
		fmt.Printf("      Book limit: %d\n", bookLimit)
	} else {
		fmt.Printf("      Book limit: unlimited\n")
	}

	if len(info.Capabilities) > 0 {
		fmt.Printf("      Capabilities: %v\n", info.Capabilities)
	}

	fmt.Printf("\n   📊 Total devices: %d\n", registry.Count())
	fmt.Println()
}

// applyDeviceConfig applies a device's sync configuration to a syncer
// This configures which smart lists to sync and their settings
func applyDeviceConfig(syncer *csync.Syncer, deviceConfig *config.DeviceConfig, lib *library.ComicLibrary) error {
	// For v0.2, we only sync the first enabled list
	// TODO: v0.3 - support syncing multiple lists per device

	for _, listConfig := range deviceConfig.Lists {
		if !listConfig.Enabled {
			continue
		}

		// 1. Lookup smart list by GUID
		smartList := config.FindListByGUID(lib, listConfig.ListID)
		if smartList == nil {
			return fmt.Errorf("smart list %s (ID: %s) not found in library", listConfig.ListName, listConfig.ListID)
		}

		// 2. Set as filter list
		if err := syncer.SetFilterList(smartList); err != nil {
			return fmt.Errorf("failed to set filter list: %w", err)
		}

		// 3. Apply settings (use list settings or device defaults)
		settings := listConfig.Settings
		if settings == nil {
			settings = deviceConfig.DefaultSettings
		}
		if settings == nil {
			settings = csync.DefaultSettings()
		}
		syncer.SetSettings(settings)

		fmt.Printf("   📋 Syncing list: %s\n", listConfig.ListName)

		// For v0.2, only sync first enabled list
		break
	}

	return nil
}

// handleSyncRequest handles a sync request from a device
// This is a placeholder for actual sync implementation (coming in future)
func handleSyncRequest(deviceID, deviceIP string, cfg *config.Config, lib *library.ComicLibrary) error {
	// Acquire global sync lock (only one device can sync at a time in v0.2)
	syncMutex.Lock()
	defer syncMutex.Unlock()

	fmt.Printf("🔄 Starting sync for device: %s\n", deviceID)

	// Create protocol client
	client := protocol.NewClient(deviceIP, device.DevicePort)

	// Create syncer
	syncer := csync.NewSyncer(client, lib)

	// Apply device config if exists
	if deviceConfig, ok := cfg.Devices[deviceID]; ok {
		if err := applyDeviceConfig(syncer, deviceConfig, lib); err != nil {
			return fmt.Errorf("failed to apply device config: %w", err)
		}
	} else {
		fmt.Printf("   ℹ️  No sync configuration for device %s, syncing all books\n", deviceID)
	}

	// TODO: Call syncer.PerformSync() when sync implementation is complete
	fmt.Printf("   ℹ️  Sync not yet implemented (coming in future milestone)\n")

	return nil
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 7620, "Server control port (TCP)")
	serverCmd.Flags().IntVarP(&discoveryPort, "discovery-port", "d", 7615, "Device discovery port (UDP multicast)")
	serverCmd.Flags().StringVarP(&libraryPath, "library", "l", "", "Path to comic library directory (required)")
	serverCmd.Flags().StringSliceVarP(&ignoreDevices, "ignore-device", "i", []string{}, "Devices to ignore (can be IP address, device ID, or device name)")
	serverCmd.MarkFlagRequired("library")
}
