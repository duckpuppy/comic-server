package configdb

import (
	"database/sql"
	"errors"
	"fmt"
)

// DMGroup is one folder in the Data Manager rule hierarchy - mirrors
// dataman.dat's nested <group>/<disabled> elements. ParentID is "" for a
// top-level group. See comic-server-764.4's design notes (schema.go's
// createDataManagerTables) for why SortOrder and Disabled exist.
type DMGroup struct {
	ID        string
	ParentID  string
	Name      string
	Comment   string
	Disabled  bool
	SortOrder int
}

// DMRuleset is one named rule container - mirrors dataman.dat's
// <ruleset> element. GroupID is "" for a top-level ruleset (dataman.dat
// allows a ruleset directly under <collection>, not just inside a group).
type DMRuleset struct {
	ID        string
	GroupID   string
	Name      string
	Comment   string
	Mode      string // "And" or "Or"
	Disabled  bool
	SortOrder int
}

// DMRule is one condition of a ruleset - field/modifier/value, matching
// internal/datamanager.Rule's shape. configdb intentionally doesn't
// import internal/datamanager (same separation KomgaTarget already
// established from internal/config) - callers convert at the boundary.
type DMRule struct {
	ID        int64
	RulesetID string
	Field     string
	Modifier  string
	Value     string
	SortOrder int
}

// DMAction is one write a ruleset performs when its rules match -
// matches internal/datamanager.Action's shape.
type DMAction struct {
	ID        int64
	RulesetID string
	Field     string
	Modifier  string
	Value     string
	SortOrder int
}

// CreateDMGroup creates a new rule group. Callers supply the ID (a UUID,
// matching how smart-list IDs are caller-supplied elsewhere in
// comic-server) rather than this generating one, since the import path
// (comic-server-764.5) needs to know child groups'/rulesets' parent IDs
// up front while building a tree, not after an autoincrement round-trip.
func (db *DB) CreateDMGroup(g DMGroup) error {
	_, err := db.Exec(`
		INSERT INTO dm_groups (id, parent_id, name, comment, disabled, sort_order)
		VALUES (?, NULLIF(?, ''), ?, ?, ?, ?)
	`, g.ID, g.ParentID, g.Name, g.Comment, g.Disabled, g.SortOrder)
	if err != nil {
		return fmt.Errorf("create dm_group %s: %w", g.ID, err)
	}
	return nil
}

// GetDMGroup returns one group by ID, or nil if it doesn't exist.
func (db *DB) GetDMGroup(id string) (*DMGroup, error) {
	g := DMGroup{ID: id}
	var parentID sql.NullString
	err := db.QueryRow(`SELECT parent_id, name, comment, disabled, sort_order FROM dm_groups WHERE id = ?`, id).
		Scan(&parentID, &g.Name, &g.Comment, &g.Disabled, &g.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get dm_group %s: %w", id, err)
	}
	g.ParentID = parentID.String
	return &g, nil
}

