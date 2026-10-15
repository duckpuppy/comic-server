package configdb

import (
	"fmt"
	"time"
)

// Archive health (comic-server-4inw): comic archives that failed to open
// when something read them. Recorded lazily from reads that already happen,
// never from a scan. Created on first use, no schema bump. Keyed by the
// resolved on-disk path; version is the file's mtime-size at the time, so a
// replaced file stops being flagged without anyone clearing it.
func (db *DB) ensureArchiveHealthTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS archive_health (
		path    TEXT PRIMARY KEY,
		version TEXT NOT NULL,
		reason  TEXT NOT NULL,
		seen_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create archive_health table: %w", err)
	}
	return nil
}

// ArchiveFailure is one recorded unreadable archive.
type ArchiveFailure struct {
	Path    string
	Version string
	Reason  string
}

// RecordArchiveFailure stores (or refreshes) a failure for path.
func (db *DB) RecordArchiveFailure(f ArchiveFailure) error {
	if err := db.ensureArchiveHealthTable(); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT INTO archive_health (path, version, reason, seen_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET version = excluded.version, reason = excluded.reason, seen_at = excluded.seen_at`,
		f.Path, f.Version, f.Reason, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("record archive failure: %w", err)
	}
	return nil
}

// ArchiveFailures returns every recorded failure.
func (db *DB) ArchiveFailures() ([]ArchiveFailure, error) {
	if err := db.ensureArchiveHealthTable(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT path, version, reason FROM archive_health`)
	if err != nil {
		return nil, fmt.Errorf("list archive failures: %w", err)
	}
	defer rows.Close()
	var out []ArchiveFailure
	for rows.Next() {
		var f ArchiveFailure
		if err := rows.Scan(&f.Path, &f.Version, &f.Reason); err != nil {
			return nil, fmt.Errorf("scan archive failure: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteArchiveFailure forgets a path (its file changed or was fixed).
func (db *DB) DeleteArchiveFailure(path string) error {
	if err := db.ensureArchiveHealthTable(); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM archive_health WHERE path = ?`, path); err != nil {
		return fmt.Errorf("delete archive failure: %w", err)
	}
	return nil
}
