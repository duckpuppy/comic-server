package configdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/duckpuppy/comic-server/internal/config"
)

// GetScanInfo returns the stored ScanInfo config, or nil if config.db has
// never been given one - callers should fall back to config.yaml's
// Server.ScanInfo in that case (see api.Server.effectiveScanInfo).
func (db *DB) GetScanInfo() (*config.ScanInfoConfig, error) {
	var enabled bool
	var scannersJSON, blacklistJSON, prefix, unknown string
	err := db.QueryRow(`SELECT enabled, scanners, blacklist, prefix, unknown FROM scan_info WHERE id = 1`).
		Scan(&enabled, &scannersJSON, &blacklistJSON, &prefix, &unknown)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get scan info: %w", err)
	}

	var scanners, blacklist []string
	if err := json.Unmarshal([]byte(scannersJSON), &scanners); err != nil {
		return nil, fmt.Errorf("unmarshal scan info scanners: %w", err)
	}
	if err := json.Unmarshal([]byte(blacklistJSON), &blacklist); err != nil {
		return nil, fmt.Errorf("unmarshal scan info blacklist: %w", err)
	}

	return &config.ScanInfoConfig{
		Enabled:   enabled,
		Scanners:  scanners,
		Blacklist: blacklist,
		Prefix:    prefix,
		Unknown:   unknown,
	}, nil
}

// UpsertScanInfo replaces the stored ScanInfo config wholesale - there's
// no separate add/remove-one-entry API, matching how the web UI edits the
// Scanners/Blacklist lists locally and saves the whole set at once. A nil
// Scanners or Blacklist is stored the same as an empty slice.
func (db *DB) UpsertScanInfo(cfg config.ScanInfoConfig) error {
	scanners := cfg.Scanners
	if scanners == nil {
		scanners = []string{}
	}
	blacklist := cfg.Blacklist
	if blacklist == nil {
		blacklist = []string{}
	}

	scannersJSON, err := json.Marshal(scanners)
	if err != nil {
		return fmt.Errorf("marshal scan info scanners: %w", err)
	}
	blacklistJSON, err := json.Marshal(blacklist)
	if err != nil {
		return fmt.Errorf("marshal scan info blacklist: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO scan_info (id, enabled, scanners, blacklist, prefix, unknown)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			enabled = excluded.enabled,
			scanners = excluded.scanners,
			blacklist = excluded.blacklist,
			prefix = excluded.prefix,
			unknown = excluded.unknown
	`, cfg.Enabled, string(scannersJSON), string(blacklistJSON), cfg.Prefix, cfg.Unknown)
	if err != nil {
		return fmt.Errorf("upsert scan info: %w", err)
	}
	return nil
}