// ListDMGroups returns every group whose parent is parentID (pass "" for
// top-level groups), ordered by sort_order.
func (db *DB) ListDMGroups(parentID string) ([]DMGroup, error) {
	const cols = `id, parent_id, name, comment, disabled, sort_order`
	var rows *sql.Rows
	var err error
	if parentID == "" {
		rows, err = db.Query(`SELECT ` + cols + ` FROM dm_groups WHERE parent_id IS NULL ORDER BY sort_order`)
	} else {
		rows, err = db.Query(`SELECT `+cols+` FROM dm_groups WHERE parent_id = ? ORDER BY sort_order`, parentID)
	}
	if err != nil {
		return nil, fmt.Errorf("list dm_groups under %q: %w", parentID, err)
	}
	defer rows.Close()

	var groups []DMGroup
	for rows.Next() {
		var g DMGroup
		var pid sql.NullString
		if err := rows.Scan(&g.ID, &pid, &g.Name, &g.Comment, &g.Disabled, &g.SortOrder); err != nil {
			return nil, fmt.Errorf("scan dm_group: %w", err)
		}
		g.ParentID = pid.String
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// DeleteDMGroup removes a group and, via ON DELETE CASCADE, every
// descendant group/ruleset/rule/action nested inside it.
func (db *DB) DeleteDMGroup(id string) error {
	if _, err := db.Exec(`DELETE FROM dm_groups WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete dm_group %s: %w", id, err)
	}
	return nil
}

// CreateDMRuleset creates a new ruleset (caller-supplied ID, same
// reasoning as CreateDMGroup).
func (db *DB) CreateDMRuleset(rs DMRuleset) error {
	mode := rs.Mode
	if mode == "" {
		mode = "And"
	}
	_, err := db.Exec(`
		INSERT INTO dm_rulesets (id, group_id, name, comment, mode, disabled, sort_order)
		VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, ?)
	`, rs.ID, rs.GroupID, rs.Name, rs.Comment, mode, rs.Disabled, rs.SortOrder)
	if err != nil {
		return fmt.Errorf("create dm_ruleset %s: %w", rs.ID, err)
	}
	return nil
}

// GetDMRuleset returns one ruleset's own row (not its rules/actions - see
// GetDMRulesetFull for that), or nil if it doesn't exist.
func (db *DB) GetDMRuleset(id string) (*DMRuleset, error) {
	rs := DMRuleset{ID: id}
	var groupID sql.NullString
	err := db.QueryRow(`SELECT group_id, name, comment, mode, disabled, sort_order FROM dm_rulesets WHERE id = ?`, id).
		Scan(&groupID, &rs.Name, &rs.Comment, &rs.Mode, &rs.Disabled, &rs.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get dm_ruleset %s: %w", id, err)
	}
	rs.GroupID = groupID.String
	return &rs, nil
}

// ListDMRulesets returns every ruleset whose group is groupID (pass "" for
// top-level rulesets), ordered by sort_order.
func (db *DB) ListDMRulesets(groupID string) ([]DMRuleset, error) {
	const cols = `id, group_id, name, comment, mode, disabled, sort_order`
	var rows *sql.Rows
	var err error
	if groupID == "" {
		rows, err = db.Query(`SELECT ` + cols + ` FROM dm_rulesets WHERE group_id IS NULL ORDER BY sort_order`)
	} else {
		rows, err = db.Query(`SELECT `+cols+` FROM dm_rulesets WHERE group_id = ? ORDER BY sort_order`, groupID)
	}
	if err != nil {
		return nil, fmt.Errorf("list dm_rulesets under %q: %w", groupID, err)
	}
	defer rows.Close()

	var rulesets []DMRuleset
	for rows.Next() {
		var rs DMRuleset
		var gid sql.NullString
		if err := rows.Scan(&rs.ID, &gid, &rs.Name, &rs.Comment, &rs.Mode, &rs.Disabled, &rs.SortOrder); err != nil {
			return nil, fmt.Errorf("scan dm_ruleset: %w", err)
		}
		rs.GroupID = gid.String
		rulesets = append(rulesets, rs)
	}
	return rulesets, rows.Err()
}

// DeleteDMRuleset removes a ruleset and, via ON DELETE CASCADE, its rules
// and actions.
func (db *DB) DeleteDMRuleset(id string) error {
	if _, err := db.Exec(`DELETE FROM dm_rulesets WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete dm_ruleset %s: %w", id, err)
	}
	return nil
}

// CreateDMRule adds one condition to a ruleset, returning its new
// autoincrement ID.
func (db *DB) CreateDMRule(r DMRule) (int64, error) {
	result, err := db.Exec(`
		INSERT INTO dm_rules (ruleset_id, field, modifier, value, sort_order)
		VALUES (?, ?, ?, ?, ?)
	`, r.RulesetID, r.Field, r.Modifier, r.Value, r.SortOrder)
	if err != nil {
		return 0, fmt.Errorf("create dm_rule for ruleset %s: %w", r.RulesetID, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create dm_rule for ruleset %s: %w", r.RulesetID, err)
	}
	return id, nil
}

// ListDMRules returns every rule belonging to rulesetID, in sort_order.
func (db *DB) ListDMRules(rulesetID string) ([]DMRule, error) {
	rows, err := db.Query(`SELECT id, field, modifier, value, sort_order FROM dm_rules WHERE ruleset_id = ? ORDER BY sort_order, id`, rulesetID)
	if err != nil {
		return nil, fmt.Errorf("list dm_rules for ruleset %s: %w", rulesetID, err)
	}
	defer rows.Close()

	var rules []DMRule
	for rows.Next() {
		r := DMRule{RulesetID: rulesetID}
		if err := rows.Scan(&r.ID, &r.Field, &r.Modifier, &r.Value, &r.SortOrder); err != nil {
			return nil, fmt.Errorf("scan dm_rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// CreateDMAction adds one action to a ruleset, returning its new
// autoincrement ID.
func (db *DB) CreateDMAction(a DMAction) (int64, error) {
	result, err := db.Exec(`
		INSERT INTO dm_actions (ruleset_id, field, modifier, value, sort_order)
		VALUES (?, ?, ?, ?, ?)
	`, a.RulesetID, a.Field, a.Modifier, a.Value, a.SortOrder)
	if err != nil {
		return 0, fmt.Errorf("create dm_action for ruleset %s: %w", a.RulesetID, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create dm_action for ruleset %s: %w", a.RulesetID, err)
	}
	return id, nil
}

// ListDMActions returns every action belonging to rulesetID, in
// sort_order - action order matters (a later action can overwrite an
// earlier one's write to the same field), unlike dm_rules' order.
func (db *DB) ListDMActions(rulesetID string) ([]DMAction, error) {
	rows, err := db.Query(`SELECT id, field, modifier, value, sort_order FROM dm_actions WHERE ruleset_id = ? ORDER BY sort_order, id`, rulesetID)
	if err != nil {
		return nil, fmt.Errorf("list dm_actions for ruleset %s: %w", rulesetID, err)
	}
	defer rows.Close()

	var actions []DMAction
	for rows.Next() {
		a := DMAction{RulesetID: rulesetID}
		if err := rows.Scan(&a.ID, &a.Field, &a.Modifier, &a.Value, &a.SortOrder); err != nil {
			return nil, fmt.Errorf("scan dm_action: %w", err)
		}
		actions = append(actions, a)
	}
	return actions, rows.Err()
}
