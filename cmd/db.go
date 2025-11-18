package cmd

import (
	"fmt"

	"github.com/duckpuppy/comic-server/internal/storage"
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management commands",
	Long:  `Commands for managing the SQLite comic library database.`,
}

var dbInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show database information and statistics",
	Long: `Display information about the SQLite database including:
- Book count
- List count
- Custom values count
- Tags count
- Library metadata
- Last import time`,
	RunE: runDBInfo,
}

var dbInfoPath string

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbInfoCmd)

	dbInfoCmd.Flags().StringVar(&dbInfoPath, "db", "", "Path to SQLite database file (required)")
	dbInfoCmd.MarkFlagRequired("db")
}

func runDBInfo(cmd *cobra.Command, args []string) error {
	// Open database
	db, err := storage.Open(dbInfoPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	fmt.Printf("Database: %s\n\n", db.Path())

	// Book count
	var bookCount int
	err = db.QueryRow("SELECT COUNT(*) FROM books").Scan(&bookCount)
	if err != nil {
		return fmt.Errorf("count books: %w", err)
	}
	fmt.Printf("Books: %d\n", bookCount)

	// List count
	var listCount int
	err = db.QueryRow("SELECT COUNT(*) FROM lists").Scan(&listCount)
	if err != nil {
		return fmt.Errorf("count lists: %w", err)
	}
	fmt.Printf("Lists: %d\n", listCount)

	// Custom values count
	var customValuesCount int
	err = db.QueryRow("SELECT COUNT(*) FROM book_custom_values").Scan(&customValuesCount)
	if err != nil {
		return fmt.Errorf("count custom values: %w", err)
	}
	fmt.Printf("Custom Values: %d\n", customValuesCount)

	// Tags count
	var tagsCount int
	err = db.QueryRow("SELECT COUNT(*) FROM book_tags").Scan(&tagsCount)
	if err != nil {
		return fmt.Errorf("count tags: %w", err)
	}
	fmt.Printf("Tags: %d\n", tagsCount)

	// Reading list items count
	var readingListItemsCount int
	err = db.QueryRow("SELECT COUNT(*) FROM reading_list_items").Scan(&readingListItemsCount)
	if err != nil {
		return fmt.Errorf("count reading list items: %w", err)
	}
	fmt.Printf("Reading List Items: %d\n", readingListItemsCount)

	// Library metadata
	fmt.Println("\nLibrary Metadata:")
	rows, err := db.Query("SELECT key, value FROM library_metadata ORDER BY key")
	if err != nil {
		return fmt.Errorf("query metadata: %w", err)
	}
	defer rows.Close()

	hasMetadata := false
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return fmt.Errorf("scan metadata: %w", err)
		}
		fmt.Printf("  %s: %s\n", key, value)
		hasMetadata = true
	}
	if !hasMetadata {
		fmt.Println("  (none)")
	}

	// Database file size
	fmt.Println()

	return nil
}
