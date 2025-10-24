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
	// CLI flag values (when explicitly provided, override config file)
	serverPort      int
	discoveryPort   int
	libraryPath     string
	ignoreDevices   []string
	bindAddress     string
	autoSync        bool
	logLevel        string
	logFormat       string

	// Track which flags were explicitly set by user
	serverPortSet      bool
	discoveryPortSet   bool
	libraryPathSet     bool
	ignoreDevicesSet   bool
	bindAddressSet     bool
	autoSyncSet        bool
	logLevelSet        bool
	logFormatSet       bool
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

	// Load config (applies defaults and environment variables)
	configPath, err := GetConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply CLI flag overrides (CLI flags take precedence over config file and environment)
	if serverPortSet {
		cfg.Server.ServerPort = serverPort
	}
	if discoveryPortSet {
		cfg.Server.DiscoveryPort = discoveryPort
	}
	if libraryPathSet {
		cfg.Server.LibraryPath = libraryPath
	}
	if ignoreDevicesSet {
		cfg.Server.IgnoreDevices = ignoreDevices
	}
	if bindAddressSet {
		cfg.Server.BindAddress = bindAddress
	}
	if autoSyncSet {
		cfg.Server.AutoSync = autoSync
	}
	if logLevelSet {
		cfg.Server.LogLevel = logLevel
	}
	if logFormatSet {
		cfg.Server.LogFormat = logFormat
	}

	// Validate final configuration (after CLI overrides)
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Check that library path is set
	if cfg.Server.LibraryPath == "" {
		return fmt.Errorf("library path is required (set via --library flag, config file, or COMIC_SERVER_LIBRARY_PATH environment variable)")
	}

	// Display configuration
	fmt.Printf("   Server control port: %d+\n", cfg.Server.ServerPort)
	fmt.Printf("   Discovery port: %d (UDP multicast %s)\n", cfg.Server.DiscoveryPort, device.MulticastGroup)
	fmt.Printf("   Library path: %s\n", cfg.Server.LibraryPath)
	if cfg.Server.BindAddress != "" {
		fmt.Printf("   Bind address: %s\n", cfg.Server.BindAddress)
	}
	if len(cfg.Server.IgnoreDevices) > 0 {
		fmt.Printf("   Ignoring devices: %v\n", cfg.Server.IgnoreDevices)
	}
	fmt.Printf("   Auto-sync: %v\n", cfg.Server.AutoSync)
	fmt.Printf("   Log level: %s\n", cfg.Server.LogLevel)
	fmt.Printf("   Log format: %s\n", cfg.Server.LogFormat)
	fmt.Printf("   Config: %s\n", configPath)
	fmt.Printf("   Configured devices: %d\n", len(cfg.Devices))
	fmt.Println()

	// Load library
	lib, err := library.LoadLibrary(cfg.Server.LibraryPath)
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
			handleDiscoveredDevice(discovered, registry, cfg, lib)
		}
	}
}

func shouldIgnoreDevice(ipAddress, deviceID, deviceName string, ignoreList []string) bool {
	for _, ignore := range ignoreList {
		if ignore == ipAddress || ignore == deviceID || ignore == deviceName {
			return true
		}
	}
	return false
}

func handleDiscoveredDevice(discovered device.DiscoveredDevice, registry *device.Registry, cfg *config.Config, lib *library.ComicLibrary) {
	// Check if this device should be ignored by IP (early check)
	if shouldIgnoreDevice(discovered.IPAddress, "", "", cfg.Server.IgnoreDevices) {
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
	if shouldIgnoreDevice(discovered.IPAddress, info.ID, info.Name, cfg.Server.IgnoreDevices) {
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

	// Handle sync request if device wants sync and auto-sync is enabled
	if discovered.WantsSync && cfg.Server.AutoSync {
		fmt.Printf("📲 Device requested sync, starting automatic sync...\n")
		// Note: We're already in handleDiscoveredDevice which is called from the main loop,
		// so we need to be careful about blocking. For v0.2, we'll sync synchronously.
		// v0.3 can improve this with goroutines and better concurrency.
		if err := handleSyncRequest(info.ID, discovered.IPAddress, cfg, lib); err != nil {
			fmt.Printf("   ❌ Sync failed: %v\n\n", err)
		}
	} else if discovered.WantsSync && !cfg.Server.AutoSync {
		fmt.Printf("   ℹ️  Device requested sync but auto-sync is disabled\n\n")
	}
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

	// Perform sync
	result, err := syncer.PerformSync()
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Display sync results
	fmt.Printf("✅ Sync completed:\n")
	fmt.Printf("   Added: %d books\n", result.BooksAdded)
	fmt.Printf("   Updated: %d books\n", result.BooksUpdated)
	fmt.Printf("   Deleted: %d books\n", result.BooksDeleted)
	if len(result.Errors) > 0 {
		fmt.Printf("   ⚠️  Errors: %d\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Printf("      - %v\n", err)
		}
	}
	fmt.Println()

	return nil
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 0, "Server control port (TCP, default: 7620)")
	serverCmd.Flags().IntVarP(&discoveryPort, "discovery-port", "d", 0, "Device discovery port (UDP multicast, default: 7615)")
	serverCmd.Flags().StringVarP(&libraryPath, "library", "l", "", "Path to ComicDB.xml file")
	serverCmd.Flags().StringSliceVarP(&ignoreDevices, "ignore-device", "i", nil, "Devices to ignore (can be IP address, device ID, or device name)")
	serverCmd.Flags().StringVarP(&bindAddress, "bind", "b", "", "Network interface to bind to (default: all interfaces)")
	serverCmd.Flags().BoolVar(&autoSync, "auto-sync", false, "Automatically sync devices when they connect")
	serverCmd.Flags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error (default: info)")
	serverCmd.Flags().StringVar(&logFormat, "log-format", "", "Log format: text, json (default: text)")

	// Mark which flags to check for being explicitly set
	serverCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		serverPortSet = cmd.Flags().Changed("port")
		discoveryPortSet = cmd.Flags().Changed("discovery-port")
		libraryPathSet = cmd.Flags().Changed("library")
		ignoreDevicesSet = cmd.Flags().Changed("ignore-device")
		bindAddressSet = cmd.Flags().Changed("bind")
		autoSyncSet = cmd.Flags().Changed("auto-sync")
		logLevelSet = cmd.Flags().Changed("log-level")
		logFormatSet = cmd.Flags().Changed("log-format")
		return nil
	}
}
