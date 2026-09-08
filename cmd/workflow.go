package cmd

import (
	"github.com/spf13/cobra"
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage the native ingest-pipeline workflow (comic-server-1iv)",
	Long: `Manage comic-server's replacement for the manual ComicRack ingest
pipeline (Convert to CBZ -> Scrape -> Scan Info -> Data Manager -> To Move),
tracked as a real per-book field instead of a set of hand-maintained smart
lists.`,
}

func init() {
	rootCmd.AddCommand(workflowCmd)
}
