package configdb

import (
	"database/sql"
	"fmt"
)

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
// ParentID/GroupID references pointing at half-written data. Every group
// and ruleset this function inserts is tagged source="import".
//
// wipeExisting, when true, deletes every EXISTING source="import"
// dm_groups/dm_rulesets row (cascading to dm_rules/dm_actions) before
// inserting the fresh parse, in the SAME transaction as the insert - so a
// re-import (the user's real workflow: they keep authoring rules in
// ComicRack's Data Manager plugin until it's fully retired, see
// comic-server-cge) replaces the old imported rule set atomically rather
// than duplicating it, and a failed re-import can never leave the
// database wiped with nothing re-inserted.
//
// Rows with source="manual" (created by hand through the native rule
// editor, comic-server-tj6o) are never deleted by a wipe - that's the
// whole point of tracking source at all (comic-server-vkpq). A manual
// group/ruleset nested INSIDE an import-sourced group that's about to be
// deleted is re-parented up to the nearest surviving (non-import)
// ancestor first (reparentManualDescendants), so it isn't destroyed as
// collateral damage by that group's ON DELETE CASCADE - a plain "delete
// every import-sourced group" would otherwise take manual content down
// with it whenever it lived inside an imported folder.
func (db *DB) ImportDataManagerRules(groups []DMImportGroup, rulesets []DMImportRuleset, wipeExisting bool) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin dm import transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	if wipeExisting {
		if err := reparentManualDescendantsOfImportGroups(tx); err != nil {
			return fmt.Errorf("reparent manual content before wipe: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM dm_rulesets WHERE source = 'import'`); err != nil {
			return fmt.Errorf("wipe existing imported dm_rulesets: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM dm_groups WHERE source = 'import'`); err != nil {
			return fmt.Errorf("wipe existing imported dm_groups: %w", err)
		}
	}

	for _, g := range groups {
		if _, err := tx.Exec(`
			INSERT INTO dm_groups (id, parent_id, name, comment, disabled, sort_order, source)
			VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, 'import')
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
			INSERT INTO dm_rulesets (id, group_id, name, comment, mode, disabled, sort_order, source)
			VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, ?, 'import')
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

// HasImportedDataManagerRules reports whether config.db holds any
// source="import" group or ruleset - the CLI import command's guard
// against accidentally duplicating a previous import (comic-server-cge)
// checks this rather than "any rules at all" (comic-server-vkpq), since a
// database that only has hand-authored source="manual" content has never
// been imported into and a plain (non---force) import should be free to
// proceed and merge with it.
func (db *DB) HasImportedDataManagerRules() (bool, error) {
	var exists int
	err := db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM dm_groups WHERE source = 'import'
		UNION ALL
		SELECT 1 FROM dm_rulesets WHERE source = 'import'
	)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check for existing imported dm rules: %w", err)
	}
	return exists != 0, nil
}

// reparentManualDescendantsOfImportGroups moves every source="manual"
// dm_groups/dm_rulesets row out from under any import-sourced group it's
// currently nested in, up to the nearest surviving ancestor (the first
// non-import group walking up the ORIGINAL parent chain, or "" for root)
// - called right before a wipe deletes every import-sourced group, so
// ON DELETE CASCADE can never reach a manual row. Reads a snapshot of
// dm_groups' id/parent_id/source once and computes every new parent
// against that fixed snapshot, so the order the UPDATEs run in doesn't
// matter - deeper manual descendants whose immediate parent is also
// manual are left untouched, since only a row whose *own* parent is
// import-sourced needs to move.
func reparentManualDescendantsOfImportGroups(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, parent_id, source FROM dm_groups`)
	if err != nil {
		return fmt.Errorf("snapshot dm_groups: %w", err)
	}
	type groupInfo struct {
		parent string
		source string
	}
	groups := make(map[string]groupInfo)
	for rows.Next() {
		var id, source string
		var parent sql.NullString
		if err := rows.Scan(&id, &parent, &source); err != nil {
			rows.Close()
			return fmt.Errorf("scan dm_groups snapshot: %w", err)
		}
		groups[id] = groupInfo{parent: parent.String, source: source}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read dm_groups snapshot: %w", err)
	}
	rows.Close()

	isImportGroup := func(id string) bool { return groups[id].source == "import" }
	survivingAncestor := func(start string) string {
		cur := start
		for cur != "" && isImportGroup(cur) {
			cur = groups[cur].parent
		}
		return cur
	}

	for id, info := range groups {
		if info.source != "manual" || info.parent == "" || !isImportGroup(info.parent) {
			continue
		}
		newParent := survivingAncestor(info.parent)
		if _, err := tx.Exec(`UPDATE dm_groups SET parent_id = NULLIF(?, '') WHERE id = ?`, newParent, id); err != nil {
			return fmt.Errorf("reparent manual dm_group %s: %w", id, err)
		}
	}

	rsRows, err := tx.Query(`SELECT id, group_id, source FROM dm_rulesets`)
	if err != nil {
		return fmt.Errorf("snapshot dm_rulesets: %w", err)
	}
	type rsInfo struct {
		id, group, source string
	}
	var rulesets []rsInfo
	for rsRows.Next() {
		var id, source string
		var group sql.NullString
		if err := rsRows.Scan(&id, &group, &source); err != nil {
			rsRows.Close()
			return fmt.Errorf("scan dm_rulesets snapshot: %w", err)
		}
		rulesets = append(rulesets, rsInfo{id: id, group: group.String, source: source})
	}
	if err := rsRows.Err(); err != nil {
		return fmt.Errorf("read dm_rulesets snapshot: %w", err)
	}
	rsRows.Close()

	for _, rs := range rulesets {
		if rs.source != "manual" || rs.group == "" || !isImportGroup(rs.group) {
			continue
		}
		newParent := survivingAncestor(rs.group)
		if _, err := tx.Exec(`UPDATE dm_rulesets SET group_id = NULLIF(?, '') WHERE id = ?`, newParent, rs.id); err != nil {
			return fmt.Errorf("reparent manual dm_ruleset %s: %w", rs.id, err)
		}
	}

	return nil
}
