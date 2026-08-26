package configdb

import (
	"database/sql"
	"errors"
	"fmt"
)

// KomgaTarget maps one comic-server smart list to one Komga collection or
// read list. Mirrors config.KomgaTarget's fields, but is its own type -
// configdb doesn't import internal/config, matching the same separation
// devices.go's Device/DeviceList already established.
type KomgaTarget struct {
	ListID         string
	ListName       string
	Type           string // "collection" or "readlist" - validated by callers, not here
	KomgaName      string
	Enabled        bool
	SyncReadStatus bool
}

// ListKomgaTargets returns every configured Komga target, ordered by
// list_id.
func (db *DB) ListKomgaTargets() ([]KomgaTarget, error) {
	rows, err := db.Query(`SELECT list_id, list_name, type, komga_name, enabled, sync_read_status FROM komga_targets ORDER BY list_id`)
	if err != nil {
		return nil, fmt.Errorf("list komga targets: %w", err)
	}
	defer rows.Close()

	var targets []KomgaTarget
	for rows.Next() {
		var t KomgaTarget
		if err := rows.Scan(&t.ListID, &t.ListName, &t.Type, &t.KomgaName, &t.Enabled, &t.SyncReadStatus); err != nil {
			return nil, fmt.Errorf("scan komga target: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// GetKomgaTarget returns the Komga target configured for one list, or nil
// if none is.
func (db *DB) GetKomgaTarget(listID string) (*KomgaTarget, error) {
	var t KomgaTarget
	t.ListID = listID
	err := db.QueryRow(`SELECT list_name, type, komga_name, enabled, sync_read_status FROM komga_targets WHERE list_id = ?`, listID).
		Scan(&t.ListName, &t.Type, &t.KomgaName, &t.Enabled, &t.SyncReadStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get komga target %s: %w", listID, err)
	}
	return &t, nil
}

// CreateKomgaTarget creates a Komga target for a list. Returns an error if
// a target already exists for that list_id - callers that want
// create-or-update should check GetKomgaTarget first, not rely on this to
// upsert.
func (db *DB) CreateKomgaTarget(t KomgaTarget) error {
	_, err := db.Exec(`
		INSERT INTO komga_targets (list_id, list_name, type, komga_name, enabled, sync_read_status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, t.ListID, t.ListName, t.Type, t.KomgaName, t.Enabled, t.SyncReadStatus)
	if err != nil {
		return fmt.Errorf("create komga target for list %s: %w", t.ListID, err)
	}
	return nil
}

// UpdateKomgaTarget updates the type, komga_name, enabled, and
// sync_read_status of an existing target. Returns sql.ErrNoRows if no
// target exists for listID.
func (db *DB) UpdateKomgaTarget(listID, targetType, komgaName string, enabled, syncReadStatus bool) error {
	result, err := db.Exec(`
		UPDATE komga_targets SET type = ?, komga_name = ?, enabled = ?, sync_read_status = ?
		WHERE list_id = ?
	`, targetType, komgaName, enabled, syncReadStatus, listID)
	if err != nil {
		return fmt.Errorf("update komga target for list %s: %w", listID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update komga target for list %s: %w", listID, err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteKomgaTarget removes the Komga target for a list. Not an error if
// none existed - callers check existence first if they need to distinguish
// "removed" from "wasn't there."
func (db *DB) DeleteKomgaTarget(listID string) error {
	if _, err := db.Exec(`DELETE FROM komga_targets WHERE list_id = ?`, listID); err != nil {
		return fmt.Errorf("delete komga target for list %s: %w", listID, err)
	}
	return nil
}
