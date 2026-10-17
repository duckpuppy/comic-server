package configdb

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Auth storage (tracker #21): one admin credential row, login sessions and
// API tokens. Tokens and session ids are stored only as SHA-256 hex, so a
// copy of config.db (for example a backup) does not hold a usable secret.
// Tables are created on first use, so no schema version bump.

// AuthToken is one API token's record. The secret is never stored.
type AuthToken struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// AuthSession is a stored login session.
type AuthSession struct {
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (db *DB) ensureAuthTables() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS auth_credentials (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			password_hash TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS auth_sessions (
			token_hash TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS auth_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			last_used_at TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("create auth tables: %w", err)
		}
	}
	return nil
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// AuthPasswordHash returns the stored hash, or "" when auth is not enabled.
func (db *DB) AuthPasswordHash() (string, error) {
	if err := db.ensureAuthTables(); err != nil {
		return "", err
	}
	var h string
	err := db.QueryRow(`SELECT password_hash FROM auth_credentials WHERE id = 1`).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read auth credential: %w", err)
	}
	return h, nil
}

// SetAuthPasswordHash stores the hash (this turns auth on) and ends every
// existing session, so a changed password signs out anyone holding an old one.
func (db *DB) SetAuthPasswordHash(hash string) error {
	if err := db.ensureAuthTables(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO auth_credentials (id, password_hash, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET password_hash = excluded.password_hash, updated_at = excluded.updated_at`,
		hash, ts(time.Now())); err != nil {
		return fmt.Errorf("store auth credential: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM auth_sessions`); err != nil {
		return fmt.Errorf("clear sessions: %w", err)
	}
	return tx.Commit()
}

// DisableAuth removes the credential and every session. API tokens are kept
// (they do nothing while auth is off, and survive a later re-enable).
func (db *DB) DisableAuth() error {
	if err := db.ensureAuthTables(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM auth_credentials`); err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM auth_sessions`); err != nil {
		return fmt.Errorf("clear sessions: %w", err)
	}
	return tx.Commit()
}

// CreateAuthSession stores a session keyed by the hash of its cookie value.
func (db *DB) CreateAuthSession(tokenHash string, expires time.Time, ip, userAgent string) error {
	if err := db.ensureAuthTables(); err != nil {
		return err
	}
	if len(userAgent) > 200 {
		userAgent = userAgent[:200]
	}
	_, err := db.Exec(`INSERT INTO auth_sessions (token_hash, created_at, expires_at, ip, user_agent) VALUES (?, ?, ?, ?, ?)`,
		tokenHash, ts(time.Now()), ts(expires), ip, userAgent)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// AuthSessionByHash returns the session, or nil if missing or expired.
func (db *DB) AuthSessionByHash(tokenHash string) (*AuthSession, error) {
	if err := db.ensureAuthTables(); err != nil {
		return nil, err
	}
	var created, expires string
	err := db.QueryRow(`SELECT created_at, expires_at FROM auth_sessions WHERE token_hash = ?`, tokenHash).Scan(&created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}
	s := &AuthSession{CreatedAt: parseTS(created), ExpiresAt: parseTS(expires)}
	if !s.ExpiresAt.After(time.Now()) {
		return nil, nil
	}
	return s, nil
}

// DeleteAuthSession ends one session.
func (db *DB) DeleteAuthSession(tokenHash string) error {
	if err := db.ensureAuthTables(); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM auth_sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// PruneAuthSessions drops expired sessions.
func (db *DB) PruneAuthSessions() error {
	if err := db.ensureAuthTables(); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM auth_sessions WHERE expires_at <= ?`, ts(time.Now()))
	return err
}

// CreateAuthToken stores a new API token's hash.
func (db *DB) CreateAuthToken(name, tokenHash string) (AuthToken, error) {
	if err := db.ensureAuthTables(); err != nil {
		return AuthToken{}, err
	}
	now := time.Now()
	res, err := db.Exec(`INSERT INTO auth_tokens (name, token_hash, created_at) VALUES (?, ?, ?)`, name, tokenHash, ts(now))
	if err != nil {
		return AuthToken{}, fmt.Errorf("create token: %w", err)
	}
	id, _ := res.LastInsertId()
	return AuthToken{ID: id, Name: name, CreatedAt: now.UTC()}, nil
}

// ListAuthTokens returns every token record, oldest first.
func (db *DB) ListAuthTokens() ([]AuthToken, error) {
	if err := db.ensureAuthTables(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, name, created_at, last_used_at FROM auth_tokens ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer rows.Close()
	out := []AuthToken{}
	for rows.Next() {
		var t AuthToken
		var created, used string
		if err := rows.Scan(&t.ID, &t.Name, &created, &used); err != nil {
			return nil, err
		}
		t.CreatedAt = parseTS(created)
		if used != "" {
			u := parseTS(used)
			t.LastUsedAt = &u
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AuthTokenIDByHash returns the token's id, or 0 when no token matches.
func (db *DB) AuthTokenIDByHash(tokenHash string) (int64, error) {
	if err := db.ensureAuthTables(); err != nil {
		return 0, err
	}
	var id int64
	err := db.QueryRow(`SELECT id FROM auth_tokens WHERE token_hash = ?`, tokenHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// TouchAuthToken records a use.
func (db *DB) TouchAuthToken(id int64) {
	_, _ = db.Exec(`UPDATE auth_tokens SET last_used_at = ? WHERE id = ?`, ts(time.Now()), id)
}

// DeleteAuthToken revokes a token. It reports whether one was removed.
func (db *DB) DeleteAuthToken(id int64) (bool, error) {
	if err := db.ensureAuthTables(); err != nil {
		return false, err
	}
	res, err := db.Exec(`DELETE FROM auth_tokens WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
