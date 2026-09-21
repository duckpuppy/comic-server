package cmd

import (
	"fmt"
	"os"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
	"github.com/spf13/cobra"
)

var importCBLCmd = &cobra.Command{
	Use:   "import-cbl",
	Short: "Import a CBL reading-list file, matching entries against the library",
	Long: `Import a ComicRack CBL reading-list file (comic-server-r1j4). Every
entry is matched against the SQLite library's books - primarily by
ComicVine issue ID (the <Database Name="cv"> extension used by
DieselTech/CBL-ReadingLists and Kavita/Komga), falling back to
ComicRackCE's own Series/Number/Volume/Year matching algorithm for
entries without one.

Matched entries become a new reading list, in the CBL's original order.
Unmatched entries are NOT added to the list - they're dropped and
reported in the summary below (see docs/plans/2026-09-20-cbl-reading-list-import.md
§2c; adding them to the wanted-books list is a separate, not-yet-built
step, comic-server-sx2d).

CBLs that embed smart-list matcher rules (rather than a plain book list)
are not yet supported and will be rejected.

Examples:
  comic-server import-cbl --db library.db --cbl "Batman.cbl"
  comic-server import-cbl --db library.db --cbl "Batman.cbl" --dry-run`,
	RunE: runImportCBL,
}

var (
	importCBLDBPath  string
	importCBLPath    string
	importCBLDryRun  bool
	importCBLVerbose bool
)

func init() {
	rootCmd.AddCommand(importCBLCmd)

	importCBLCmd.Flags().StringVar(&importCBLDBPath, "db", "", "Path to SQLite database file (required)")
	importCBLCmd.Flags().StringVar(&importCBLPath, "cbl", "", "Path to the CBL file to import (required)")
	importCBLCmd.Flags().BoolVar(&importCBLDryRun, "dry-run", false, "Parse and match without writing a new list")
	importCBLCmd.Flags().BoolVar(&importCBLVerbose, "verbose", false, "List every unmatched entry")

	importCBLCmd.MarkFlagRequired("db")
	importCBLCmd.MarkFlagRequired("cbl")
}

func runImportCBL(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(importCBLDBPath); os.IsNotExist(err) {
		return fmt.Errorf("database file not found: %s", importCBLDBPath)
	}

	f, err := os.Open(importCBLPath)
	if err != nil {
		return fmt.Errorf("open CBL file: %w", err)
	}
	defer f.Close()

	rl, err := cbl.Parse(f)
	if err != nil {
		return fmt.Errorf("parse CBL file: %w", err)
	}

	db, err := storage.Open(importCBLDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if importCBLDryRun {
		if rl.IsSmartList() {
			return storage.ErrCBLIsSmartList
		}
		candidates, err := db.GetAllBooks()
		if err != nil {
			return fmt.Errorf("load library for matching: %w", err)
		}
		candidatePtrs := make([]*library.ComicBook, len(candidates))
		for i := range candidates {
			candidatePtrs[i] = &candidates[i]
		}

		var cvHits, stringHits, misses int
		for _, entry := range rl.Books {
			switch cbl.MatchEntry(entry, candidatePtrs).Path {
			case cbl.MatchCVID:
				cvHits++
			case cbl.MatchSeriesNumber:
				stringHits++
			default:
				misses++
			}
		}
		fmt.Printf("Dry run: %q (%d entries)\n", rl.Name, len(rl.Books))
		fmt.Printf("  Would match (ComicVine ID):  %d\n", cvHits)
		fmt.Printf("  Would match (series/number): %d\n", stringHits)
		fmt.Printf("  Would be unmatched:          %d\n", misses)
		fmt.Println("\nNo file was written - no list was created.")
		return nil
	}

	result, err := db.ImportCBL(rl, storage.CBLImportSource{Source: "local_file"})
	if err != nil {
		return fmt.Errorf("import CBL: %w", err)
	}

	fmt.Printf("Imported %q as list %s\n", rl.Name, result.ListID)
	fmt.Printf("  Matched (ComicVine ID):     %d\n", result.MatchedCVID)
	fmt.Printf("  Matched (series/number):    %d\n", result.MatchedOther)
	fmt.Printf("  Unmatched:                  %d\n", result.Unmatched)

	if importCBLVerbose && result.Unmatched > 0 {
		fmt.Println("\nUnmatched entries:")
		for _, e := range result.Entries {
			if e.BookID == "" {
				fmt.Printf("  - %s #%s (%d)\n", e.Series, e.Number, e.Year)
			}
		}
	}

	return nil
}
