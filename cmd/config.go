package cmd

import (
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage device sync configuration",
	Long: `Manage device sync configuration including smart list selection,
sync options, and device settings.

Configuration is stored in $XDG_CONFIG_HOME/comic-server/config.yaml
(or ~/.config/comic-server/config.yaml if XDG_CONFIG_HOME is not set).

Use --config to specify a different configuration file.`,
}

func init() {
	rootCmd.AddCommand(configCmd)
}
