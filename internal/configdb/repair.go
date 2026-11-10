package configdb

import (
	"database/sql"
	"errors"
	"fmt"
)

// Errors returned by RepairDevice.
var (
	// ErrRepairNotRegistered: the device to move from has no registration.
	ErrRepairNotRegistered = errors.New("the device to re-pair is not registered")
	// ErrRepairSameDevice: old and new IDs are the same.
	ErrRepairSameDevice = errors.New("the new device ID is the same as the old one")
	// ErrRepairTargetInUse: the new ID already has lists or real sync history,
	// so moving onto it would hide or lose that.
	ErrRepairTargetInUse = errors.New("the new device ID already has lists or sync history")
)

// RepairDevice moves a registered device's configuration to a new device ID
// (tracker #53), for an app that made a new Sync ID after a reinstall or a
// new phone. In one transaction it moves the devices row (friendly name,
// default settings, last seen, manual address), the device_lists rows and
// the sync_history rows from oldID to newID. If anything fails, everything
// stays on oldID.
//
// newID may already be registered, but only as an empty entry: no assigned
// lists and no real sync history (a sync that completed or changed books).
// That is the usual state when someone pressed Register on the new device
// before noticing its lists were missing. The empty entry is replaced.
// Failed attempts such as "device is not registered" do not count as history.
// Anything else is ErrRepairTargetInUse.
func (db *DB) RepairDevice(oldID, newID string) error {
	if oldID == newID {
		return ErrRepairSameDevice
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("re-pair %s: begin: %w", oldID, err)
	}
	defer tx.Rollback()

	var name, address string
	var lastSeen, settings *string
	err = tx.QueryRow(`SELECT friendly_name, last_seen, default_settings, address FROM devices WHERE device_id = ?`, oldID).
		Scan(&name, &lastSeen, &settings, &address)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRepairNotRegistered
		}
		return fmt.Errorf("re-pair %s: read device: %w", oldID, err)
	}

	var lists, realHistory int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM device_lists WHERE device_id = ?`, newID).Scan(&lists); err != nil {
		return fmt.Errorf("re-pair %s: count target lists: %w", oldID, err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sync_history WHERE device_id = ?
		AND (status = 'completed' OR books_added + books_updated + books_deleted > 0)`, newID).Scan(&realHistory); err != nil {
		return fmt.Errorf("re-pair %s: count target history: %w", oldID, err)
	}
	if lists > 0 || realHistory > 0 {
		return ErrRepairTargetInUse
	}

	stmts := []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM devices WHERE device_id = ?`, []any{newID}}, // the empty entry, if any
		{`INSERT INTO devices (device_id, friendly_name, last_seen, default_settings, address) VALUES (?, ?, ?, ?, ?)`,
			[]any{newID, name, lastSeen, settings, address}},
		{`UPDATE device_lists SET device_id = ? WHERE device_id = ?`, []any{newID, oldID}},
		{`UPDATE sync_history SET device_id = ? WHERE device_id = ?`, []any{newID, oldID}},
		{`DELETE FROM devices WHERE device_id = ?`, []any{oldID}},
	}
	for _, st := range stmts {
		if _, err := tx.Exec(st.sql, st.args...); err != nil {
			return fmt.Errorf("re-pair %s to %s: %w", oldID, newID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("re-pair %s: commit: %w", oldID, err)
	}
	return nil
}
