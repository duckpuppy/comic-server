package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/duckpuppy/comic-server/internal/api"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/covers"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/komga"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/protocol"
	"github.com/duckpuppy/comic-server/internal/ratelimit"
	"github.com/duckpuppy/comic-server/internal/storage"
	csync "github.com/duckpuppy/comic-server/internal/sync"
	"github.com/duckpuppy/comic-server/internal/syncstate"
	"github.com/duckpuppy/comic-server/internal/trash"
	"github.com/duckpuppy/comic-server/internal/websocket"
	"github.com/spf13/cobra"
)

var (
	// CLI flag values (when explicitly provided, override config file)
	serverPort             int
	discoveryPort          int
	libraryPath            string
	dbPath                 string
	coverCacheDir          string
	ignoreDevices          []string
	bindAddress            string
	autoSync               bool
	maxConcurrentConns     int
	maxConnectionsPerIP    int
	maxRequestsPerDevice   int
	rateLimitWindowSeconds int
	logLevel               string
	logFormat              string
	pingDevice             string

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
	pingDeviceSet             bool
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
	if dbPath != "" {
		cfg.Server.DatabasePath = dbPath
	}
	if coverCacheDir != "" {
		cfg.Server.CoverCacheDir = coverCacheDir
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

	// Check that a library source is configured
	if cfg.Server.LibraryPath == "" && cfg.Server.DatabasePath == "" {
		return fmt.Errorf("library path is required (set via --library or --db flag, config file, or COMIC_SERVER_LIBRARY_PATH / COMIC_SERVER_DATABASE_PATH environment variable)")
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

	// Open config.db - a small, always-open database for record-shaped
	// config (device registrations, list assignments, Komga targets),
	// independent of which library backend is active below. See
	// comic-server-745 for the design record, comic-server-ihb for this
	// foundation issue, and comic-server-3ek for the device/list tables.
	//
	// Deliberately placed next to configPath (the resolved --config file),
	// NOT config.GetConfigDir()'s XDG default - GetConfigDir() ignores
	// --config entirely, so a container invoked with
	// `--config /config/config.yaml` (comic-server's own Docker image) was
	// putting config.db under $HOME/.config/comic-server instead, a path
	// with no volume mount at all. That's the container's throwaway
	// filesystem: every restart silently wiped device registrations, list
	// assignments, and Komga targets, discovered 2026-08-26 when
	// mediaserver's real device data survived only by chance until this
	// was caught (see comic-server-3ek/cde's one-time migration, which by
	// then had already cleared the config.yaml fields it moved out of).
	configDBPath := filepath.Join(filepath.Dir(configPath), "config.db")
	configDB, err := configdb.Open(configDBPath)
	if err != nil {
		return fmt.Errorf("failed to open config database: %w", err)
	}
	defer configDB.Close()
	log.Info().Str("path", configDBPath).Msg("Config database opened")

	// One-time migration: import any devices/list assignments still sitting
	// in config.yaml (the pre-comic-server-3ek storage) into config.db, then
	// stop config.yaml from carrying them going forward. Only runs when
	// config.db's devices table is empty, so it's safe to leave in
	// permanently rather than a one-shot flag - a fresh install with no
	// config.yaml devices just skips it, and it never re-imports once
	// config.db has at least one device (even if that device was later
	// unregistered, leaving config.db empty again - re-importing stale
	// config.yaml data at that point would be wrong, not helpful).
	if len(cfg.Devices) > 0 {
		existing, err := configDB.ListDevices()
		if err != nil {
			return fmt.Errorf("failed to check config database for existing devices: %w", err)
		}
		if len(existing) == 0 {
			migrated, err := migrateDevicesToConfigDB(cfg, configDB)
			if err != nil {
				return fmt.Errorf("failed to migrate devices to config database: %w", err)
			}
			log.Info().Int("devices", migrated).Msg("Migrated device registrations and list assignments from config.yaml to config.db")

			cfg.Devices = nil
			if err := config.Save(cfg, configPath); err != nil {
				log.Error().Err(err).Msg("Failed to save config.yaml after migrating devices to config.db")
			}
		}
	}

	// One-time migration: import any Komga targets still sitting in
	// config.yaml (the pre-comic-server-cde storage) into config.db, then
	// stop config.yaml from carrying them going forward. Same guard
	// rationale as the device migration above: only runs when config.db's
	// komga_targets table is empty, safe to leave in permanently.
	if len(cfg.Server.Komga.Targets) > 0 {
		existing, err := configDB.ListKomgaTargets()
		if err != nil {
			return fmt.Errorf("failed to check config database for existing komga targets: %w", err)
		}
		if len(existing) == 0 {
			migrated, err := migrateKomgaTargetsToConfigDB(cfg, configDB)
			if err != nil {
				return fmt.Errorf("failed to migrate komga targets to config database: %w", err)
			}
			log.Info().Int("targets", migrated).Msg("Migrated Komga targets from config.yaml to config.db")

			cfg.Server.Komga.Targets = nil
			if err := config.Save(cfg, configPath); err != nil {
				log.Error().Err(err).Msg("Failed to save config.yaml after migrating komga targets to config.db")
			}
		}
	}

	// One-time migration: import Server.ScanInfo (Scanners/Blacklist/
	// Prefix/Unknown/Enabled) still sitting in config.yaml into config.db,
	// then stop config.yaml from carrying it going forward
	// (comic-server-4ms - the first UI/API surface for this section,
	// previously config.yaml-hand-edit-only). scan_info is a single row
	// rather than a many-row table like devices/komga_targets, so "already
	// migrated" is "a row exists" instead of "the table is non-empty" -
	// same idea, just phrased for a singleton.
	if cfg.Server.ScanInfo.Enabled || len(cfg.Server.ScanInfo.Scanners) > 0 || len(cfg.Server.ScanInfo.Blacklist) > 0 {
		existing, err := configDB.GetScanInfo()
		if err != nil {
			return fmt.Errorf("failed to check config database for existing scan info: %w", err)
		}
		if existing == nil {
			if err := configDB.UpsertScanInfo(cfg.Server.ScanInfo); err != nil {
				return fmt.Errorf("failed to migrate scan info to config database: %w", err)
			}
			log.Info().Msg("Migrated scan_info config from config.yaml to config.db")

			cfg.Server.ScanInfo = config.ScanInfoConfig{}
			if err := config.Save(cfg, configPath); err != nil {
				log.Error().Err(err).Msg("Failed to save config.yaml after migrating scan info to config.db")
			}
		}
	}

	// Load library using appropriate backend
	var backend library.Backend
	if cfg.Server.DatabasePath != "" {
		// Use SQLite backend
		log.Info().Str("path", cfg.Server.DatabasePath).Msg("Loading library from SQLite database")
		sqliteBackend, err := storage.NewSQLiteBackend(cfg.Server.DatabasePath, cfg.Server.LibraryPath)
		if err != nil {
			return fmt.Errorf("failed to open SQLite database: %w", err)
		}
		backend = sqliteBackend
		defer backend.Close()

		if cfg.Server.LibraryPath == "" {
			log.Warn().Msg("No --library path configured alongside --db - this database will not stay in sync with external ComicRack changes; reimport manually and restart to pick up updates")
		}

		log.Info().
			Int("books", backend.BookCount()).
			Str("library_id", backend.LibraryID()).
			Msg("SQLite library loaded successfully")
	} else {
		// Use XML backend (default)
		log.Debug().Str("path", cfg.Server.LibraryPath).Msg("Loading comic library")
		flushInterval := time.Duration(cfg.Server.LibraryCacheFlushIntervalSec) * time.Second
		xmlBackend, err := library.NewXMLBackend(cfg.Server.LibraryPath, flushInterval)
		if err != nil {
			return fmt.Errorf("failed to load library: %w", err)
		}
		backend = xmlBackend
		defer backend.Close()

		log.Info().
			Int("books", backend.BookCount()).
			Str("library_id", backend.LibraryID()).
			Msg("Library loaded successfully")

		if flushInterval > 0 {
			log.Info().
				Dur("flush_interval", flushInterval).
				Msg("Library cache enabled with automatic flushing")
		} else {
			log.Info().Msg("Library cache enabled with manual flushing only")
		}
	}

	// Create device registry
	registry := device.NewRegistry()

	// Create sync state manager (max 100 history entries), persisted to
	// config.db so history survives a restart (comic-server-7vu)
	syncManager, err := syncstate.NewManagerWithStore(100, configDB)
	if err != nil {
		return fmt.Errorf("create sync state manager: %w", err)
	}

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
	listener, err := device.NewDiscoveryListener(cfg.Server.DiscoveryPort)
	if err != nil {
		return fmt.Errorf("failed to start discovery listener: %w", err)
	}
	defer listener.Stop()

	log.Info().
		Str("multicast_group", device.MulticastGroup).
		Int("port", cfg.Server.DiscoveryPort).
		Msg("Listening for device broadcasts")

	// Start listening
	deviceChan, errorChan := listener.Start()

	// Create context for direct ping loop
	pingCtx, pingCancel := context.WithCancel(context.Background())
	defer pingCancel()

	// Send direct ping to device if specified (useful for WSL2, VPNs, complex networks)
	if pingDeviceSet && pingDevice != "" {
		go sendDirectPingAndRegister(pingCtx, pingDevice, registry, syncManager, cfg, backend, configDB, ipLimiter, deviceLimiter, syncSemaphore)
	}

	// Create and start WebSocket hub
	wsHub := websocket.NewHub()
	go wsHub.Run()
	log.Info().Msg("WebSocket hub started")

	// Start REST API HTTP server
	apiVersion := api.VersionInfo{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
	}
	apiServer := api.NewServer(syncManager, registry, backend, cfg, configPath, apiVersion, wsHub, configDB)
	apiServer.SetSyncTrigger(func(deviceID string) error {
		return triggerManualSync(deviceID, registry, syncManager, cfg, backend, configDB, deviceLimiter, syncSemaphore)
	})

	if cfg.Server.ComicVineAPIKey != "" {
		if err := wireScraperAPI(apiServer, cfg.Server.ComicVineAPIKey); err != nil {
			log.Warn().Err(err).Msg("Failed to enable ComicVine scraper API endpoints")
		}
	}

	if resolvedCoverCacheDir, err := resolveCoverCacheDir(cfg.Server.CoverCacheDir); err != nil {
		log.Warn().Err(err).Msg("Failed to determine cover cache directory; cover images will be extracted on every request without caching or resizing")
	} else if coverCache, err := covers.NewCache(resolvedCoverCacheDir, covers.DefaultThumbnailWidth); err != nil {
		log.Warn().Err(err).Str("dir", resolvedCoverCacheDir).Msg("Failed to initialize cover thumbnail cache; cover images will be extracted on every request without caching or resizing")
	} else {
		apiServer.SetCoverCache(coverCache)
		log.Info().Str("dir", resolvedCoverCacheDir).Msg("Cover thumbnail cache enabled")
	}

	var komgaStatus *komga.StatusStore
	if cfg.Server.Komga.Enabled {
		komgaStatus = komga.NewStatusStore()
		apiServer.SetKomgaStatus(komgaStatus)
	}

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
		log.Info().Msgf("  POST http://localhost:%d/api/scrape - Start a ComicVine scrape job", cfg.Server.ServerPort)
		log.Info().Msgf("  GET  http://localhost:%d/api/scrape/status - Scrape job progress", cfg.Server.ServerPort)
		log.Info().Msgf("  GET  http://localhost:%d/api/scrape/review - Books pending manual review", cfg.Server.ServerPort)
		log.Info().Msgf("  GET  http://localhost:%d/api/komga/status - Komga sync status", cfg.Server.ServerPort)
		log.Info().Msgf("  GET  http://localhost:%d/metrics - Prometheus metrics", cfg.Server.ServerPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("REST API server error")
		}
	}()

	// Start ComicVine background sync if API key is configured
	cvCtx, cvCancel := context.WithCancel(context.Background())
	defer cvCancel()
	if cfg.Server.ComicVineAPIKey != "" {
		go startComicVineSync(cvCtx, cfg.Server.ComicVineAPIKey, backend)
	}

	// Start Komga collection/read-list sync if enabled
	komgaCtx, komgaCancel := context.WithCancel(context.Background())
	defer komgaCancel()
	var komgaSyncer *komga.Syncer
	if cfg.Server.Komga.Enabled {
		// Built even with zero enabled targets, so the web UI's Komga target
		// management endpoints (comic-server-d3w) always have a live Syncer
		// to push newly-added targets into via SetTargets/TriggerNow -
		// otherwise the first target added after startup would need a
		// restart to take effect.
		komgaSyncer, err = buildKomgaSyncer(cfg.Server.Komga, configDB, backend)
		if err != nil {
			return err
		}
		apiServer.SetKomgaSyncer(komgaSyncer)
		go startKomgaSync(komgaCtx, komgaSyncer, cfg.Server.Komga, komgaStatus)
	}

	// Start the trash background sweep if a quarantine directory is
	// configured - independent of any specific feature being enabled,
	// since internal/trash is generic infra (currently used by
	// comic-server-43b's CBZ conversion, potentially others later). Purges
	// quarantined files older than TrashRetentionDays; see comic-server-1up.
	trashCtx, trashCancel := context.WithCancel(context.Background())
	defer trashCancel()
	if cfg.Server.TrashPath != "" {
		tr, err := trash.New(cfg.Server.TrashPath, cfg.Server.TrashRetentionDays)
		if err != nil {
			log.Error().Err(err).Msg("Invalid trash configuration, background sweep not started")
		} else {
			go tr.Run(trashCtx, trash.DefaultSweepInterval, func(result trash.SweepResult) {
				if result.Removed > 0 || len(result.Errs) > 0 {
					log.Info().Int("removed", result.Removed).Int("errors", len(result.Errs)).Msg("Trash sweep completed")
				}
				for _, e := range result.Errs {
					log.Warn().Err(e).Msg("Trash sweep error")
				}
			})
		}
	}

	// Watch the library source file for external changes (e.g. ComicRack
	// saving ComicDb.xml) and reload automatically - for the XML backend
	// that means re-reading the file in place; for the SQLite backend it
	// means re-importing it into the database (see SQLiteBackend.Reload).
	// Requires both a real file path and a backend that supports reload -
	// an XMLBackend built without a path, or a SQLiteBackend opened
	// without a known XML source, can't reload either way.
	if reloadable, ok := backend.(library.ReloadableBackend); ok && cfg.Server.LibraryPath != "" {
		watcher, err := library.NewWatcher(reloadable, cfg.Server.LibraryPath)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to start library file watcher; library changes will require a restart to pick up")
		} else {
			watcher.OnReload(func() {
				apiServer.InvalidateListCache()
				wsHub.Broadcast(websocket.EventLibraryReloaded, map[string]any{
					"book_count": reloadable.BookCount(),
				})
				if komgaSyncer != nil {
					komgaSyncer.TriggerNow()
				}
			})
			watcherCtx, watcherCancel := context.WithCancel(context.Background())
			defer watcherCancel()
			go watcher.Run(watcherCtx)
		}
	}

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

				// Note: Library reload on SIGHUP is not currently supported with the backend interface.
				// To change the library path, restart the server.
				if newCfg.Server.LibraryPath != cfg.Server.LibraryPath {
					log.Warn().
						Str("old_path", cfg.Server.LibraryPath).
						Str("new_path", newCfg.Server.LibraryPath).
						Msg("Library path changed in config - server restart required to apply")
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
			handleDiscoveredDevice(discovered, registry, syncManager, cfg, backend, configDB, ipLimiter, deviceLimiter, syncSemaphore)
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
	backend library.Backend,
	configDB *configdb.DB,
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
	if dev, ok := registry.Get(discovered.Key); ok {
		// Device already in registry, update last seen timestamp
		logger.Debug().Msg("Device already in registry, updating last seen")
		registry.UpdateLastSeen(discovered.Key)

		// Handle sync request if device wants sync (manual button press on device)
		if discovered.WantsSync {
			logger.Info().Msg("Known device requesting sync (user pressed sync button)")
			if err := handleSyncRequest(dev.Info, discovered.IPAddress, cfg, backend, configDB, syncManager, deviceLimiter, syncSemaphore); err != nil {
				logger.Error().Err(err).Msg("Sync failed")
			}
			return
		}

		// If device is registered and not requesting sync, send pong to make sync button appear
		registeredDevice, err := configDB.GetDevice(dev.Info.ID)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to look up device registration in config database")
		}
		if registeredDevice != nil {
			logger.Debug().Msg("Sending ClientPong to registered device")
			client := protocol.NewClient(discovered.IPAddress, device.DevicePort)
			if err := client.SendClientPong(); err != nil {
				logger.Debug().Err(err).Msg("Failed to send ClientPong (device may be offline)")
			} else {
				logger.Debug().Msg("ClientPong sent successfully")
			}
		}
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
		if err := handleSyncRequest(info, discovered.IPAddress, cfg, backend, configDB, syncManager, deviceLimiter, syncSemaphore); err != nil {
			logger.Error().Err(err).Msg("Sync failed")
		}
	} else if discovered.WantsSync && !cfg.Server.AutoSync {
		logger.Info().Msg("Device requested sync but auto-sync is disabled")
	}
}

