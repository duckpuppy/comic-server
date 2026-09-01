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
again on a config.db that already has Data Manager rules is refused
unless --force is passed - since rules are still actively authored in
ComicRack's Data Manager plugin until it's fully retired, re-import is a
real recurring workflow, not a one-time migration. --force WIPES every
existing Data Manager group/ruleset/rule/action and replaces them with
the freshly parsed file, atomically (nothing is deleted if the parse or
insert fails) - it does not merge, and any rule you created directly in
config.db (not from a dataman.dat import) would be lost too.`,
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
		existing, err := db.ListDMGroups("")
		if err != nil {
			return fmt.Errorf("failed to check for existing Data Manager rules: %w", err)
		}
		existingRulesets, err := db.ListDMRulesets("")
		if err != nil {
			return fmt.Errorf("failed to check for existing Data Manager rules: %w", err)
		}
		if len(existing) > 0 || len(existingRulesets) > 0 {
			return fmt.Errorf("config.db already has Data Manager rules - pass --force to wipe them and import fresh (this replaces, it does not merge)")
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
		verb = "Replaced existing Data Manager rules with"
	}
	fmt.Printf("%s %d groups and %d rulesets from %s\n", verb, len(groups), len(rulesets), dmImportPath)
	return nil
}
