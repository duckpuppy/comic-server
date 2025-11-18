package cmd

import (
	"fmt"
	"os"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a ComicRack library into SQLite database",
	Long: `Import a ComicRack ComicDb.xml file into a SQLite database.

The import is idempotent - importing the same file twice will result in no changes.
Only modified records will be updated, and deleted records will be removed.

Examples:
  comic-server import --xml ~/.local/share/ComicRack/ComicDb.xml --db library.db
  comic-server import --xml /path/to/ComicDb.xml --db /path/to/library.db --verbose
  comic-server import --xml /path/to/ComicDb.xml --db /path/to/library.db --dry-run`,
	RunE: runImport,
}

var (
	importXMLPath string
	importDBPath  string
	importDryRun  bool
	importVerbose bool
)

func init() {
	rootCmd.AddCommand(importCmd)

	importCmd.Flags().StringVar(&importXMLPath, "xml", "", "Path to ComicDb.xml file (required)")
	importCmd.Flags().StringVar(&importDBPath, "db", "", "Path to SQLite database file (required)")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Show what would change without modifying database")
	importCmd.Flags().BoolVar(&importVerbose, "verbose", false, "Show detailed import progress")

	importCmd.MarkFlagRequired("xml")
	importCmd.MarkFlagRequired("db")
}

func runImport(cmd *cobra.Command, args []string) error {
	// Check that XML file exists
	if _, err := os.Stat(importXMLPath); os.IsNotExist(err) {
		return fmt.Errorf("XML file not found: %s", importXMLPath)
	}

	// Load library from XML
	if importVerbose {
		fmt.Printf("Loading library from %s...\n", importXMLPath)
	}

	lib, err := library.LoadLibrary(importXMLPath)
	if err != nil {
		return fmt.Errorf("load library: %w", err)
	}

	if importVerbose {
		fmt.Printf("Loaded %d books and %d lists\n", len(lib.Books), len(lib.ComicLists))
	}

	// Open or create database
	if importVerbose {
		fmt.Printf("Opening database %s...\n", importDBPath)
	}

	db, err := storage.Open(importDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Perform import
	if importDryRun {
		fmt.Println("Dry run mode - no changes will be made")
	}

	if importVerbose {
		fmt.Println("Importing...")
	}

	stats, err := db.Import(lib, storage.ImportOptions{
		DryRun:  importDryRun,
		Verbose: importVerbose,
	})
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	// Print results
	fmt.Printf("\nImport complete in %s\n\n", stats.Duration.Round(1e6))

	fmt.Printf("Books: %d total\n", len(lib.Books))
	fmt.Printf("  Added:     %d\n", stats.BooksAdded)
	fmt.Printf("  Updated:   %d\n", stats.BooksUpdated)
	fmt.Printf("  Deleted:   %d\n", stats.BooksDeleted)
	fmt.Printf("  Unchanged: %d\n", stats.BooksUnchanged)

	fmt.Printf("\nLists: %d total\n", countLists(lib.ComicLists))
	fmt.Printf("  Added:     %d\n", stats.ListsAdded)
	fmt.Printf("  Updated:   %d\n", stats.ListsUpdated)
	fmt.Printf("  Deleted:   %d\n", stats.ListsDeleted)
	fmt.Printf("  Unchanged: %d\n", stats.ListsUnchanged)

	if importDryRun {
		fmt.Println("\nDry run complete - no changes were made")
	}

	return nil
}

// countLists recursively counts all lists including nested ones
func countLists(lists []library.ComicListItem) int {
	count := len(lists)
	for _, list := range lists {
		count += countLists(list.ChildItems)
	}
	return count
}