// migrateDevicesToConfigDB imports every device registration and list
// assignment from cfg.Devices (the pre-comic-server-3ek config.yaml
// storage) into configDB. Called once at startup, only when configDB has
// no devices yet - see runServer's call site for why that guard makes
// this safe to leave in permanently.
func migrateDevicesToConfigDB(cfg *config.Config, configDB *configdb.DB) (int, error) {
	count := 0
	for deviceID, dc := range cfg.Devices {
		if err := configDB.UpsertDevice(deviceID, dc.FriendlyName, dc.LastSeen, dc.DefaultSettings); err != nil {
			return count, fmt.Errorf("migrate device %s: %w", deviceID, err)
		}
		for _, lc := range dc.Lists {
			dl := configdb.DeviceList{
				ListID:   lc.ListID,
				ListName: lc.ListName,
				Enabled:  lc.Enabled,
				Settings: lc.Settings,
			}
			if err := configDB.AddDeviceList(deviceID, dl); err != nil {
				return count, fmt.Errorf("migrate device %s list %s: %w", deviceID, lc.ListID, err)
			}
		}
		count++
	}
	return count, nil
}

// migrateKomgaTargetsToConfigDB copies every Komga target from
// cfg.Server.Komga.Targets into config.db's komga_targets table.
func migrateKomgaTargetsToConfigDB(cfg *config.Config, configDB *configdb.DB) (int, error) {
	count := 0
	for _, t := range cfg.Server.Komga.Targets {
		target := configdb.KomgaTarget{
			ListID:         t.ListID,
			ListName:       t.ListName,
			Type:           string(t.Type),
			KomgaName:      t.KomgaName,
			Enabled:        t.Enabled,
			SyncReadStatus: t.SyncReadStatus,
		}
		if err := configDB.CreateKomgaTarget(target); err != nil {
			return count, fmt.Errorf("migrate komga target %s: %w", t.ListID, err)
		}
		count++
	}
	return count, nil
}

