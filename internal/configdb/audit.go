package configdb

import (
	"fmt"
	"strings"
	"time"
)

// Audit log storage (comic-server-vqsj). Created lazily with CREATE TABLE IF
// NOT EXISTS, like the duplicate tables, so it adds no schema migration.
//
// Stored here (config.db) rather than in the library backend so it works the
// same for the XML and SQLite library backends and survives library
// re-imports.

// AuditRow is one recorded action against one book.
type AuditRow struct {
	ID          int64     `json:"id"`
	At          time.Time `json:"at"`
	BookID      string    `json:"book_id"`
	Action      string    `json:"action"`
	Series      string    `json:"series"`
	Number      string    `json:"number"`
	StageBefore string    `json:"stage_before"`
	StageAfter  string    `json:"stage_after"`
	Changes     string    `json:"changes"` // JSON array of {field, old, new}
}

// AuditFilter narrows ListAudit. Zero values mean "any".
type AuditFilter struct {
	BookID string
	Action string
	Limit  int
	Offset int
}

func (db *DB) ensureAuditTable() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS audit_log (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			at           TEXT NOT NULL,
			book_id      TEXT NOT NULL,
			action       TEXT NOT NULL,
			series       TEXT NOT NULL DEFAULT '',
			number       TEXT NOT NULL DEFAULT '',
			stage_before TEXT NOT NULL DEFAULT '',
			stage_after  TEXT NOT NULL DEFAULT '',
			changes      TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_book ON audit_log (book_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_at ON audit_log (at)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("create audit table: %w", err)
		}
	}
	return nil
}

// AppendAudit stores rows in one transaction.
func (db *DB) AppendAudit(rows []AuditRow) error {
	if len(rows) == 0 {
		return nil
	}
	if err := db.ensureAuditTable(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin audit append: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`INSERT INTO audit_log (at, book_id, action, series, number, stage_before, stage_after, changes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare audit append: %w", err)
	}
	defer stmt.Close()
	for _, r := range rows {
		at := r.At
		if at.IsZero() {
			at = time.Now()
		}
		changes := r.Changes
		if changes == "" {
			changes = "[]"
		}
		if _, err := stmt.Exec(at.UTC().Format(time.RFC3339), r.BookID, r.Action, r.Series, r.Number, r.StageBefore, r.StageAfter, changes); err != nil {
			return fmt.Errorf("append audit row: %w", err)
		}
	}
	return tx.Commit()
}

// ListAudit returns matching rows newest first, plus the total match count.
func (db *DB) ListAudit(f AuditFilter) ([]AuditRow, int, error) {
	if err := db.ensureAuditTable(); err != nil {
		return nil, 0, err
	}
	var where []string
	var args []any
	if f.BookID != "" {
		where = append(where, "book_id = ?")
		args = append(args, f.BookID)
	}
	if f.Action != "" {
		where = append(where, "action = ?")
		args = append(args, f.Action)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log`+clause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit rows: %w", err)
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := `SELECT id, at, book_id, action, series, number, stage_before, stage_after, changes
		FROM audit_log` + clause + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	rows, err := db.Query(q, append(args, limit, max(f.Offset, 0))...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit rows: %w", err)
	}
	defer rows.Close()

	var out []AuditRow
	for rows.Next() {
		var r AuditRow
		var at string
		if err := rows.Scan(&r.ID, &at, &r.BookID, &r.Action, &r.Series, &r.Number, &r.StageBefore, &r.StageAfter, &r.Changes); err != nil {
			return nil, 0, fmt.Errorf("scan audit row: %w", err)
		}
		r.At, _ = time.Parse(time.RFC3339, at)
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// PruneAudit deletes rows older than cutoff and returns how many went.
func (db *DB) PruneAudit(cutoff time.Time) (int64, error) {
	if err := db.ensureAuditTable(); err != nil {
		return 0, err
	}
	res, err := db.Exec(`DELETE FROM audit_log WHERE at < ?`, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("prune audit log: %w", err)
	}
	return res.RowsAffected()
}
