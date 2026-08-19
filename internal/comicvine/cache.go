package comicvine

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const cacheSchemaVersion = 2

// Cache provides SQLite-backed storage for ComicVine API responses.
type Cache struct {
	db *sql.DB
}

// OpenCache opens or creates the ComicVine cache database.
func OpenCache(path string) (*Cache, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open cache database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping cache database: %w", err)
	}

	c := &Cache{db: db}
	if err := c.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize cache schema: %w", err)
	}
	return c, nil
}

func (c *Cache) initSchema() error {
	var version int
	if err := c.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version >= cacheSchemaVersion {
		return nil
	}

	_, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS cv_volumes (
			cv_id          INTEGER PRIMARY KEY,
			name           TEXT NOT NULL,
			publisher      TEXT,
			start_year     TEXT,
			issue_count    INTEGER DEFAULT 0,
			site_url       TEXT,
			sync_status    TEXT NOT NULL DEFAULT 'pending',
			synced_at      TEXT,
			issues_synced  INTEGER DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS cv_issues (
			cv_id          INTEGER PRIMARY KEY,
			volume_cv_id   INTEGER NOT NULL REFERENCES cv_volumes(cv_id),
			number         TEXT NOT NULL,
			name           TEXT,
			cover_date     TEXT,
			store_date     TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_cv_issues_volume ON cv_issues(volume_cv_id);
		CREATE INDEX IF NOT EXISTS idx_cv_issues_number ON cv_issues(volume_cv_id, number);

		CREATE TABLE IF NOT EXISTS cv_sync_state (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cv_issue_details (
			cv_id        INTEGER PRIMARY KEY,
			details_json TEXT NOT NULL,
			fetched_at   TEXT
		);
	`)
	if err != nil {
		return fmt.Errorf("create tables: %w", err)
	}

	_, err = c.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", cacheSchemaVersion))
	return err
}

// Close closes the cache database.
func (c *Cache) Close() error {
	return c.db.Close()
}

// UpsertVolume inserts or updates a cached volume.
func (c *Cache) UpsertVolume(v *CachedVolume) error {
	_, err := c.db.Exec(`
		INSERT INTO cv_volumes (cv_id, name, publisher, start_year, issue_count, site_url, sync_status, synced_at, issues_synced)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cv_id) DO UPDATE SET
			name=excluded.name, publisher=excluded.publisher, start_year=excluded.start_year,
			issue_count=excluded.issue_count, site_url=excluded.site_url,
			sync_status=excluded.sync_status, synced_at=excluded.synced_at,
			issues_synced=excluded.issues_synced`,
		v.CVID, v.Name, v.Publisher, v.StartYear, v.IssueCount, v.SiteURL,
		string(v.SyncStatus), v.SyncedAt.Format(time.RFC3339), v.IssuesSynced)
	return err
}

// UpsertIssues inserts or updates a batch of cached issues in a single transaction.
func (c *Cache) UpsertIssues(issues []CachedIssue) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO cv_issues (cv_id, volume_cv_id, number, name, cover_date, store_date)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(cv_id) DO UPDATE SET
			number=excluded.number, name=excluded.name,
			cover_date=excluded.cover_date, store_date=excluded.store_date`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, iss := range issues {
		if _, err := stmt.Exec(iss.CVID, iss.VolumeCVID, iss.Number, iss.Name, iss.CoverDate, iss.StoreDate); err != nil {
			return fmt.Errorf("upsert issue %d: %w", iss.CVID, err)
		}
	}
	return tx.Commit()
}

// GetVolume returns a cached volume, or nil if not found.
func (c *Cache) GetVolume(cvID int) (*CachedVolume, error) {
	row := c.db.QueryRow(`SELECT cv_id, COALESCE(name,''), COALESCE(publisher,''),
		COALESCE(start_year,''), COALESCE(issue_count,0), COALESCE(site_url,''),
		COALESCE(sync_status,'pending'), synced_at, COALESCE(issues_synced,0)
		FROM cv_volumes WHERE cv_id = ?`, cvID)

	var v CachedVolume
	var status string
	var syncedAt sql.NullString
	err := row.Scan(&v.CVID, &v.Name, &v.Publisher, &v.StartYear, &v.IssueCount,
		&v.SiteURL, &status, &syncedAt, &v.IssuesSynced)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.SyncStatus = SyncStatus(status)
	if syncedAt.Valid {
		v.SyncedAt, _ = time.Parse(time.RFC3339, syncedAt.String)
	}
	return &v, nil
}

// GetIssuesForVolume returns all cached issues for a volume.
func (c *Cache) GetIssuesForVolume(volumeCVID int) ([]CachedIssue, error) {
	rows, err := c.db.Query(`SELECT cv_id, volume_cv_id, number, name, cover_date, store_date
		FROM cv_issues WHERE volume_cv_id = ? ORDER BY cv_id`, volumeCVID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []CachedIssue
	for rows.Next() {
		var iss CachedIssue
		if err := rows.Scan(&iss.CVID, &iss.VolumeCVID, &iss.Number, &iss.Name, &iss.CoverDate, &iss.StoreDate); err != nil {
			return nil, err
		}
		issues = append(issues, iss)
	}
	return issues, rows.Err()
}

// PendingVolumeIDs returns ComicVine volume IDs that haven't been synced yet,
// ordered by priority (most-owned-issues first). Takes a map of cvVolumeID → count
// of books owned in the library for prioritization.
func (c *Cache) PendingVolumeIDs(owned map[int]int) ([]int, error) {
	rows, err := c.db.Query(`SELECT cv_id FROM cv_volumes WHERE sync_status = 'pending' OR issues_synced = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by owned count descending (most-owned volumes first)
	sortByOwned(ids, owned)
	return ids, nil
}

// EnsureVolumesExist creates pending records for volume IDs not yet in the cache.
func (c *Cache) EnsureVolumesExist(cvIDs []int) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO cv_volumes (cv_id, name, sync_status) VALUES (?, '', 'pending')`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range cvIDs {
		if _, err := stmt.Exec(id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SyncStats returns counts of synced, pending, and failed volumes.
func (c *Cache) SyncStats() (synced, pending, failed int, err error) {
	row := c.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN sync_status='synced' AND issues_synced=1 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN sync_status='pending' OR (sync_status='synced' AND issues_synced=0) THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN sync_status='failed' THEN 1 ELSE 0 END), 0)
		FROM cv_volumes`)
	err = row.Scan(&synced, &pending, &failed)
	return
}

// SetSyncState stores a key-value pair for sync state tracking (e.g. backoff timers).
func (c *Cache) SetSyncState(key, value string) error {
	_, err := c.db.Exec(`INSERT INTO cv_sync_state (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// GetSyncState retrieves a sync state value.
func (c *Cache) GetSyncState(key string) (string, error) {
	var val string
	err := c.db.QueryRow(`SELECT value FROM cv_sync_state WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

// UpsertIssueDetail stores the full metadata for an issue, fetched lazily during scraping.
func (c *Cache) UpsertIssueDetail(detail *IssueDetail) error {
	data, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal issue detail: %w", err)
	}
	_, err = c.db.Exec(`
		INSERT INTO cv_issue_details (cv_id, details_json, fetched_at)
		VALUES (?, ?, ?)
		ON CONFLICT(cv_id) DO UPDATE SET
			details_json=excluded.details_json, fetched_at=excluded.fetched_at`,
		detail.ID, string(data), time.Now().Format(time.RFC3339))
	return err
}

// GetIssueDetail returns cached full issue metadata, or nil if not cached.
func (c *Cache) GetIssueDetail(cvID int) (*IssueDetail, error) {
	var data string
	err := c.db.QueryRow(`SELECT details_json FROM cv_issue_details WHERE cv_id = ?`, cvID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var detail IssueDetail
	if err := json.Unmarshal([]byte(data), &detail); err != nil {
		return nil, fmt.Errorf("unmarshal issue detail: %w", err)
	}
	return &detail, nil
}

// sortByOwned sorts volume IDs by how many books are owned, descending.
func sortByOwned(ids []int, owned map[int]int) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && owned[ids[j]] > owned[ids[j-1]]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}
