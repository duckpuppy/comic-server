package configdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// OIDCSettings is the Authelia (or any OpenID Connect provider) sign-in
// configuration (tracker #22). One row. ClientSecret lives here in plain text
// like the ComicVine key does in config.yaml; the API never returns it.
type OIDCSettings struct {
	Enabled       bool
	Issuer        string
	ClientID      string
	ClientSecret  string
	PublicURL     string // the address browsers use for comic-server, no trailing slash
	AllowedGroups []string
	AllowedUsers  []string
}

func (db *DB) ensureOIDCTable() error {
	if err := db.ensureAuthTables(); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS auth_oidc (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		enabled INTEGER NOT NULL DEFAULT 0,
		issuer TEXT NOT NULL DEFAULT '',
		client_id TEXT NOT NULL DEFAULT '',
		client_secret TEXT NOT NULL DEFAULT '',
		public_url TEXT NOT NULL DEFAULT '',
		allowed_groups TEXT NOT NULL DEFAULT '[]',
		allowed_users TEXT NOT NULL DEFAULT '[]'
	)`)
	if err != nil {
		return fmt.Errorf("create auth_oidc: %w", err)
	}
	return nil
}

// GetOIDCSettings returns the saved settings, or the zero value (off).
func (db *DB) GetOIDCSettings() (OIDCSettings, error) {
	if err := db.ensureOIDCTable(); err != nil {
		return OIDCSettings{}, err
	}
	var s OIDCSettings
	var en int
	var groups, users string
	err := db.QueryRow(`SELECT enabled, issuer, client_id, client_secret, public_url, allowed_groups, allowed_users
		FROM auth_oidc WHERE id = 1`).Scan(&en, &s.Issuer, &s.ClientID, &s.ClientSecret, &s.PublicURL, &groups, &users)
	if errors.Is(err, sql.ErrNoRows) {
		return OIDCSettings{}, nil
	}
	if err != nil {
		return OIDCSettings{}, fmt.Errorf("read oidc settings: %w", err)
	}
	s.Enabled = en != 0
	_ = json.Unmarshal([]byte(groups), &s.AllowedGroups)
	_ = json.Unmarshal([]byte(users), &s.AllowedUsers)
	return s, nil
}

// SaveOIDCSettings replaces the saved settings.
func (db *DB) SaveOIDCSettings(s OIDCSettings) error {
	if err := db.ensureOIDCTable(); err != nil {
		return err
	}
	groups, _ := json.Marshal(nonNil(s.AllowedGroups))
	users, _ := json.Marshal(nonNil(s.AllowedUsers))
	en := 0
	if s.Enabled {
		en = 1
	}
	_, err := db.Exec(`INSERT INTO auth_oidc (id, enabled, issuer, client_id, client_secret, public_url, allowed_groups, allowed_users)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET enabled = excluded.enabled, issuer = excluded.issuer, client_id = excluded.client_id,
			client_secret = excluded.client_secret, public_url = excluded.public_url,
			allowed_groups = excluded.allowed_groups, allowed_users = excluded.allowed_users`,
		en, s.Issuer, s.ClientID, s.ClientSecret, s.PublicURL, string(groups), string(users))
	if err != nil {
		return fmt.Errorf("save oidc settings: %w", err)
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
