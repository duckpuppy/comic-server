package configdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Trusted proxy list saved from Settings > Security (tracker #24). One row.
// No row means "never saved": config.yaml / the env var apply instead. A
// saved empty list is a real choice (trust no proxy) and wins over them.

func (db *DB) ensureProxyTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS auth_trusted_proxies (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		proxies TEXT NOT NULL DEFAULT '[]'
	)`)
	if err != nil {
		return fmt.Errorf("create auth_trusted_proxies: %w", err)
	}
	return nil
}

// GetTrustedProxies returns the saved list and whether one was ever saved.
func (db *DB) GetTrustedProxies() (list []string, saved bool, err error) {
	if err := db.ensureProxyTable(); err != nil {
		return nil, false, err
	}
	var raw string
	err = db.QueryRow(`SELECT proxies FROM auth_trusted_proxies WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read trusted proxies: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, false, fmt.Errorf("decode trusted proxies: %w", err)
	}
	return list, true, nil
}

// SaveTrustedProxies stores the list (which may be empty).
func (db *DB) SaveTrustedProxies(list []string) error {
	if err := db.ensureProxyTable(); err != nil {
		return err
	}
	if list == nil {
		list = []string{}
	}
	raw, _ := json.Marshal(list)
	_, err := db.Exec(`INSERT INTO auth_trusted_proxies (id, proxies) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET proxies = excluded.proxies`, string(raw))
	if err != nil {
		return fmt.Errorf("save trusted proxies: %w", err)
	}
	return nil
}

// ClearTrustedProxies forgets the saved list, so config.yaml / the env var apply again.
func (db *DB) ClearTrustedProxies() error {
	if err := db.ensureProxyTable(); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM auth_trusted_proxies`)
	return err
}
