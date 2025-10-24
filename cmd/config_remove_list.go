package cmd

import (
	"fmt"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/spf13/cobra"
)

var removeListCmd = &cobra.Command{
	Use:   "remove-list <device> <list-name>",
	Short: "Remove a smart list from a device's sync configuration",
	Long: `Remove a smart list from a device's sync configuration.
The device can be specified by friendly name or device ID.
The list name must match a configured smart list.`,
	Args: cobra.ExactArgs(2),
	RunE: runRemoveList,
}

var (
	removeListLibraryPath string
)

func runRemoveList(cmd *cobra.Command, args []string) error {
	deviceNameOrID := args[0]
	listName := args[1]

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
		if removeListLibraryPath != "" {
			lib, err := library.LoadLibrary(removeListLibraryPath)
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

	// Remove list
	if err := device.RemoveList(listID); err != nil {
		return err
	}

	// Save config
	if err := config.Save(cfg, configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	deviceName := device.FriendlyName
	if deviceName == "" {
		deviceName = deviceID
	}

	fmt.Printf("✓ Removed smart list %q from device %q\n", listName, deviceName)

	return nil
}

func init() {
	configCmd.AddCommand(removeListCmd)

	removeListCmd.Flags().StringVarP(&removeListLibraryPath, "library", "l", "", "Path to ComicDb.xml file (optional, for resolving list names)")
}
