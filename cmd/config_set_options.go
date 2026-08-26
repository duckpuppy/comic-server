package cmd

import (
	"fmt"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/sync"
	"github.com/spf13/cobra"
)

var setOptionsCmd = &cobra.Command{
	Use:   "set-options <device> <list-name>",
	Short: "Configure sync options for a smart list on a device",
	Long: `Configure sync options for a smart list on a device.
The device can be specified by friendly name or device ID.
The list must already be added to the device.

Use flags to specify which options to change. Options not specified
will keep their current values.`,
	Args: cobra.ExactArgs(2),
	RunE: runSetOptions,
}

var (
	setOptionsLibraryPath string

	// Sync option flags for set-options
	setOnlyUnread        *bool
	setKeepLastRead      *bool
	setKeepLastReadCount *int
	setOnlyChecked       *bool
	setLimit             *int
	setLimitType         string
	setSortType          string
)

func runSetOptions(cmd *cobra.Command, args []string) error {
	deviceNameOrID := args[0]
	listName := args[1]

	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Resolve device
	device, err := resolveConfigDBDevice(db, deviceNameOrID)
	if err != nil {
		return err
	}

	// Find list by name in device config
	var listID string
	for i := range device.Lists {
		if device.Lists[i].ListName == listName {
			listID = device.Lists[i].ListID
			break
		}
	}

	if listID == "" {
		// Try loading library to resolve by name
		if setOptionsLibraryPath != "" {
			lib, err := library.LoadLibrary(setOptionsLibraryPath)
			if err != nil {
				return fmt.Errorf("list %q not found in device configuration", listName)
			}

			resolvedID, _, err := config.ResolveSmartList(lib, listName)
			if err != nil {
				return fmt.Errorf("list %q not found in device configuration", listName)
			}
			listID = resolvedID
		} else {
			return fmt.Errorf("list %q not found in device configuration", listName)
		}
	}

	// Get current list config
	var current *sync.SharedListSettings
	found := false
	for i := range device.Lists {
		if device.Lists[i].ListID == listID {
			current = device.Lists[i].Settings
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("list %q not found in device configuration", listName)
	}

	// Get or create settings
	settings := current
	if settings == nil {
		settings = sync.DefaultSettings()
	}

	// Apply flag changes
	if cmd.Flags().Changed("only-unread") {
		settings.OnlyUnread = *setOnlyUnread
	}
	if cmd.Flags().Changed("keep-last-read") {
		settings.KeepLastRead = *setKeepLastRead
	}
	if cmd.Flags().Changed("keep-last-read-count") {
		settings.KeepLastReadCount = *setKeepLastReadCount
	}
	if cmd.Flags().Changed("only-checked") {
		settings.OnlyChecked = *setOnlyChecked
	}

	if cmd.Flags().Changed("limit") {
		if *setLimit > 0 {
			settings.Limit = true
			settings.LimitValue = *setLimit
		} else {
			settings.Limit = false
		}
	}

	if cmd.Flags().Changed("limit-type") {
		switch setLimitType {
		case "books":
			settings.LimitValueType = sync.LimitTypeBooks
		case "mb":
			settings.LimitValueType = sync.LimitTypeMB
		case "gb":
			settings.LimitValueType = sync.LimitTypeGB
		default:
			return fmt.Errorf("invalid limit-type: %s (use books, mb, or gb)", setLimitType)
		}
	}

	if cmd.Flags().Changed("sort") {
		switch setSortType {
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
			return fmt.Errorf("invalid sort type: %s", setSortType)
		}
		settings.Sort = true
	}

	// Update list settings
	if err := db.UpdateDeviceList(device.DeviceID, listID, nil, settings); err != nil {
		return fmt.Errorf("failed to update list settings: %w", err)
	}

	deviceName := device.FriendlyName
	if deviceName == "" {
		deviceName = device.DeviceID
	}

	fmt.Printf("✓ Updated sync options for %q on device %q\n", listName, deviceName)

	fmt.Println("\nCurrent settings:")
	printSettingsSummary(settings)

	return nil
}

func init() {
	configCmd.AddCommand(setOptionsCmd)

	setOptionsCmd.Flags().StringVarP(&setOptionsLibraryPath, "library", "l", "", "Path to ComicDb.xml file (optional, for resolving list names)")

	// Use pointers for bool flags to detect if they were set
	setOnlyUnread = setOptionsCmd.Flags().Bool("only-unread", false, "Only sync unread books")
	setKeepLastRead = setOptionsCmd.Flags().Bool("keep-last-read", false, "Keep recently read books")
	setKeepLastReadCount = setOptionsCmd.Flags().Int("keep-last-read-count", 0, "Number of recently read books to keep (default 3)")
	setOnlyChecked = setOptionsCmd.Flags().Bool("only-checked", false, "Only sync checked books")
	setLimit = setOptionsCmd.Flags().Int("limit", 0, "Limit number of books (0 = no limit)")
	setOptionsCmd.Flags().StringVar(&setLimitType, "limit-type", "", "Limit type: books, mb, or gb")
	setOptionsCmd.Flags().StringVar(&setSortType, "sort", "", "Sort type: series, random, published, added, story-arc, list-order, alternate-series")
}
