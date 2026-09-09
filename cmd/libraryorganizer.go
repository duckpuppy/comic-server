package cmd

import (
	"github.com/spf13/cobra"
)

var libraryOrganizerCmd = &cobra.Command{
	Use:   "library-organizer",
	Short: "Manage Library Organizer profiles (file move/rename engine)",
	Long: `Manage profiles migrated from the ComicRack Library Organizer plugin.
Profiles are stored in config.db alongside device and sync configuration.

comic-server's first feature that moves/renames the user's own existing
comic files on disk.`,
}

func init() {
	rootCmd.AddCommand(libraryOrganizerCmd)
}
