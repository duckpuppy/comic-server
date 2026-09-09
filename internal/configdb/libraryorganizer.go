package configdb

import (
	"database/sql"
	"errors"
	"fmt"
)

// LOProfile is one Library Organizer profile's scalar settings - mirrors
// losettingsx.dat's <Profile> element. Caller-supplied ID (a UUID,
// matching the pattern established for dm_groups/dm_rulesets), since the
// import path needs to know a profile's ID up front to attach its items
// and exclude rules.
type LOProfile struct {
	ID                    string
	Name                  string
	BaseFolder            string
	FolderTemplate        string
	FileTemplate          string
	EmptyFolder           string
	Mode                  string // "Move" or "Copy"
	CopyMode              bool
	UseFolder             bool
	UseFileName           bool
	ReplaceMultipleSpaces bool
	AutoSpaceFields       bool
	RemoveEmptyFolder     bool
	MoveFileless          bool
	FilelessFormat        string
	FailEmptyValues       bool
	MoveFailed            bool
	FailedFolder          string
	ExcludeMode           string // "Only" or "Do not"
	ExcludeOperator       string // "Any" or "All"
	SortOrder             int
}

// LOProfileItem is one entry in one of a profile's seven Name/Value
// collections (IllegalCharacters, Months, Prefix, Postfix, Seperator,
// TextBox, EmptyData) - see schema.go's createLibraryOrganizerTables doc
// comment for why these share one generic table instead of seven.
type LOProfileItem struct {
	ID        int64
	ProfileID string
	Category  string
	Name      string
	Value     string
}

// LOExcludeRule is one condition in a profile's exclude-rule set (see
// comic-server-3bz.3 for evaluation).
type LOExcludeRule struct {
	ID        int64
	ProfileID string
	Field     string
	Operator  string
	Value     string
	SortOrder int
}