// applyDeviceConfig applies a device's sync configuration to a syncer
// This configures which lists to sync and their settings
func applyDeviceConfig(syncer *csync.Syncer, deviceConfig *configdb.Device, backend library.Backend) error {
	logger := log.With().Str("device_id", deviceConfig.DeviceID).Logger()

	// Collect all enabled lists - any list type with real book membership
	// works (smart list, ID list, reading list; see
	// (*csync.Syncer).SetFilterLists / ComputeSyncPlan's use of
	// GetBooksForList), not just smart lists. Originally restricted to
	// smart lists only under the mistaken belief that this reflected a
	// real ComicRack wireless-sync protocol constraint; confirmed against
	// ComicRackCE's own source that no such restriction exists - relaxed
	// 2026-08-26 after a real "To Read" ID list assigned to a device made
	// the device's ENTIRE sync fail outright (not just that one list -
	// this error aborts handleSyncRequest), same class of bug as
	// comic-server-vwl's Komga-target fix.
	var entries []csync.FilterListEntry
	var listNames []string

	for _, listConfig := range deviceConfig.Lists {
		if !listConfig.Enabled {
			continue
		}

		// Lookup list by GUID (uses recursive search for nested folders)
		list, err := backend.FindListByID(listConfig.ListID)
		if err != nil {
			return fmt.Errorf("error looking up list %s: %w", listConfig.ListName, err)
		}
		if list == nil || strings.Contains(list.Type, "Folder") {
			return fmt.Errorf("list %s (ID: %s) not found in library, or is a folder", listConfig.ListName, listConfig.ListID)
		}

		entries = append(entries, csync.FilterListEntry{List: list, Settings: listConfig.Settings})
		listNames = append(listNames, listConfig.ListName)
	}

	// Apply the device's default settings as the fallback for any list
	// with no override of its own (see FilterListEntry.Settings).
	defaultSettings := deviceConfig.DefaultSettings
	if defaultSettings == nil {
		defaultSettings = csync.DefaultSettings()
	}
	syncer.SetSettings(defaultSettings)

	// Set all filter lists (union of all lists) - each list's own
	// settings (only-unread, limit, sort, etc.) are applied to that
	// list's own books before the union, instead of being discarded in
	// favor of one shared settings object (comic-server-3oq).
	if len(entries) > 0 {
		if err := syncer.SetFilterListsWithSettings(entries); err != nil {
			return fmt.Errorf("failed to set filter lists: %w", err)
		}

		logger.Info().
			Int("list_count", len(entries)).
			Strs("list_names", listNames).
			Msg("Applied multiple lists to syncer")
	}

	return nil
}

