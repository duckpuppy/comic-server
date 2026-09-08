package cmd

import (
	"fmt"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
	"github.com/duckpuppy/comic-server/internal/workflow"
	"github.com/spf13/cobra"
)

var workflowBackfillLibraryPath string

var workflowBackfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Assign a starting pipeline stage to every book that doesn't have one yet",
	Long: `One-time backfill (comic-server-1iv.1): every book that has never had an
explicit workflow stage set gets one inferred from its current data shape
(file extension, ComicVine tag, ScanInformation, and whether the
currently-configured Data Manager rules would still change it). Safe to
run more than once - a book that already has an explicit stage is left
untouched.`,
	RunE: runWorkflowBackfill,
}

func init() {
	workflowBackfillCmd.Flags().StringVar(&workflowBackfillLibraryPath, "library", "", "path to ComicDb.xml (default: server.library_path from config)")
	workflowCmd.AddCommand(workflowBackfillCmd)
}

func runWorkflowBackfill(cmd *cobra.Command, args []string) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	libPath := workflowBackfillLibraryPath
	if libPath == "" {
		libPath = cfg.Server.LibraryPath
	}
	if libPath == "" {
		return fmt.Errorf("no library path: pass --library or set server.library_path in config")
	}

	var backend library.Backend
	if cfg.Server.DatabasePath != "" {
		sqliteBackend, err := storage.NewSQLiteBackend(cfg.Server.DatabasePath, libPath)
		if err != nil {
			return fmt.Errorf("failed to open SQLite database: %w", err)
		}
		backend = sqliteBackend
	} else {
		xmlBackend, err := library.NewXMLBackend(libPath, 0)
		if err != nil {
			return fmt.Errorf("failed to load library: %w", err)
		}
		backend = xmlBackend
	}
	defer backend.Close()

	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	rulesets, err := datamanager.LoadRulesets(db)
	if err != nil {
		return fmt.Errorf("failed to load Data Manager rules: %w", err)
	}

	books, err := backend.GetAllBooks()
	if err != nil {
		return fmt.Errorf("failed to load books: %w", err)
	}

	counts := map[workflow.Stage]int{}
	var toUpdate []*library.ComicBook
	for i := range books {
		book := &books[i]
		if workflow.GetStage(book) != workflow.StageUnknown {
			continue
		}
		stage := workflow.InferStage(book, rulesets)
		workflow.SetStage(book, stage)
		counts[stage]++
		toUpdate = append(toUpdate, book)
	}

	if len(toUpdate) == 0 {
		fmt.Println("Every book already has an explicit workflow stage - nothing to backfill.")
		return nil
	}

	if err := backend.UpdateBooks(toUpdate); err != nil {
		return fmt.Errorf("failed to save workflow stages: %w", err)
	}

	fmt.Printf("Backfilled %d books:\n", len(toUpdate))
	for _, s := range workflow.Stages {
		if n := counts[s]; n > 0 {
			fmt.Printf("  %-16s %d\n", s.Label(), n)
		}
	}
	return nil
}