// CreateLOProfile creates a new profile.
func (db *DB) CreateLOProfile(p LOProfile) error {
	_, err := db.Exec(`
		INSERT INTO lo_profiles (
			id, name, base_folder, folder_template, file_template, empty_folder,
			mode, copy_mode, use_folder, use_filename, replace_multiple_spaces,
			auto_space_fields, remove_empty_folder, move_fileless, fileless_format,
			fail_empty_values, move_failed, failed_folder, exclude_mode, exclude_operator,
			sort_order
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.Name, p.BaseFolder, p.FolderTemplate, p.FileTemplate, p.EmptyFolder,
		p.Mode, p.CopyMode, p.UseFolder, p.UseFileName, p.ReplaceMultipleSpaces,
		p.AutoSpaceFields, p.RemoveEmptyFolder, p.MoveFileless, p.FilelessFormat,
		p.FailEmptyValues, p.MoveFailed, p.FailedFolder, p.ExcludeMode, p.ExcludeOperator,
		p.SortOrder)
	if err != nil {
		return fmt.Errorf("create lo_profile %s (%s): %w", p.ID, p.Name, err)
	}
	return nil
}

const loProfileCols = `id, name, base_folder, folder_template, file_template, empty_folder,
	mode, copy_mode, use_folder, use_filename, replace_multiple_spaces,
	auto_space_fields, remove_empty_folder, move_fileless, fileless_format,
	fail_empty_values, move_failed, failed_folder, exclude_mode, exclude_operator,
	sort_order`

func scanLOProfile(row interface{ Scan(...any) error }) (LOProfile, error) {
	var p LOProfile
	err := row.Scan(&p.ID, &p.Name, &p.BaseFolder, &p.FolderTemplate, &p.FileTemplate, &p.EmptyFolder,
		&p.Mode, &p.CopyMode, &p.UseFolder, &p.UseFileName, &p.ReplaceMultipleSpaces,
		&p.AutoSpaceFields, &p.RemoveEmptyFolder, &p.MoveFileless, &p.FilelessFormat,
		&p.FailEmptyValues, &p.MoveFailed, &p.FailedFolder, &p.ExcludeMode, &p.ExcludeOperator,
		&p.SortOrder)
	return p, err
}

// GetLOProfile returns one profile by ID, or nil if it doesn't exist.
func (db *DB) GetLOProfile(id string) (*LOProfile, error) {
	row := db.QueryRow(`SELECT `+loProfileCols+` FROM lo_profiles WHERE id = ?`, id)
	p, err := scanLOProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get lo_profile %s: %w", id, err)
	}
	return &p, nil
}

// ListLOProfiles returns every profile, ordered by sort_order.
func (db *DB) ListLOProfiles() ([]LOProfile, error) {
	rows, err := db.Query(`SELECT ` + loProfileCols + ` FROM lo_profiles ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("list lo_profiles: %w", err)
	}
	defer rows.Close()

	var profiles []LOProfile
	for rows.Next() {
		p, err := scanLOProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("scan lo_profile: %w", err)
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

// DeleteLOProfile removes a profile and, via ON DELETE CASCADE, its items
// and exclude rules.
func (db *DB) DeleteLOProfile(id string) error {
	if _, err := db.Exec(`DELETE FROM lo_profiles WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete lo_profile %s: %w", id, err)
	}
	return nil
}

// CreateLOProfileItem adds one Name/Value entry to a profile's category
// collection, returning its new autoincrement ID.
func (db *DB) CreateLOProfileItem(item LOProfileItem) (int64, error) {
	result, err := db.Exec(`
		INSERT INTO lo_profile_items (profile_id, category, name, value)
		VALUES (?, ?, ?, ?)
	`, item.ProfileID, item.Category, item.Name, item.Value)
	if err != nil {
		return 0, fmt.Errorf("create lo_profile_item for profile %s (%s): %w", item.ProfileID, item.Category, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create lo_profile_item for profile %s (%s): %w", item.ProfileID, item.Category, err)
	}
	return id, nil
}

// ListLOProfileItems returns every item in one profile's category
// collection (e.g. "illegal_characters", "months").
func (db *DB) ListLOProfileItems(profileID, category string) ([]LOProfileItem, error) {
	rows, err := db.Query(`
		SELECT id, name, value FROM lo_profile_items
		WHERE profile_id = ? AND category = ?
	`, profileID, category)
	if err != nil {
		return nil, fmt.Errorf("list lo_profile_items for profile %s (%s): %w", profileID, category, err)
	}
	defer rows.Close()

	var items []LOProfileItem
	for rows.Next() {
		item := LOProfileItem{ProfileID: profileID, Category: category}
		if err := rows.Scan(&item.ID, &item.Name, &item.Value); err != nil {
			return nil, fmt.Errorf("scan lo_profile_item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// CreateLOExcludeRule adds one exclude-rule condition to a profile,
// returning its new autoincrement ID.
func (db *DB) CreateLOExcludeRule(rule LOExcludeRule) (int64, error) {
	result, err := db.Exec(`
		INSERT INTO lo_exclude_rules (profile_id, field, operator, value, sort_order)
		VALUES (?, ?, ?, ?, ?)
	`, rule.ProfileID, rule.Field, rule.Operator, rule.Value, rule.SortOrder)
	if err != nil {
		return 0, fmt.Errorf("create lo_exclude_rule for profile %s: %w", rule.ProfileID, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create lo_exclude_rule for profile %s: %w", rule.ProfileID, err)
	}
	return id, nil
}

// ListLOExcludeRules returns every exclude rule for a profile, in
// sort_order.
func (db *DB) ListLOExcludeRules(profileID string) ([]LOExcludeRule, error) {
	rows, err := db.Query(`
		SELECT id, field, operator, value, sort_order FROM lo_exclude_rules
		WHERE profile_id = ? ORDER BY sort_order, id
	`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list lo_exclude_rules for profile %s: %w", profileID, err)
	}
	defer rows.Close()

	var rules []LOExcludeRule
	for rows.Next() {
		r := LOExcludeRule{ProfileID: profileID}
		if err := rows.Scan(&r.ID, &r.Field, &r.Operator, &r.Value, &r.SortOrder); err != nil {
			return nil, fmt.Errorf("scan lo_exclude_rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}