// triggerManualSync starts a sync for deviceID immediately, if it's
// currently connected and not already syncing (comic-server-yfp) -
// letting a user force a sync from the web UI instead of waiting for
// auto-sync or the device's own sync button. Backs api.Server's
// SetSyncTrigger callback.
//
// The two pre-checks (device connected, not already syncing) happen
// synchronously so an HTTP caller gets an immediate, specific error; the
// sync itself runs in a background goroutine rather than blocking the
// caller for however long the sync takes, mirroring how a discovery-loop-
// triggered sync (handleDiscoveredDevice) already never blocks anything
// else in the running server for that request's own success/failure -
// this just gets that same async result for a REST-triggered sync too.
func triggerManualSync(
	deviceID string,
	registry *device.Registry,
	syncManager *syncstate.Manager,
	cfg *config.Config,
	backend library.Backend,
	configDB *configdb.DB,
	deviceLimiter *ratelimit.DeviceLimiter,
	syncSemaphore chan struct{},
) error {
	dev, ok := registry.Get(deviceID)
	if !ok {
		return device.ErrNotConnected
	}

	if syncManager.IsDeviceSyncing(deviceID) {
		return &syncstate.DeviceAlreadySyncingError{DeviceID: deviceID}
	}

	logger := log.With().
		Str("device_id", deviceID).
		Str("device_ip", dev.IPAddress).
		Logger()
	logger.Info().Msg("Manual sync triggered via API")

	go func() {
		if err := handleSyncRequest(dev.Info, dev.IPAddress, cfg, backend, configDB, syncManager, deviceLimiter, syncSemaphore); err != nil {
			logger.Error().Err(err).Msg("Manually triggered sync failed")
		}
	}()

	return nil
}

