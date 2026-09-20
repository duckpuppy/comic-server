package cmd

import (
	"fmt"
	"os"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the SQLite library database back to a ComicDb.xml file",
	Long: `Export comic-server's SQLite library database (books and smart/reading
lists, including matchers) to a ComicDb.xml file that real ComicRackCE can
open. This is the reverse of the "import" command.

Three comic-server-only smart list matcher types
(ComicServerCVSeriesCompleteMatcher, ComicServerCVMissingCountMatcher,
ComicServerCVPercentOwnedMatcher) aren't understood by ComicRackCE. Any
smart list containing one of these has just that matcher dropped on
export - the list's other matchers and structure are preserved - and a
warning is printed for each one dropped.

Examples:
  comic-server export --db library.db --xml ~/.local/share/ComicRack/ComicDb.xml
  comic-server export --db /path/to/library.db --xml /path/to/ComicDb.xml --verbose
  comic-server export --db /path/to/library.db --xml /path/to/ComicDb.xml --dry-run`,
	RunE: runExport,
}

var (
	exportDBPath  string
	exportXMLPath string
	exportDryRun  bool
	exportVerbose bool
)

func init() {
	rootCmd.AddCommand(exportCmd)

	exportCmd.Flags().StringVar(&exportDBPath, "db", "", "Path to SQLite database file (required)")
	exportCmd.Flags().StringVar(&exportXMLPath, "xml", "", "Path to write the exported ComicDb.xml file (required)")
	exportCmd.Flags().BoolVar(&exportDryRun, "dry-run", false, "Build and report the export without writing the XML file")
	exportCmd.Flags().BoolVar(&exportVerbose, "verbose", false, "Show detailed export progress")

	exportCmd.MarkFlagRequired("db")
	exportCmd.MarkFlagRequired("xml")
}

func runExport(cmd *cobra.Command, args []string) error {
	// Check that the database file exists
	if _, err := os.Stat(exportDBPath); os.IsNotExist(err) {
		return fmt.Errorf("database file not found: %s", exportDBPath)
	}

	if exportVerbose {
		fmt.Printf("Opening database %s...\n", exportDBPath)
	}

	db, err := storage.Open(exportDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if exportVerbose {
		fmt.Println("Reading books and lists from database...")
	}

	result, err := db.Export()
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	for _, warning := range result.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}

	fmt.Printf("Books: %d\n", len(result.Library.Books))
	fmt.Printf("Lists: %d (top-level)\n", len(result.Library.ComicLists))

	if exportDryRun {
		fmt.Println("\nDry run mode - no file was written")
		return nil
	}

	if exportVerbose {
		fmt.Printf("Writing library to %s...\n", exportXMLPath)
	}

	if err := library.SaveLibrary(exportXMLPath, result.Library); err != nil {
		return fmt.Errorf("save library: %w", err)
	}

	fmt.Printf("\nExport complete: %s\n", exportXMLPath)

	return nil
}
