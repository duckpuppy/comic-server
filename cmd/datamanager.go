package cmd

import (
	"github.com/spf13/cobra"
)

var datamanagerCmd = &cobra.Command{
	Use:   "datamanager",
	Short: "Manage Data Manager rules (rule engine + taxonomy migration)",
	Long: `Manage rules migrated from the ComicRack Data Manager plugin
(https://github.com/maforget/CRDataManager). Rules are stored in config.db
alongside device and sync configuration.`,
}

func init() {
	rootCmd.AddCommand(datamanagerCmd)
}
