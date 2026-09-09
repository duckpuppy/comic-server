package cmd

import (
	"fmt"
	"os"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/libraryorganizer"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	loImportPath  string
	loImportForce bool
)

var libraryOrganizerImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a losettingsx.dat file into config.db",
	Long: `Parses a ComicRack Library Organizer plugin's losettingsx.dat file and
imports every profile (BaseFolder, templates, mode, exclude rules, and
all seven Name/Value collections) into config.db.

This only ever reads the source file, never modifies it. Running import
again on a config.db that already has Library Organizer profiles is
refused unless --force is passed - matching comic-server datamanager
import's own re-import semantics (comic-server-cge): --force WIPES every
existing profile and replaces them with the freshly parsed file,
atomically, rather than merging.`,
	RunE: runLibraryOrganizerImport,
}

func init() {
	libraryOrganizerImportCmd.Flags().StringVar(&loImportPath, "dat", "", "path to losettingsx.dat (required)")
	libraryOrganizerImportCmd.Flags().BoolVar(&loImportForce, "force", false, "wipe existing Library Organizer profiles and replace them with this import")
	_ = libraryOrganizerImportCmd.MarkFlagRequired("dat")
	libraryOrganizerCmd.AddCommand(libraryOrganizerImportCmd)
}

func runLibraryOrganizerImport(cmd *cobra.Command, args []string) error {
	f, err := os.Open(loImportPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", loImportPath, err)
	}
	defer f.Close()

	result, err := libraryorganizer.ParseLOSettings(f, func() string { return uuid.New().String() })
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", loImportPath, err)
	}

	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if !loImportForce {
		existing, err := db.ListLOProfiles()
		if err != nil {
			return fmt.Errorf("failed to check for existing Library Organizer profiles: %w", err)
		}
		if len(existing) > 0 {
			return fmt.Errorf("config.db already has Library Organizer profiles - pass --force to wipe them and import fresh (this replaces, it does not merge)")
		}
	}

	profiles := make([]configdb.LOImportProfile, len(result.Profiles))
	for i, p := range result.Profiles {
		items := make([]configdb.LOImportItem, len(p.Items))
		for j, item := range p.Items {
			items[j] = configdb.LOImportItem{Category: item.Category, Name: item.Name, Value: item.Value}
		}
		rules := make([]configdb.LOImportExcludeRule, len(p.ExcludeRules))
		for j, r := range p.ExcludeRules {
			rules[j] = configdb.LOImportExcludeRule{Field: r.Field, Operator: r.Operator, Value: r.Value}
		}
		profiles[i] = configdb.LOImportProfile{
			ID:                    p.ID,
			Name:                  p.Name,
			BaseFolder:            p.BaseFolder,
			FolderTemplate:        p.FolderTemplate,
			FileTemplate:          p.FileTemplate,
			EmptyFolder:           p.EmptyFolder,
			Mode:                  p.Mode,
			CopyMode:              p.CopyMode,
			UseFolder:             p.UseFolder,
			UseFileName:           p.UseFileName,
			ReplaceMultipleSpaces: p.ReplaceMultipleSpaces,
			AutoSpaceFields:       p.AutoSpaceFields,
			RemoveEmptyFolder:     p.RemoveEmptyFolder,
			MoveFileless:          p.MoveFileless,
			FilelessFormat:        p.FilelessFormat,
			FailEmptyValues:       p.FailEmptyValues,
			MoveFailed:            p.MoveFailed,
			FailedFolder:          p.FailedFolder,
			ExcludeMode:           p.ExcludeMode,
			ExcludeOperator:       p.ExcludeOperator,
			SortOrder:             p.SortOrder,
			Items:                 items,
			ExcludeRules:          rules,
		}
	}

	if err := db.ImportLibraryOrganizerProfiles(profiles, loImportForce); err != nil {
		return fmt.Errorf("failed to import Library Organizer profiles: %w", err)
	}

	verb := "Imported"
	if loImportForce {
		verb = "Replaced existing Library Organizer profiles with"
	}
	fmt.Printf("%s %d profiles from %s\n", verb, len(profiles), loImportPath)
	return nil
}
