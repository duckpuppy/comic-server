// Package auth is comic-server's built-in login (tracker #21): one admin
// password hashed with argon2id, cookie sessions and API tokens, all kept in
// config.db. Auth is OFF until a password is set, so an upgrade can never
// lock anyone out.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// MinPasswordLength is the shortest password accepted.
const MinPasswordLength = 12

// maxPasswordLength stops a huge body from costing a huge hash.
const maxPasswordLength = 256

const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// ValidatePassword checks length only; the owner picks the rest.
func ValidatePassword(pw string) error {
	if len(pw) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	if len(pw) > maxPasswordLength {
		return fmt.Errorf("password must be at most %d characters", maxPasswordLength)
	}
	return nil
}

// HashPassword returns an argon2id hash in the standard PHC string form.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read random salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

var errBadHash = errors.New("unrecognised password hash")

// VerifyPassword reports whether pw matches a hash made by HashPassword.
// A malformed hash never matches.
func VerifyPassword(hash, pw string) bool {
	if len(pw) > maxPasswordLength {
		return false
	}
	salt, key, m, t, p, err := parseHash(hash)
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1
}

func parseHash(hash string) (salt, key []byte, m, t uint32, p uint8, err error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, errBadHash
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return nil, nil, 0, 0, 0, errBadHash
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return nil, nil, 0, 0, 0, errBadHash
	}
	// Refuse absurd parameters from a corrupted row rather than allocate them.
	if m == 0 || m > 1024*1024 || t == 0 || t > 20 || p == 0 {
		return nil, nil, 0, 0, 0, errBadHash
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil || len(salt) == 0 {
		return nil, nil, 0, 0, 0, errBadHash
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil || len(key) == 0 {
		return nil, nil, 0, 0, 0, errBadHash
	}
	return salt, key, m, t, p, nil
}
