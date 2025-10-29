package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duckpuppy/comic-server/internal/api"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/protocol"
	"github.com/duckpuppy/comic-server/internal/ratelimit"
	csync "github.com/duckpuppy/comic-server/internal/sync"
	"github.com/duckpuppy/comic-server/internal/syncstate"
	"github.com/spf13/cobra"
)

var (
	// CLI flag values (when explicitly provided, override config file)
	serverPort             int
	discoveryPort          int
	libraryPath            string
	ignoreDevices          []string
	bindAddress            string
	autoSync               bool
	maxConcurrentConns     int
	maxConnectionsPerIP    int
	maxRequestsPerDevice   int
	rateLimitWindowSeconds int
	logLevel               string
	logFormat              string

	// Track which flags were explicitly set by user
	serverPortSet             bool
	discoveryPortSet          bool
	libraryPathSet            bool
	ignoreDevicesSet          bool
	bindAddressSet            bool
	autoSyncSet               bool
	maxConcurrentConnsSet     bool
	maxConnectionsPerIPSet    bool
	maxRequestsPerDeviceSet   bool
	rateLimitWindowSecondsSet bool
	logLevelSet               bool
	logFormatSet              bool
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
	if maxConcurrentConnsSet {
		cfg.Server.MaxConcurrentConnections = maxConcurrentConns
	}
	if maxConnectionsPerIPSet {
		cfg.Server.MaxConnectionsPerIP = maxConnectionsPerIP
	}
	if maxRequestsPerDeviceSet {
		cfg.Server.MaxRequestsPerDevice = maxRequestsPerDevice
	}
	if rateLimitWindowSecondsSet {
		cfg.Server.RateLimitWindowSeconds = rateLimitWindowSeconds
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

	// Initialize logging with validated configuration
	if err := log.Init(log.Config{
		Level:  cfg.Server.LogLevel,
		Format: cfg.Server.LogFormat,
		Output: "stdout",
	}); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}

	// Check that library path is set
	if cfg.Server.LibraryPath == "" {
		return fmt.Errorf("library path is required (set via --library flag, config file, or COMIC_SERVER_LIBRARY_PATH environment variable)")
	}

	// Log server startup and configuration
	log.Info().Msg("Starting comic-server")
	log.Info().
		Int("server_port", cfg.Server.ServerPort).
		Int("discovery_port", cfg.Server.DiscoveryPort).
		Str("multicast_group", device.MulticastGroup).
		Str("library_path", cfg.Server.LibraryPath).
		Str("bind_address", cfg.Server.BindAddress).
		Strs("ignore_devices", cfg.Server.IgnoreDevices).
		Bool("auto_sync", cfg.Server.AutoSync).
		Str("log_level", cfg.Server.LogLevel).
		Str("log_format", cfg.Server.LogFormat).
		Str("config_path", configPath).
		Int("configured_devices", len(cfg.Devices)).
		Msg("Server configuration loaded")

	// Load library
	log.Debug().Str("path", cfg.Server.LibraryPath).Msg("Loading comic library")
	lib, err := library.LoadLibrary(cfg.Server.LibraryPath)
	if err != nil {
		return fmt.Errorf("failed to load library: %w", err)
	}

	log.Info().
		Int("books", len(lib.Books)).
		Int("lists", len(lib.ComicLists)).
		Msg("Library loaded successfully")

	// Create device registry
	registry := device.NewRegistry()

	// Create sync state manager (max 100 history entries)
	syncManager := syncstate.NewManager(100)

	// Initialize rate limiters
	var ipLimiter *ratelimit.IPLimiter
	var deviceLimiter *ratelimit.DeviceLimiter
	var syncSemaphore chan struct{}

	// Create IP rate limiter if enabled
	if cfg.Server.MaxConnectionsPerIP > 0 {
		window := time.Duration(cfg.Server.RateLimitWindowSeconds) * time.Second
		ipLimiter = ratelimit.NewIPLimiter(cfg.Server.MaxConnectionsPerIP, window)
		defer ipLimiter.Stop()
		log.Info().
			Int("max_per_ip", cfg.Server.MaxConnectionsPerIP).
			Int("window_seconds", cfg.Server.RateLimitWindowSeconds).
			Msg("IP rate limiting enabled")
	}

	// Create device rate limiter if enabled
	if cfg.Server.MaxRequestsPerDevice > 0 {
		window := time.Duration(cfg.Server.RateLimitWindowSeconds) * time.Second
		deviceLimiter = ratelimit.NewDeviceLimiter(cfg.Server.MaxRequestsPerDevice, window)
		defer deviceLimiter.Stop()
		log.Info().
			Int("max_per_device", cfg.Server.MaxRequestsPerDevice).
			Int("window_seconds", cfg.Server.RateLimitWindowSeconds).
			Msg("Device rate limiting enabled")
	}

	// Create connection semaphore if concurrent connection limit is set
	if cfg.Server.MaxConcurrentConnections > 0 {
		syncSemaphore = make(chan struct{}, cfg.Server.MaxConcurrentConnections)
		log.Info().
			Int("max_concurrent", cfg.Server.MaxConcurrentConnections).
			Msg("Concurrent connection limiting enabled")
	}

	// Start UDP multicast listener
	log.Debug().Msg("Creating discovery listener")
	listener, err := device.NewDiscoveryListener()
	if err != nil {
		return fmt.Errorf("failed to start discovery listener: %w", err)
	}
	defer listener.Stop()

	log.Info().
		Str("multicast_group", device.MulticastGroup).
		Int("port", device.DiscoveryPort).
		Msg("Listening for device broadcasts")

	// Start listening
	deviceChan, errorChan := listener.Start()

	// Start REST API HTTP server
	apiVersion := api.VersionInfo{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
	}
	apiServer := api.NewServer(syncManager, registry, cfg, apiVersion)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.ServerPort),
		Handler: apiServer,
	}

	// Start HTTP server in background
	go func() {
		log.Info().
			Int("port", cfg.Server.ServerPort).
			Msg("REST API server listening")
		log.Info().Msg("Available API endpoints:")
		log.Info().Msgf("  GET  http://localhost:%d/api/health - Health check with version info", cfg.Server.ServerPort)
		log.Info().Msgf("  GET  http://localhost:%d/api/version - Build version information", cfg.Server.ServerPort)
		log.Info().Msgf("  GET  http://localhost:%d/api/sync/status - Active sync operations", cfg.Server.ServerPort)
		log.Info().Msgf("  GET  http://localhost:%d/api/sync/history?limit=N - Sync history", cfg.Server.ServerPort)
		log.Info().Msgf("  GET  http://localhost:%d/api/devices - Registered devices", cfg.Server.ServerPort)
		log.Info().Msgf("  GET  http://localhost:%d/api/stats - Server statistics", cfg.Server.ServerPort)
		log.Info().Msgf("  GET  http://localhost:%d/metrics - Prometheus metrics", cfg.Server.ServerPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("REST API server error")
		}
	}()

	// Handle signals gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	log.Info().Msg("Server ready - press Ctrl+C to stop")

	// Main loop
	for {
		select {
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGHUP:
				log.Info().Msg("Received SIGHUP, reloading configuration")
				// Reload configuration
				newCfg, err := config.Load(configPath)
				if err != nil {
					log.Error().Err(err).Msg("Failed to reload configuration")
					continue
				}

				// Re-apply CLI flag overrides
				if serverPortSet {
					newCfg.Server.ServerPort = serverPort
				}
				if discoveryPortSet {
					newCfg.Server.DiscoveryPort = discoveryPort
				}
				if libraryPathSet {
					newCfg.Server.LibraryPath = libraryPath
				}
				if ignoreDevicesSet {
					newCfg.Server.IgnoreDevices = ignoreDevices
				}
				if bindAddressSet {
					newCfg.Server.BindAddress = bindAddress
				}
				if autoSyncSet {
					newCfg.Server.AutoSync = autoSync
				}
				if logLevelSet {
					newCfg.Server.LogLevel = logLevel
				}
				if logFormatSet {
					newCfg.Server.LogFormat = logFormat
				}

				// Validate new configuration
				if err := newCfg.Validate(); err != nil {
					log.Error().Err(err).Msg("Invalid configuration, keeping current config")
					continue
				}

				// Reinitialize logging with new config
				if err := log.Init(log.Config{
					Level:  newCfg.Server.LogLevel,
					Format: newCfg.Server.LogFormat,
					Output: "stdout",
				}); err != nil {
					log.Error().Err(err).Msg("Failed to reinitialize logging")
				}

				// Reload library if path changed
				if newCfg.Server.LibraryPath != cfg.Server.LibraryPath {
					log.Info().Str("path", newCfg.Server.LibraryPath).Msg("Library path changed, reloading library")
					newLib, err := library.LoadLibrary(newCfg.Server.LibraryPath)
					if err != nil {
						log.Error().Err(err).Msg("Failed to reload library, keeping current library")
					} else {
						lib = newLib
						log.Info().
							Int("books", len(lib.Books)).
							Int("lists", len(lib.ComicLists)).
							Msg("Library reloaded successfully")
					}
				}

				// Update configuration
				cfg = newCfg
				log.Info().Msg("Configuration reloaded successfully")

			case os.Interrupt, syscall.SIGTERM:
				log.Info().Str("signal", sig.String()).Msg("Shutting down server")

				// Gracefully shutdown HTTP server
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := httpServer.Shutdown(shutdownCtx); err != nil {
					log.Error().Err(err).Msg("Failed to shutdown HTTP server gracefully")
				}

				return nil
			}

		case err := <-errorChan:
			log.Error().Err(err).Msg("Discovery listener error")

		case discovered := <-deviceChan:
			handleDiscoveredDevice(discovered, registry, syncManager, cfg, lib, ipLimiter, deviceLimiter, syncSemaphore)
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

func handleDiscoveredDevice(
	discovered device.DiscoveredDevice,
	registry *device.Registry,
	syncManager *syncstate.Manager,
	cfg *config.Config,
	lib *library.ComicLibrary,
	ipLimiter *ratelimit.IPLimiter,
	deviceLimiter *ratelimit.DeviceLimiter,
	syncSemaphore chan struct{},
) {
	logger := log.With().
		Str("ip", discovered.IPAddress).
		Str("device_key", discovered.Key).
		Logger()

	// Check IP rate limit first (before any processing)
	if ipLimiter != nil && !ipLimiter.Allow(discovered.IPAddress) {
		logger.Warn().
			Int("current_attempts", ipLimiter.GetAttemptCount(discovered.IPAddress)).
			Msg("Device connection rejected: IP rate limit exceeded")
		return
	}

	// Check if this device should be ignored by IP (early check)
	if shouldIgnoreDevice(discovered.IPAddress, "", "", cfg.Server.IgnoreDevices) {
		logger.Debug().Msg("Device ignored by IP filter")
		return
	}

	// Check if we already know this device
	if _, ok := registry.Get(discovered.Key); ok {
		// Device already registered, just update last seen timestamp
		logger.Debug().Msg("Device already registered, updating last seen")
		registry.UpdateLastSeen(discovered.Key)
		return
	}

	logger.Info().
		Bool("wants_sync", discovered.WantsSync).
		Msg("New device discovered")

	// Connect to device and read comicrack.ini
	client := protocol.NewClient(discovered.IPAddress, device.DevicePort)

	logger.Debug().Msg("Reading device info")
	iniData, err := client.ReadFile("comicrack.ini")
	if err != nil {
		logger.Error().Err(err).Msg("Failed to read device info")
		return
	}

	// Parse device info
	info, err := device.ParseINI(iniData)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to parse device info")
		return
	}

	// Validate device
	if err := info.Validate(); err != nil {
		logger.Error().Err(err).Msg("Device validation failed")
		return
	}

	// Update logger with device details
	logger = log.With().
		Str("ip", discovered.IPAddress).
		Str("device_id", info.ID).
		Str("device_name", info.Name).
		Logger()

	// Check if device should be ignored by ID or name
	if shouldIgnoreDevice(discovered.IPAddress, info.ID, info.Name, cfg.Server.IgnoreDevices) {
		logger.Info().Msg("Device ignored by filter")
		return
	}

	// Add to registry
	registry.Add(info, discovered.IPAddress)

	// Log device information
	bookLimit := info.BookLimit()
	logger.Info().
		Str("model", fmt.Sprintf("%s %s", info.Manufacturer, info.Model)).
		Str("edition", string(info.Edition)).
		Int("version", info.Version).
		Int("book_limit", bookLimit).
		Strs("capabilities", info.Capabilities).
		Int("total_devices", registry.Count()).
		Msg("Device registered successfully")

	// Handle sync request if device wants sync and auto-sync is enabled
	if discovered.WantsSync && cfg.Server.AutoSync {
		logger.Info().Msg("Device requested sync, starting automatic sync")
		if err := handleSyncRequest(info, discovered.IPAddress, cfg, lib, syncManager, deviceLimiter, syncSemaphore); err != nil {
			logger.Error().Err(err).Msg("Sync failed")
		}
	} else if discovered.WantsSync && !cfg.Server.AutoSync {
		logger.Info().Msg("Device requested sync but auto-sync is disabled")
	}
}