// handleSyncRequest handles a sync request from a device
func handleSyncRequest(
	deviceInfo *device.Info,
	deviceIP string,
	cfg *config.Config,
	backend library.Backend,
	configDB *configdb.DB,
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

	// Create syncer with backend
	syncer := csync.NewSyncer(client, backend)
	syncer.SetPathResolver(cfg.ResolveLibraryFilePath)
	syncer.SetStatusDetailCallback(func(detail string) {
		syncManager.SetDetail(deviceID, detail)
	})
	syncer.SetProgressCallback(func(percent, total, added, updated, deleted, errorCount int) {
		syncManager.UpdateProgress(deviceID, percent, total, added, updated, deleted, errorCount)
	})

	// Apply device config if exists
	deviceConfig, err := configDB.GetDevice(deviceID)
	if err != nil {
		return fmt.Errorf("failed to look up device config: %w", err)
	}
	if deviceConfig != nil {
		if err := applyDeviceConfig(syncer, deviceConfig, backend); err != nil {
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

// sendDirectPingAndRegister sends periodic discovery pings directly to a device IP address
// and registers the device on first contact. This is useful for environments where multicast
// discovery is unreliable (WSL2, VPNs, complex network setups, firewalls blocking multicast, etc.)
// Pings are sent every 30 seconds to keep the sync button visible on the device
func sendDirectPingAndRegister(
	ctx context.Context,
	address string,
	registry *device.Registry,
	syncManager *syncstate.Manager,
	cfg *config.Config,
	backend library.Backend,
	configDB *configdb.DB,
	ipLimiter *ratelimit.IPLimiter,
	deviceLimiter *ratelimit.DeviceLimiter,
	syncSemaphore chan struct{},
) {
	// Parse IP:PORT, default port is 7614 (device port)
	deviceIP := address
	devicePort := 7614

	// Check if port is specified
	if idx := strings.LastIndex(address, ":"); idx != -1 {
		portStr := address[idx+1:]
		deviceIP = address[:idx]

		// Try to parse port
		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil {
			devicePort = port
		} else {
			log.Warn().
				Str("address", address).
				Err(err).
				Msg("Failed to parse port, using default 7614")
		}
	}

	// Create protocol client for the device
	client := protocol.NewClient(deviceIP, devicePort)

	// Send initial ping immediately
	log.Info().
		Str("ip", deviceIP).
		Int("port", devicePort).
		Msg("Sending direct discovery ping to device")

	if err := client.SendClientPong(); err != nil {
		log.Error().
			Err(err).
			Str("ip", deviceIP).
			Int("port", devicePort).
			Msg("Failed to send direct ping to device")
	} else {
		log.Info().
			Str("ip", deviceIP).
			Int("port", devicePort).
			Msg("Successfully sent direct ping to device")

		// Try to register the device by reading comicrack.ini
		iniData, err := client.ReadFile("comicrack.ini")
		if err != nil {
			log.Warn().
				Err(err).
				Str("ip", deviceIP).
				Int("port", devicePort).
				Msg("Failed to read device info for registration, will retry on periodic pings")
		} else {
			// Parse device info
			deviceInfo, err := device.ParseINI(iniData)
			if err != nil {
				log.Warn().
					Err(err).
					Str("ip", deviceIP).
					Int("port", devicePort).
					Msg("Failed to parse device info")
			} else {
				// Create a discovered device and register it
				discovered := device.DiscoveredDevice{
					Key:       deviceInfo.ID + ":ComicRack",
					IPAddress: deviceIP,
				}
				log.Info().
					Str("ip", deviceIP).
					Str("device_id", deviceInfo.ID).
					Str("device_name", deviceInfo.Name).
					Msg("Registering device from direct ping")

				handleDiscoveredDevice(discovered, registry, syncManager, cfg, backend, configDB, ipLimiter, deviceLimiter, syncSemaphore)

				// In direct-ping mode, automatically trigger sync since device can't signal back via UDP
				// Check if device is configured for sync (has smart lists assigned)
				deviceCfg, err := configDB.GetDevice(deviceInfo.ID)
				if err != nil {
					log.Error().Err(err).Str("device_id", deviceInfo.ID).Msg("Failed to look up device config")
				}
				if deviceCfg != nil && len(deviceCfg.Lists) > 0 {
					log.Info().
						Str("device_id", deviceInfo.ID).
						Int("lists", len(deviceCfg.Lists)).
						Msg("Device has smart lists configured, triggering automatic sync")

					if err := handleSyncRequest(deviceInfo, deviceIP, cfg, backend, configDB, syncManager, deviceLimiter, syncSemaphore); err != nil {
						log.Error().
							Err(err).
							Str("device_id", deviceInfo.ID).
							Msg("Automatic sync failed")
					}
				} else {
					log.Info().
						Str("device_id", deviceInfo.ID).
						Msg("Device registered but no smart lists configured - skipping automatic sync")
				}
			}
		}
	}

	// Send periodic pings every 10 seconds to keep sync button visible
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().
				Str("ip", deviceIP).
				Int("port", devicePort).
				Msg("Stopping direct ping loop")
			return
		case <-ticker.C:
			if err := client.SendClientPong(); err != nil {
				log.Warn().
					Err(err).
					Str("ip", deviceIP).
					Int("port", devicePort).
					Msg("Failed to send periodic ping to device")
			} else {
				log.Debug().
					Str("ip", deviceIP).
					Int("port", devicePort).
					Msg("Sent periodic ping to device")
			}
		}
	}
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 0, "Server control port (TCP, default: 7620)")
	serverCmd.Flags().IntVarP(&discoveryPort, "discovery-port", "d", 0, "Device discovery port (UDP multicast, default: 7615)")
	serverCmd.Flags().StringVarP(&libraryPath, "library", "l", "", "Path to ComicDB.xml file")
	serverCmd.Flags().StringVar(&dbPath, "db", "", "Path to SQLite database file (experimental, alternative to --library). Combine with --library to also keep the database in sync with that XML file via the file watcher")
	serverCmd.Flags().StringVar(&coverCacheDir, "cover-cache-dir", "", "Directory to cache resized comic cover thumbnails in (default: XDG cache dir). Set this to a path under a mounted volume in Docker so the cache survives container recreates")
	serverCmd.Flags().StringSliceVarP(&ignoreDevices, "ignore-device", "i", nil, "Devices to ignore (can be IP address, device ID, or device name)")
	serverCmd.Flags().StringVarP(&bindAddress, "bind", "b", "", "Network interface to bind to (default: all interfaces)")
	serverCmd.Flags().BoolVar(&autoSync, "auto-sync", false, "Automatically sync devices when they connect")
	serverCmd.Flags().IntVar(&maxConcurrentConns, "max-concurrent", 0, "Maximum concurrent connections (0 = unlimited, default: 5)")
	serverCmd.Flags().IntVar(&maxConnectionsPerIP, "max-connections-per-ip", 0, "Max connection attempts per IP per window (0 = unlimited, default: 10)")
	serverCmd.Flags().IntVar(&maxRequestsPerDevice, "max-requests-per-device", 0, "Max requests per device per window (0 = unlimited, default: 100)")
	serverCmd.Flags().IntVar(&rateLimitWindowSeconds, "rate-limit-window", 0, "Rate limit window in seconds (default: 60)")
	serverCmd.Flags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error (default: info)")
	serverCmd.Flags().StringVar(&logFormat, "log-format", "", "Log format: text, json (default: text)")
	serverCmd.Flags().StringVar(&pingDevice, "ping-device", "", "Send discovery ping directly to device IP[:PORT] (useful for WSL2, VPNs, complex networks)")

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
		pingDeviceSet = cmd.Flags().Changed("ping-device")
		return nil
	}
}

