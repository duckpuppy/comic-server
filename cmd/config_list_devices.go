package cmd

import (
	"fmt"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/spf13/cobra"
)

var listDevicesCmd = &cobra.Command{
	Use:   "list-devices",
	Short: "List all configured devices",
	Long: `List all devices that have sync configuration.
Shows device ID, friendly name, and basic sync information.`,
	RunE: runListDevices,
}

func runListDevices(cmd *cobra.Command, args []string) error {
	// Load config
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Display devices
	fmt.Println(config.FormatDeviceList(cfg.Devices))

	return nil
}

func init() {
	configCmd.AddCommand(listDevicesCmd)
}
