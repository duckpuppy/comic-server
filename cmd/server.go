package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/protocol"
	"github.com/spf13/cobra"
)

var (
	serverPort    int
	discoveryPort int
	libraryPath   string
	ignoreDevices []string
)

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

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 7620, "Server control port (TCP)")
	serverCmd.Flags().IntVarP(&discoveryPort, "discovery-port", "d", 7615, "Device discovery port (UDP multicast)")
	serverCmd.Flags().StringVarP(&libraryPath, "library", "l", "", "Path to comic library directory (required)")
	serverCmd.Flags().StringSliceVarP(&ignoreDevices, "ignore-device", "i", []string{}, "Devices to ignore (can be IP address, device ID, or device name)")
	serverCmd.MarkFlagRequired("library")
}
