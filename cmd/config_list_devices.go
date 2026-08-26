package cmd

import (
	"fmt"

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
	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	devices, err := db.ListDevices()
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	fmt.Println(formatConfigDBDeviceList(devices))

	return nil
}

func init() {
	configCmd.AddCommand(listDevicesCmd)
}
