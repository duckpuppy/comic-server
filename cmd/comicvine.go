package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/spf13/cobra"
)

var cvCmd = &cobra.Command{
	Use:   "comicvine",
	Short: "ComicVine enrichment commands",
	Long:  `Commands for managing the ComicVine enrichment cache.`,
}

var cvStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show ComicVine sync status",
	Long:  `Display the current state of the ComicVine enrichment cache including synced, pending, and failed volume counts.`,
	RunE:  runCVStatus,
}

func init() {
	rootCmd.AddCommand(cvCmd)
	cvCmd.AddCommand(cvStatusCmd)
}

func runCVStatus(cmd *cobra.Command, args []string) error {
	dataDir, err := config.GetDataDir()
	if err != nil {
		return fmt.Errorf("get data directory: %w", err)
	}

	cachePath := filepath.Join(dataDir, "comicvine_cache.db")
	cache, err := comicvine.OpenCache(cachePath)
	if err != nil {
		return fmt.Errorf("open cache: %w", err)
	}
	defer cache.Close()

	synced, pending, failed, err := cache.SyncStats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	total := synced + pending + failed
	fmt.Printf("ComicVine Enrichment Cache\n")
	fmt.Printf("  Path:    %s\n", cachePath)
	fmt.Printf("  Volumes: %d total\n", total)
	fmt.Printf("  Synced:  %d", synced)
	if total > 0 {
		fmt.Printf(" (%.1f%%)", float64(synced)/float64(total)*100)
	}
	fmt.Println()
	fmt.Printf("  Pending: %d\n", pending)
	fmt.Printf("  Failed:  %d\n", failed)

	return nil
}