// applyDeviceConfig applies a device's sync configuration to a syncer
// This configures which smart lists to sync and their settings
func applyDeviceConfig(syncer *csync.Syncer, deviceConfig *config.DeviceConfig, lib *library.ComicLibrary) error {
	logger := log.With().Str("device_id", deviceConfig.DeviceID).Logger()

	// Collect all enabled smart lists
	var smartLists []*library.ComicListItem
	var listNames []string

	for _, listConfig := range deviceConfig.Lists {
		if !listConfig.Enabled {
			continue
		}

		// Lookup smart list by GUID
		smartList := config.FindListByGUID(lib, listConfig.ListID)
		if smartList == nil {
			return fmt.Errorf("smart list %s (ID: %s) not found in library", listConfig.ListName, listConfig.ListID)
		}

		smartLists = append(smartLists, smartList)
		listNames = append(listNames, listConfig.ListName)
	}

	// Set all filter lists (union of all lists)
	if len(smartLists) > 0 {
		if err := syncer.SetFilterLists(smartLists); err != nil {
			return fmt.Errorf("failed to set filter lists: %w", err)
		}

		logger.Info().
			Int("list_count", len(smartLists)).
			Strs("list_names", listNames).
			Msg("Applied multiple smart lists to syncer")
	}

	// Apply settings (use device default settings)
	// Note: Per-list settings are not currently supported with multi-list sync
	// All lists use the device's default settings
	settings := deviceConfig.DefaultSettings
	if settings == nil {
		settings = csync.DefaultSettings()
	}
	syncer.SetSettings(settings)

	return nil
}

