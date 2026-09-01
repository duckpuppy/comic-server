package configdb

import "fmt"

// DMImportGroup/DMImportRuleset are the persistence-layer shape of an
// imported Data Manager group/ruleset - structurally mirrors
// internal/datamanager.ImportedGroup/ImportedRuleset without importing
// that package (same separation KomgaTarget already established from
// internal/config - see comic-server-764.4's design notes). Callers
// convert at the boundary (cmd/datamanager_import.go).
type DMImportGroup struct {
	ID        string
	ParentID  string
	Name      string
	Comment   string
	Disabled  bool
	SortOrder int
}

type DMImportRuleset struct {
	ID        string
	GroupID   string
	Name      string
	Comment   string
	Mode      string
	Disabled  bool
	SortOrder int
	Rules     []DMImportRule
	Actions   []DMImportAction
}

type DMImportRule struct {
	Field    string
	Modifier string
	Value    string
}

type DMImportAction struct {
	Field    string
	Modifier string
	Value    string
}

// ImportDataManagerRules writes a fully-parsed dataman.dat (groups,
// rulesets, and each ruleset's rules/actions) into dm_groups/dm_rulesets/
// dm_rules/dm_actions in one transaction - either the whole import lands
// or none of it does, since a partial import would leave dangling
// ParentID/GroupID references pointing at half-written data.
func (db *DB) ImportDataManagerRules(groups []DMImportGroup, rulesets []DMImportRuleset) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin dm import transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	for _, g := range groups {
		if _, err := tx.Exec(`
			INSERT INTO dm_groups (id, parent_id, name, comment, disabled, sort_order)
			VALUES (?, NULLIF(?, ''), ?, ?, ?, ?)
		`, g.ID, g.ParentID, g.Name, g.Comment, g.Disabled, g.SortOrder); err != nil {
			return fmt.Errorf("import dm_group %s (%s): %w", g.ID, g.Name, err)
		}
	}

	for _, rs := range rulesets {
		mode := rs.Mode
		if mode == "" {
			mode = "And"
		}
		if _, err := tx.Exec(`
			INSERT INTO dm_rulesets (id, group_id, name, comment, mode, disabled, sort_order)
			VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, ?)
		`, rs.ID, rs.GroupID, rs.Name, rs.Comment, mode, rs.Disabled, rs.SortOrder); err != nil {
			return fmt.Errorf("import dm_ruleset %s (%s): %w", rs.ID, rs.Name, err)
		}
		for i, r := range rs.Rules {
			if _, err := tx.Exec(`
				INSERT INTO dm_rules (ruleset_id, field, modifier, value, sort_order)
				VALUES (?, ?, ?, ?, ?)
			`, rs.ID, r.Field, r.Modifier, r.Value, i); err != nil {
				return fmt.Errorf("import dm_rule for ruleset %s: %w", rs.ID, err)
			}
		}
		for i, a := range rs.Actions {
			if _, err := tx.Exec(`
				INSERT INTO dm_actions (ruleset_id, field, modifier, value, sort_order)
				VALUES (?, ?, ?, ?, ?)
			`, rs.ID, a.Field, a.Modifier, a.Value, i); err != nil {
				return fmt.Errorf("import dm_action for ruleset %s: %w", rs.ID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dm import transaction: %w", err)
	}
	return nil
}
