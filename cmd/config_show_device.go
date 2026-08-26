package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/duckpuppy/comic-server/internal/sync"
)

var showDeviceCmd = &cobra.Command{
	Use:   "show-device <device-name-or-id>",
	Short: "Show sync configuration for a device",
	Long: `Display the complete sync configuration for a specific device,
including configured smart lists and their settings.`,
	Args: cobra.ExactArgs(1),
	RunE: runShowDevice,
}

var (
	verboseFlag bool
)

func runShowDevice(cmd *cobra.Command, args []string) error {
	deviceNameOrID := args[0]

	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	device, err := resolveConfigDBDevice(db, deviceNameOrID)
	if err != nil {
		return err
	}

	// Display device info
	name := device.FriendlyName
	if name == "" {
		name = "(unnamed)"
	}

	fmt.Printf("Device: %s\n", name)
	if verboseFlag || device.FriendlyName == "" {
		fmt.Printf("ID: %s\n", device.DeviceID)
	}
	if !device.LastSeen.IsZero() {
		fmt.Printf("Last seen: %s\n", device.LastSeen.Format("2006-01-02 15:04:05"))
	}
	fmt.Println()

	// Display default settings
	if device.DefaultSettings != nil {
		fmt.Println("Default Settings:")
		printSettingsSummary(device.DefaultSettings)
		fmt.Println()
	}

	// Display configured lists
	if len(device.Lists) == 0 {
		fmt.Println("No smart lists configured")
		return nil
	}

	fmt.Printf("Configured Smart Lists (%d):\n\n", len(device.Lists))

	for _, list := range device.Lists {
		fmt.Print(formatConfigDBList(list))
		fmt.Println()
	}

	return nil
}

func printSettingsSummary(settings *sync.SharedListSettings) {
	if settings.OnlyUnread {
		fmt.Println("  - Only unread books")
	}
	if settings.KeepLastRead {
		fmt.Printf("  - Keep last read books (%d)\n", sync.EffectiveKeepLastReadCount(settings))
	}
	if settings.OnlyChecked {
		fmt.Println("  - Only checked books")
	}
	if settings.Limit {
		fmt.Printf("  - Limit: %d %s\n", settings.LimitValue, settings.LimitValueType)
	}
	if settings.Sort {
		fmt.Printf("  - Sort: %s\n", settings.ListSortType)
	}
}

func init() {
	configCmd.AddCommand(showDeviceCmd)

	showDeviceCmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show additional details including device ID")
}