// handleSyncRequest handles a sync request from a device
func handleSyncRequest(
	deviceInfo *device.Info,
	deviceIP string,
	cfg *config.Config,
	lib *library.ComicLibrary,
	syncManager *syncstate.Manager,
	deviceLimiter *ratelimit.DeviceLimiter,
	syncSemaphore chan struct{},
) error {
	deviceID := deviceInfo.ID
	logger := log.With().
		Str("device_id", deviceID).
		Str("device_ip", deviceIP).
		Logger()

	// Check device rate limit
	if deviceLimiter != nil && !deviceLimiter.Allow(deviceID) {
		availableTokens := deviceLimiter.GetAvailableTokens(deviceID)
		logger.Warn().
			Float64("available_tokens", availableTokens).
			Msg("Sync rejected: device rate limit exceeded")
		return fmt.Errorf("device rate limit exceeded (%.2f tokens available)", availableTokens)
	}

	// Start tracking sync state
	if err := syncManager.StartSync(deviceID, deviceIP, deviceInfo.Name); err != nil {
		// Device is already syncing
		logger.Warn().Msg("Sync rejected: device is already syncing")
		return err
	}

	// Ensure sync state is cleaned up on exit
	defer func() {
		// Check if sync is still active (might have been completed/failed already)
		if syncManager.IsDeviceSyncing(deviceID) {
			syncManager.AbortSync(deviceID)
		}
	}()

	// Acquire semaphore slot if connection limiting is enabled
	if syncSemaphore != nil {
		select {
		case syncSemaphore <- struct{}{}:
			// Got a slot, continue
			defer func() { <-syncSemaphore }()
			logger.Debug().
				Int("slots_available", cap(syncSemaphore)-len(syncSemaphore)).
				Msg("Acquired sync connection slot")
		default:
			// No slots available
			logger.Warn().
				Int("max_concurrent", cap(syncSemaphore)).
				Msg("Sync rejected: maximum concurrent connections reached")
			syncManager.FailSync(deviceID, "maximum concurrent connections reached")
			return fmt.Errorf("maximum concurrent connections (%d) reached", cap(syncSemaphore))
		}
	}

	logger.Info().Msg("Starting sync")

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
		logger.Info().Msg("No sync configuration found, syncing all books")
	}

	// Perform sync
	logger.Debug().Msg("Performing sync operation")
	result, err := syncer.PerformSync()
	if err != nil {
		// Check if this is a network error (disconnection/timeout)
		if protocol.IsNetworkError(err) {
			errorType := protocol.ErrorTypeString(err)
			logger.Warn().
				Str("error_type", errorType).
				Err(err).
				Msg("Device disconnected during sync")
			syncManager.FailSync(deviceID, fmt.Sprintf("device disconnected: %s", errorType))
			return fmt.Errorf("device disconnected (%s): %w", errorType, err)
		}

		// Other errors (logic, file I/O, etc.)
		logger.Error().Err(err).Msg("Sync failed with non-network error")
		syncManager.FailSync(deviceID, err.Error())
		return fmt.Errorf("sync failed: %w", err)
	}

	// Mark sync as completed
	syncManager.CompleteSync(deviceID, result.BooksAdded, result.BooksUpdated, result.BooksDeleted, len(result.Errors))

	// Log sync results
	syncLogger := logger.With().
		Int("books_added", result.BooksAdded).
		Int("books_updated", result.BooksUpdated).
		Int("books_deleted", result.BooksDeleted).
		Int("errors", len(result.Errors)).
		Logger()

	if len(result.Errors) > 0 {
		syncLogger.Warn().Msg("Sync completed with errors")
		for _, err := range result.Errors {
			logger.Error().Err(err).Msg("Sync error")
		}
	} else {
		syncLogger.Info().Msg("Sync completed successfully")
	}

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
	serverCmd.Flags().IntVar(&maxConcurrentConns, "max-concurrent", 0, "Maximum concurrent connections (0 = unlimited, default: 5)")
	serverCmd.Flags().IntVar(&maxConnectionsPerIP, "max-connections-per-ip", 0, "Max connection attempts per IP per window (0 = unlimited, default: 10)")
	serverCmd.Flags().IntVar(&maxRequestsPerDevice, "max-requests-per-device", 0, "Max requests per device per window (0 = unlimited, default: 100)")
	serverCmd.Flags().IntVar(&rateLimitWindowSeconds, "rate-limit-window", 0, "Rate limit window in seconds (default: 60)")
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
		maxConcurrentConnsSet = cmd.Flags().Changed("max-concurrent")
		maxConnectionsPerIPSet = cmd.Flags().Changed("max-connections-per-ip")
		maxRequestsPerDeviceSet = cmd.Flags().Changed("max-requests-per-device")
		rateLimitWindowSecondsSet = cmd.Flags().Changed("rate-limit-window")
		logLevelSet = cmd.Flags().Changed("log-level")
		logFormatSet = cmd.Flags().Changed("log-format")
		return nil
	}
}
