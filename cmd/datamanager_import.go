package cmd

import (
	"fmt"
	"os"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	dmImportPath  string
	dmImportForce bool
)

var datamanagerImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a dataman.dat file into config.db",
	Long: `Parses a ComicRack Data Manager plugin's dataman.dat file and imports
its groups, rulesets, rules, and actions into config.db, preserving the
file's own nested folder structure and depth-first evaluation order.

This only ever reads the source file, never modifies it. Running import
again on a config.db that already holds a previous import is refused
unless --force is passed - since rules are still actively authored in
ComicRack's Data Manager plugin until it's fully retired, re-import is a
real recurring workflow, not a one-time migration. --force replaces every
PREVIOUSLY IMPORTED group/ruleset/rule/action with the freshly parsed
file, atomically (nothing is deleted if the parse or insert fails). Any
rule created directly in the web UI's rule editor is never touched by
this, forced or not - import and the editor merge, they don't clobber
each other.`,
	RunE: runDatamanagerImport,
}

func init() {
	datamanagerImportCmd.Flags().StringVar(&dmImportPath, "dat", "", "path to dataman.dat (required)")
	datamanagerImportCmd.Flags().BoolVar(&dmImportForce, "force", false, "wipe existing Data Manager rules and replace them with this import")
	_ = datamanagerImportCmd.MarkFlagRequired("dat")
	datamanagerCmd.AddCommand(datamanagerImportCmd)
}

func runDatamanagerImport(cmd *cobra.Command, args []string) error {
	f, err := os.Open(dmImportPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", dmImportPath, err)
	}
	defer f.Close()

	result, err := datamanager.ParseDataman(f, func() string { return uuid.New().String() })
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", dmImportPath, err)
	}

	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if !dmImportForce {
		hasImported, err := db.HasImportedDataManagerRules()
		if err != nil {
			return fmt.Errorf("failed to check for existing imported Data Manager rules: %w", err)
		}
		if hasImported {
			return fmt.Errorf("config.db already has a previous dataman.dat import - pass --force to replace it with this import (rules created in the web UI's rule editor are never affected)")
		}
	}

	groups := make([]configdb.DMImportGroup, len(result.Groups))
	for i, g := range result.Groups {
		groups[i] = configdb.DMImportGroup{
			ID:        g.ID,
			ParentID:  g.ParentID,
			Name:      g.Name,
			Comment:   g.Comment,
			Disabled:  g.Disabled,
			SortOrder: g.SortOrder,
		}
	}

	rulesets := make([]configdb.DMImportRuleset, len(result.Rulesets))
	for i, rs := range result.Rulesets {
		rules := make([]configdb.DMImportRule, len(rs.Rules))
		for j, r := range rs.Rules {
			rules[j] = configdb.DMImportRule{Field: r.Field, Modifier: r.Modifier, Value: r.Value}
		}
		actions := make([]configdb.DMImportAction, len(rs.Actions))
		for j, a := range rs.Actions {
			actions[j] = configdb.DMImportAction{Field: a.Field, Modifier: a.Modifier, Value: a.Value}
		}
		rulesets[i] = configdb.DMImportRuleset{
			ID:        uuid.New().String(),
			GroupID:   rs.GroupID,
			Name:      rs.Name,
			Comment:   rs.Comment,
			Mode:      rs.Mode,
			Disabled:  rs.Disabled,
			SortOrder: rs.SortOrder,
			Rules:     rules,
			Actions:   actions,
		}
	}

	if err := db.ImportDataManagerRules(groups, rulesets, dmImportForce); err != nil {
		return fmt.Errorf("failed to import Data Manager rules: %w", err)
	}

	verb := "Imported"
	if dmImportForce {
		verb = "Replaced previously-imported Data Manager rules with"
	}
	fmt.Printf("%s %d groups and %d rulesets from %s\n", verb, len(groups), len(rulesets), dmImportPath)
	return nil
}