// wireScraperAPI opens a ComicVine client and cache for the lifetime of the
// server process and wires them into the API server's /api/scrape endpoints.
// The cache handle is intentionally not closed here; it's released when the
// process exits.
func wireScraperAPI(apiServer *api.Server, apiKey string) error {
	dataDir, err := config.EnsureDataDir()
	if err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	cachePath := filepath.Join(dataDir, "comicvine_cache.db")
	cache, err := comicvine.OpenCache(cachePath)
	if err != nil {
		return fmt.Errorf("open ComicVine cache: %w", err)
	}

	client := comicvine.NewClient(apiKey)
	apiServer.SetScraper(client, cache)
	return nil
}

func startComicVineSync(ctx context.Context, apiKey string, backend library.Backend) {
	dataDir, err := config.EnsureDataDir()
	if err != nil {
		log.Error().Err(err).Msg("ComicVine sync: failed to create data directory")
		return
	}

	cachePath := filepath.Join(dataDir, "comicvine_cache.db")
	cache, err := comicvine.OpenCache(cachePath)
	if err != nil {
		log.Error().Err(err).Msg("ComicVine sync: failed to open cache database")
		return
	}
	defer cache.Close()

	client := comicvine.NewClient(apiKey,
		comicvine.WithCircuitBreaker(comicvine.NewCircuitBreaker(
			comicvine.WithOnStateChange(func(from, to comicvine.CircuitState) {
				log.Info().
					Str("from", from.String()).
					Str("to", to.String()).
					Msg("ComicVine sync: circuit breaker state change")
			}),
		)),
	)

	syncer := comicvine.NewSyncer(client, cache)

	// Seed from library
	books, err := backend.GetAllBooks()
	if err != nil {
		log.Error().Err(err).Msg("ComicVine sync: failed to get books from library")
		return
	}

	stores := make([]string, len(books))
	for i := range books {
		stores[i] = books[i].CustomValuesStore
	}

	seeded, err := syncer.SeedFromLibrary(stores)
	if err != nil {
		log.Error().Err(err).Msg("ComicVine sync: failed to seed volumes")
		return
	}

	owned := comicvine.BuildOwnedCounts(stores)

	// Build and apply initial completeness data from any previously cached volumes
	updateCVData(backend, books, cache)

	synced, pending, failed, _ := cache.SyncStats()
	log.Info().
		Int("seeded_volumes", seeded).
		Int("synced", synced).
		Int("pending", pending).
		Int("failed", failed).
		Int("library_books", len(books)).
		Str("cache_path", cachePath).
		Msg("ComicVine sync: starting background sync")

	if err := syncer.Run(ctx, owned); err != nil && ctx.Err() == nil {
		log.Error().Err(err).Msg("ComicVine sync: background sync stopped with error")
	}

	// Final update after sync completes
	updateCVData(backend, books, cache)
}

