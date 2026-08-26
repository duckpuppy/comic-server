package cmd

import (
	"fmt"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/sync"
	"github.com/spf13/cobra"
)

var addListCmd = &cobra.Command{
	Use:   "add-list <device> <list-name>",
	Short: "Add a smart list to a device's sync configuration",
	Long: `Add a smart list to a device's sync configuration.
The device can be specified by friendly name or device ID.
The list name must match a smart list in the library.

Sync options can be specified with flags. If not specified,
the device's default settings will be used.`,
	Args: cobra.ExactArgs(2),
	RunE: runAddList,
}

var (
	addListLibraryPath string

	// Sync option flags
	onlyUnread        bool
	keepLastRead      bool
	keepLastReadCount int
	onlyChecked       bool
	limit             int
	limitType         string
	sortType          string
)

func runAddList(cmd *cobra.Command, args []string) error {
	deviceNameOrID := args[0]
	listName := args[1]

	// Load library
	lib, err := library.LoadLibrary(addListLibraryPath)
	if err != nil {
		return fmt.Errorf("failed to load library: %w", err)
	}

	// Resolve smart list name to GUID
	listID, resolvedName, err := config.ResolveSmartList(lib, listName)
	if err != nil {
		return err
	}

	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Resolve device (or create if doesn't exist)
	device, err := resolveConfigDBDevice(db, deviceNameOrID)
	if err != nil {
		// Device not found - create new device entry.
		// Assume the input is a device ID.
		deviceID := deviceNameOrID
		if err := db.UpsertDevice(deviceID, "", time.Time{}, nil); err != nil {
			return fmt.Errorf("failed to create device: %w", err)
		}
		device, err = db.GetDevice(deviceID)
		if err != nil {
			return fmt.Errorf("failed to load new device: %w", err)
		}
		fmt.Printf("Created new device configuration for: %s\n", deviceID)
	}

	// Parse sync options
	settings, err := parseSyncOptions(cmd)
	if err != nil {
		return err
	}

	for _, l := range device.Lists {
		if l.ListID == listID {
			return fmt.Errorf("list %q is already configured for this device", resolvedName)
		}
	}

	if err := db.AddDeviceList(device.DeviceID, configdb.DeviceList{
		ListID:   listID,
		ListName: resolvedName,
		Enabled:  true,
		Settings: settings,
	}); err != nil {
		return fmt.Errorf("failed to add list to device: %w", err)
	}

	deviceName := device.FriendlyName
	if deviceName == "" {
		deviceName = device.DeviceID
	}

	fmt.Printf("✓ Added smart list %q to device %q\n", resolvedName, deviceName)

	if settings != nil {
		fmt.Println("\nWith settings:")
		printSettingsSummary(settings)
	} else {
		fmt.Println("\n(Using device default settings)")
	}

	return nil
}

func parseSyncOptions(cmd *cobra.Command) (*sync.SharedListSettings, error) {
	// Check if any flags were set
	flagsSet := cmd.Flags().Changed("only-unread") ||
		cmd.Flags().Changed("keep-last-read") ||
		cmd.Flags().Changed("keep-last-read-count") ||
		cmd.Flags().Changed("only-checked") ||
		cmd.Flags().Changed("limit") ||
		cmd.Flags().Changed("limit-type") ||
		cmd.Flags().Changed("sort")

	if !flagsSet {
		// No options specified - use device defaults
		return nil, nil
	}

	// Create settings with defaults
	settings := sync.DefaultSettings()

	// Apply flags
	if cmd.Flags().Changed("only-unread") {
		settings.OnlyUnread = onlyUnread
	}
	if cmd.Flags().Changed("keep-last-read") {
		settings.KeepLastRead = keepLastRead
	}
	if cmd.Flags().Changed("keep-last-read-count") {
		settings.KeepLastReadCount = keepLastReadCount
	}
	if cmd.Flags().Changed("only-checked") {
		settings.OnlyChecked = onlyChecked
	}

	if cmd.Flags().Changed("limit") {
		if limit > 0 {
			settings.Limit = true
			settings.LimitValue = limit
		} else {
			settings.Limit = false
		}
	}

	if cmd.Flags().Changed("limit-type") {
		switch limitType {
		case "books":
			settings.LimitValueType = sync.LimitTypeBooks
		case "mb":
			settings.LimitValueType = sync.LimitTypeMB
		case "gb":
			settings.LimitValueType = sync.LimitTypeGB
		default:
			return nil, fmt.Errorf("invalid limit-type: %s (use books, mb, or gb)", limitType)
		}
	}

	if cmd.Flags().Changed("sort") {
		switch sortType {
		case "series":
			settings.ListSortType = sync.SortTypeSeries
		case "random":
			settings.ListSortType = sync.SortTypeRandom
		case "published":
			settings.ListSortType = sync.SortTypePublished
		case "added":
			settings.ListSortType = sync.SortTypeAdded
		case "story-arc":
			settings.ListSortType = sync.SortTypeStoryArc
		case "list-order":
			settings.ListSortType = sync.SortTypeListOrder
		case "alternate-series":
			settings.ListSortType = sync.SortTypeAlternateSeries
		default:
			return nil, fmt.Errorf("invalid sort type: %s", sortType)
		}
		settings.Sort = true
	}

	return settings, nil
}

func init() {
	configCmd.AddCommand(addListCmd)

	addListCmd.Flags().StringVarP(&addListLibraryPath, "library", "l", "", "Path to ComicDb.xml file (required)")
	addListCmd.MarkFlagRequired("library")

	// Sync option flags
	addListCmd.Flags().BoolVar(&onlyUnread, "only-unread", false, "Only sync unread books")
	addListCmd.Flags().BoolVar(&keepLastRead, "keep-last-read", false, "Keep recently read books")
	addListCmd.Flags().IntVar(&keepLastReadCount, "keep-last-read-count", 0, "Number of recently read books to keep (default 3)")
	addListCmd.Flags().BoolVar(&onlyChecked, "only-checked", false, "Only sync checked books")
	addListCmd.Flags().IntVar(&limit, "limit", 0, "Limit number of books (0 = no limit)")
	addListCmd.Flags().StringVar(&limitType, "limit-type", "books", "Limit type: books, mb, or gb")
	addListCmd.Flags().StringVar(&sortType, "sort", "series", "Sort type: series, random, published, added, story-arc, list-order, alternate-series")
}
