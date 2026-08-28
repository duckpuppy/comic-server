package configdb

import (
	"database/sql"
	"fmt"
	"time"
)

// SyncHistoryRecord is one persisted sync history entry - mirrors
// syncstate.SyncState, but kept as this package's own type rather than
// importing internal/syncstate, since configdb is a storage layer and
// shouldn't depend on the package that consumes it.
type SyncHistoryRecord struct {
	DeviceID     string
	DeviceIP     string
	DeviceName   string
	StartTime    time.Time
	EndTime      *time.Time
	Status       string
	Progress     int
	BooksTotal   int
	BooksAdded   int
	BooksUpdated int
	BooksDeleted int
	ErrorCount   int
	ErrorMessage string
}

// AppendSyncHistory inserts one completed/failed/aborted sync record, then
// prunes the table back down to its most recent `keep` rows. The table
// only exists to back syncstate.Manager's bounded in-memory history ring
// buffer across restarts, so it's kept at the same row cap rather than
// growing unbounded or needing a separate age-based sweep.
func (db *DB) AppendSyncHistory(rec SyncHistoryRecord, keep int) error {
	var endTime sql.NullString
	if rec.EndTime != nil {
		endTime = sql.NullString{String: rec.EndTime.Format(time.RFC3339), Valid: true}
	}

	_, err := db.Exec(`
		INSERT INTO sync_history (
			device_id, device_ip, device_name, start_time, end_time, status,
			progress, books_total, books_added, books_updated, books_deleted,
			error_count, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rec.DeviceID, rec.DeviceIP, rec.DeviceName, rec.StartTime.Format(time.RFC3339), endTime, rec.Status,
		rec.Progress, rec.BooksTotal, rec.BooksAdded, rec.BooksUpdated, rec.BooksDeleted,
		rec.ErrorCount, rec.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("append sync history for device %s: %w", rec.DeviceID, err)
	}

	if keep > 0 {
		if _, err := db.Exec(`DELETE FROM sync_history WHERE id NOT IN (SELECT id FROM sync_history ORDER BY id DESC LIMIT ?)`, keep); err != nil {
			return fmt.Errorf("prune sync history: %w", err)
		}
	}

	return nil
}

// LoadRecentSyncHistory returns up to `limit` most recent sync history
// records, oldest first - matching syncstate.Manager's FIFO in-memory
// ordering, so it can be used directly to warm that cache at startup.
func (db *DB) LoadRecentSyncHistory(limit int) ([]SyncHistoryRecord, error) {
	rows, err := db.Query(`
		SELECT device_id, device_ip, device_name, start_time, end_time, status,
			progress, books_total, books_added, books_updated, books_deleted,
			error_count, error_message
		FROM sync_history ORDER BY id DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("load sync history: %w", err)
	}
	defer rows.Close()

	var records []SyncHistoryRecord
	for rows.Next() {
		var rec SyncHistoryRecord
		var startTime string
		var endTime sql.NullString
		if err := rows.Scan(
			&rec.DeviceID, &rec.DeviceIP, &rec.DeviceName, &startTime, &endTime, &rec.Status,
			&rec.Progress, &rec.BooksTotal, &rec.BooksAdded, &rec.BooksUpdated, &rec.BooksDeleted,
			&rec.ErrorCount, &rec.ErrorMessage,
		); err != nil {
			return nil, fmt.Errorf("scan sync history: %w", err)
		}
		rec.StartTime, err = time.Parse(time.RFC3339, startTime)
		if err != nil {
			return nil, fmt.Errorf("parse sync history start_time: %w", err)
		}
		if endTime.Valid {
			t, err := time.Parse(time.RFC3339, endTime.String)
			if err != nil {
				return nil, fmt.Errorf("parse sync history end_time: %w", err)
			}
			rec.EndTime = &t
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load sync history: %w", err)
	}

	// Rows come back newest-first (id DESC); reverse to oldest-first.
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	return records, nil
}
