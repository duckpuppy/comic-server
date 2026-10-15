package configdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ScrapePublisherFilter is the stored allow/block publisher lists for
// automatic ComicVine matching (comic-server-pl94, moved from config.yaml
// by comic-server-2w44's follow-up). Created lazily with CREATE TABLE IF
// NOT EXISTS, so there is no schema bump.
type ScrapePublisherFilter struct {
	Allow []string
	Block []string
}

func (db *DB) ensureScrapePublisherFilterTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS scrape_publisher_filter (
		id    INTEGER PRIMARY KEY CHECK (id = 1),
		allow TEXT NOT NULL DEFAULT '[]',
		block TEXT NOT NULL DEFAULT '[]'
	)`)
	if err != nil {
		return fmt.Errorf("create scrape_publisher_filter table: %w", err)
	}
	return nil
}

// GetScrapePublisherFilter returns the stored lists, or nil when none were
// ever saved (callers then fall back to config.yaml / env).
func (db *DB) GetScrapePublisherFilter() (*ScrapePublisherFilter, error) {
	if err := db.ensureScrapePublisherFilterTable(); err != nil {
		return nil, err
	}
	var allowJSON, blockJSON string
	err := db.QueryRow(`SELECT allow, block FROM scrape_publisher_filter WHERE id = 1`).Scan(&allowJSON, &blockJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get scrape publisher filter: %w", err)
	}
	f := &ScrapePublisherFilter{}
	if err := json.Unmarshal([]byte(allowJSON), &f.Allow); err != nil {
		return nil, fmt.Errorf("unmarshal scrape publisher allow: %w", err)
	}
	if err := json.Unmarshal([]byte(blockJSON), &f.Block); err != nil {
		return nil, fmt.Errorf("unmarshal scrape publisher block: %w", err)
	}
	return f, nil
}

// UpsertScrapePublisherFilter replaces the stored lists wholesale. Saving
// two empty lists is meaningful: it overrides config.yaml with "no filter".
func (db *DB) UpsertScrapePublisherFilter(f ScrapePublisherFilter) error {
	if err := db.ensureScrapePublisherFilterTable(); err != nil {
		return err
	}
	if f.Allow == nil {
		f.Allow = []string{}
	}
	if f.Block == nil {
		f.Block = []string{}
	}
	allowJSON, err := json.Marshal(f.Allow)
	if err != nil {
		return fmt.Errorf("marshal scrape publisher allow: %w", err)
	}
	blockJSON, err := json.Marshal(f.Block)
	if err != nil {
		return fmt.Errorf("marshal scrape publisher block: %w", err)
	}
	_, err = db.Exec(`
		INSERT INTO scrape_publisher_filter (id, allow, block) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET allow = excluded.allow, block = excluded.block
	`, string(allowJSON), string(blockJSON))
	if err != nil {
		return fmt.Errorf("upsert scrape publisher filter: %w", err)
	}
	return nil
}