// startKomgaSync runs komga.Syncer for the life of ctx, pushing each
// configured target's smart list result into Komga on cfg.SyncIntervalSec.
// comic-server has no way to detect ComicRack library changes while
// running (see comic-server-bwz), so this is a scheduled push, not
// change-triggered.
// buildKomgaSyncer translates config.KomgaConfig plus config.db's
// komga_targets table into a *komga.Syncer, skipping disabled targets.
// Always returns a non-nil Syncer when Komga sync is enabled - even with
// zero enabled targets - so the web UI's Komga target management
// endpoints (comic-server-d3w) always have a live Syncer to call
// SetTargets/TriggerNow on. Syncer.syncOnce is a no-op when there are no
// targets, so this doesn't cost any Komga API calls while idle. Split out
// from startKomgaSync so callers (e.g. the library watcher) can hold a
// reference to the syncer and call TriggerNow() on it.
func buildKomgaSyncer(cfg config.KomgaConfig, configDB *configdb.DB, backend library.Backend) (*komga.Syncer, error) {
	dbTargets, err := configDB.ListKomgaTargets()
	if err != nil {
		return nil, fmt.Errorf("failed to load komga targets from config database: %w", err)
	}
	targets := komgaTargetsFromConfigDB(dbTargets)
	if len(targets) == 0 {
		log.Warn().Msg("Komga sync: enabled but no enabled targets configured yet")
	}

	return komga.NewSyncer(backend, komga.SyncOptions{
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		LocalRoot:  cfg.LocalRoot,
		RemoteRoot: cfg.RemoteRoot,
		Targets:    targets,
		Interval:   time.Duration(cfg.SyncIntervalSec) * time.Second,
	}), nil
}

