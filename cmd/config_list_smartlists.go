package cmd

import (
	"fmt"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/spf13/cobra"
)

var listSmartlistsCmd = &cobra.Command{
	Use:   "list-smartlists",
	Short: "List all smart lists in the library",
	Long: `List all smart lists available in the ComicRack library.
These lists can be added to device sync configurations.`,
	RunE: runListSmartlists,
}

var (
	libraryPathFlag string
)

func runListSmartlists(cmd *cobra.Command, args []string) error {
	// Load library
	lib, err := library.LoadLibrary(libraryPathFlag)
	if err != nil {
		return fmt.Errorf("failed to load library: %w", err)
	}

	// Filter smart lists
	var smartLists []library.ComicListItem
	for _, list := range lib.ComicLists {
		if strings.Contains(list.Type, "SmartList") {
			smartLists = append(smartLists, list)
		}
	}

	if len(smartLists) == 0 {
		fmt.Println("No smart lists found in library")
		return nil
	}

	// Display smart lists
	fmt.Printf("Smart Lists (%d):\n\n", len(smartLists))

	for _, list := range smartLists {
		// Count matching books
		books, err := lib.MatchBooks(&list)
		if err != nil {
			fmt.Printf("• %s\n", list.Name)
			fmt.Printf("  ID: %s\n", list.ID)
			fmt.Printf("  Error: %v\n\n", err)
			continue
		}

		fmt.Printf("• %s\n", list.Name)
		fmt.Printf("  ID: %s\n", list.ID)
		fmt.Printf("  Books: %d\n", len(books))
		fmt.Printf("  Matcher mode: %s\n", list.MatcherMode)
		fmt.Printf("  Matchers: %d\n\n", len(list.Matchers))
	}

	return nil
}

func init() {
	configCmd.AddCommand(listSmartlistsCmd)

	listSmartlistsCmd.Flags().StringVarP(&libraryPathFlag, "library", "l", "", "Path to ComicDb.xml file (required)")
	listSmartlistsCmd.MarkFlagRequired("library")
}
