package configdb

import (
	"database/sql"
	"errors"
	"fmt"
)

// ValidThemes are the only accepted values for the stored theme setting.
var ValidThemes = map[string]bool{"light": true, "dark": true, "system": true}

// GetTheme returns the stored default theme ("light", "dark", or
// "system"), or "system" if none has ever been set - matching the
// ui_settings table's own column default, so a database that predates
// this feature (migrated via migrateV7ToV8) and one created fresh both
// behave the same without a special no-rows case.
func (db *DB) GetTheme() (string, error) {
	var theme string
	err := db.QueryRow(`SELECT theme FROM ui_settings WHERE id = 1`).Scan(&theme)
	if errors.Is(err, sql.ErrNoRows) {
		return "system", nil
	}
	if err != nil {
		return "", fmt.Errorf("get theme: %w", err)
	}
	return theme, nil
}

// SetTheme stores the default theme. Callers must pass a value present in
// ValidThemes.
func (db *DB) SetTheme(theme string) error {
	if !ValidThemes[theme] {
		return fmt.Errorf("invalid theme %q", theme)
	}
	_, err := db.Exec(`
		INSERT INTO ui_settings (id, theme) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET theme = excluded.theme
	`, theme)
	if err != nil {
		return fmt.Errorf("set theme: %w", err)
	}
	return nil
}
