package configdb

import "fmt"

// LOImportProfile/LOImportItem/LOImportExcludeRule are the persistence-
// layer shape of an imported Library Organizer profile - structurally
// mirrors internal/libraryorganizer.ImportedProfile/ImportedItem/
// ImportedExcludeRule without importing that package (same separation
// KomgaTarget/DMImportGroup already established). Callers convert at the
// boundary (cmd/libraryorganizer_import.go).
type LOImportProfile struct {
	ID                    string
	Name                  string
	BaseFolder            string
	FolderTemplate        string
	FileTemplate          string
	EmptyFolder           string
	Mode                  string
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
	ExcludeMode           string
	ExcludeOperator       string
	SortOrder             int

	Items        []LOImportItem
	ExcludeRules []LOImportExcludeRule
}

type LOImportItem struct {
	Category string
	Name     string
	Value    string
}

type LOImportExcludeRule struct {
	Field    string
	Operator string
	Value    string
}

// ImportLibraryOrganizerProfiles writes a fully-parsed losettingsx.dat
// (profiles, each one's item collections, and exclude rules) into
// lo_profiles/lo_profile_items/lo_exclude_rules in one transaction -
// either the whole import lands or none of it does, matching
// ImportDataManagerRules' own atomicity reasoning.
//
// wipeExisting, when true, deletes every existing lo_profiles row
// (cascading to items/exclude rules) before inserting the fresh parse -
// same re-import semantics comic-server-cge established for Data Manager
// after the user pointed out re-import is a real recurring workflow
// while they're still actively using ComicRack, not a one-time
// migration; the same reasoning applies here.
func (db *DB) ImportLibraryOrganizerProfiles(profiles []LOImportProfile, wipeExisting bool) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin library organizer import transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	if wipeExisting {
		if _, err := tx.Exec(`DELETE FROM lo_profiles`); err != nil {
			return fmt.Errorf("wipe existing lo_profiles: %w", err)
		}
	}

	for _, p := range profiles {
		if _, err := tx.Exec(`
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
			p.SortOrder); err != nil {
			return fmt.Errorf("import lo_profile %s (%s): %w", p.ID, p.Name, err)
		}

		for _, item := range p.Items {
			if _, err := tx.Exec(`
				INSERT INTO lo_profile_items (profile_id, category, name, value)
				VALUES (?, ?, ?, ?)
			`, p.ID, item.Category, item.Name, item.Value); err != nil {
				return fmt.Errorf("import lo_profile_item for profile %s (%s): %w", p.ID, item.Category, err)
			}
		}

		for i, rule := range p.ExcludeRules {
			if _, err := tx.Exec(`
				INSERT INTO lo_exclude_rules (profile_id, field, operator, value, sort_order)
				VALUES (?, ?, ?, ?, ?)
			`, p.ID, rule.Field, rule.Operator, rule.Value, i); err != nil {
				return fmt.Errorf("import lo_exclude_rule for profile %s: %w", p.ID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit library organizer import transaction: %w", err)
	}
	return nil
}
