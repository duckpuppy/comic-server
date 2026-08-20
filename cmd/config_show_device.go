package cmd

import (
	"fmt"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/sync"
	"github.com/spf13/cobra"
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

	// Load config
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Resolve device
	deviceID, device, err := config.ResolveDevice(cfg, deviceNameOrID)
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
		fmt.Printf("ID: %s\n", deviceID)
	}
	if !device.LastSeen.IsZero() {
		fmt.Printf("Last seen: %s\n", device.LastSeen.Format("2006-01-02 15:04:05"))
	}
	fmt.Println()

	// Display default settings
	if device.DefaultSettings != nil {
		fmt.Println("Default Settings:")
		if device.DefaultSettings.OnlyUnread {
			fmt.Println("  - Only unread books")
		}
		if device.DefaultSettings.KeepLastRead {
			fmt.Printf("  - Keep last read books (%d)\n", sync.EffectiveKeepLastReadCount(device.DefaultSettings))
		}
		if device.DefaultSettings.OnlyChecked {
			fmt.Println("  - Only checked books")
		}
		if device.DefaultSettings.Limit {
			fmt.Printf("  - Limit: %d %s\n", device.DefaultSettings.LimitValue, device.DefaultSettings.LimitValueType)
		}
		if device.DefaultSettings.Sort {
			fmt.Printf("  - Sort: %s\n", device.DefaultSettings.ListSortType)
		}
		fmt.Println()
	}

	// Display configured lists
	if len(device.Lists) == 0 {
		fmt.Println("No smart lists configured")
		return nil
	}

	fmt.Printf("Configured Smart Lists (%d):\n\n", len(device.Lists))

	for _, list := range device.Lists {
		fmt.Print(config.FormatSmartListConfig(&list))
		fmt.Println()
	}

	return nil
}

func init() {
	configCmd.AddCommand(showDeviceCmd)

	showDeviceCmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show additional details including device ID")
}