// komgaTargetsFromConfigDB translates enabled configdb.KomgaTarget entries
// into komga.Target, skipping disabled targets and logging (then skipping)
// any with an unrecognized Type. Used both at startup (buildKomgaSyncer)
// and by the API server after a Komga target is added/updated/removed via
// the web UI (comic-server-d3w) to rebuild the live Syncer's target set.
// resolveCoverCacheDir returns configuredDir if set, else the "covers"
// subdirectory of the XDG cache directory. Configurable because the XDG
// cache dir isn't one of the Docker image's declared volumes (/config,
// /data, /comics) - Docker deployments should set cover_cache_dir (or
// --cover-cache-dir / COMIC_SERVER_COVER_CACHE_DIR) to a path under a
// mounted volume so the cache survives container recreates, though as a
// cache it's fine (if slower on the next request) if it's lost.
func resolveCoverCacheDir(configuredDir string) (string, error) {
	if configuredDir != "" {
		return configuredDir, nil
	}
	cacheDir, err := config.GetCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "covers"), nil
}

func komgaTargetsFromConfigDB(dbTargets []configdb.KomgaTarget) []komga.Target {
	targets := make([]komga.Target, 0, len(dbTargets))
	for _, t := range dbTargets {
		if !t.Enabled {
			continue
		}
		var targetType komga.TargetType
		switch config.KomgaTargetType(t.Type) {
		case config.KomgaTargetCollection:
			targetType = komga.TargetCollection
		case config.KomgaTargetReadList:
			targetType = komga.TargetReadList
		default:
			log.Error().Str("list_id", t.ListID).Str("type", t.Type).Msg("Komga sync: unknown target type, skipping")
			continue
		}
		targets = append(targets, komga.Target{
			ListID:         t.ListID,
			KomgaName:      t.KomgaName,
			Type:           targetType,
			SyncReadStatus: t.SyncReadStatus,
		})
	}
	return targets
}

// startKomgaSync runs syncer for the life of ctx, recording results into
// status and logging matched/unmatched counts and errors.
func startKomgaSync(ctx context.Context, syncer *komga.Syncer, cfg config.KomgaConfig, status *komga.StatusStore) {
	log.Info().
		Int("interval_sec", cfg.SyncIntervalSec).
		Str("base_url", cfg.BaseURL).
		Msg("Komga sync: starting background sync")

	syncer.Run(ctx, func(r komga.TargetResult) {
		status.Record(r)

		// An empty Target.ListID means BuildIndex itself failed (see
		// Syncer.syncOnce) - that isn't a per-target push failure.
		if r.Target.ListID == "" {
			if r.Err != nil {
				log.Error().Err(r.Err).Msg("Komga sync: failed to build Komga path index, skipping this pass")
			}
			return
		}

		logger := log.With().Str("komga_name", r.Target.KomgaName).Str("list_id", r.Target.ListID).Logger()
		if r.Err != nil {
			logger.Error().Err(r.Err).Msg("Komga sync: target push failed")
			return
		}
		event := logger.Info().Int("matched", r.MatchedCount)
		if len(r.Unmatched) > 0 {
			event = event.Int("unmatched", len(r.Unmatched))
		}
		event.Msg("Komga sync: target pushed")

		for _, u := range r.Unmatched {
			logger.Warn().
				Str("book_id", u.Book.ID).
				Str("file_path", u.Book.FilePath).
				Str("reason", u.Reason).
				Msg("Komga sync: book skipped (unmatched)")
		}
	})
}

func updateCVData(backend library.Backend, books []library.ComicBook, cache *comicvine.Cache) {
	cvData, err := comicvine.BuildCompletenessMap(books, cache)
	if err != nil {
		log.Error().Err(err).Msg("ComicVine sync: failed to build completeness map")
		return
	}
	switch b := backend.(type) {
	case *library.XMLBackend:
		b.SetCVData(cvData)
		log.Info().Int("books_with_cv_data", len(cvData)).Msg("ComicVine sync: completeness data updated")
	case *storage.SQLiteBackend:
		b.SetCVData(cvData)
		log.Info().Int("books_with_cv_data", len(cvData)).Msg("ComicVine sync: completeness data updated")
	default:
		log.Warn().Msg("ComicVine sync: CV-based smart list matchers are unsupported on this backend type")
	}
}
