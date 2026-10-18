package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/ratelimit"
)

const (
	// CookieName is the session cookie.
	CookieName = "comic_server_session"
	// SessionLifetime is how long a login lasts.
	SessionLifetime = 30 * 24 * time.Hour
	// TokenPrefix marks an API token so it is easy to spot in a leak scan.
	TokenPrefix = "cs_"

	loginAttempts = 10
	loginWindow   = 15 * time.Minute
	enabledTTL    = 5 * time.Second
)

// ErrRateLimited is returned when a client IP has tried to log in too often.
var ErrRateLimited = errors.New("too many login attempts, try again later")

// ErrBadCredentials is returned for a wrong password.
var ErrBadCredentials = errors.New("wrong password")

// Manager holds the login rules. It is safe for concurrent use.
type Manager struct {
	db      *configdb.DB
	proxies []netip.Prefix
	limiter *ratelimit.IPLimiter
	oidc    *oidcState

	mu          sync.Mutex
	enabled     bool
	enabledAt   time.Time
	enabledSeen bool
}

// New builds a Manager. trustedProxies are IPs or CIDRs whose forwarding
// headers are believed; an empty list trusts none.
func New(db *configdb.DB, trustedProxies []string) (*Manager, error) {
	prefixes, err := config.ParseTrustedProxies(trustedProxies)
	if err != nil {
		return nil, err
	}
	return &Manager{
		db:      db,
		proxies: prefixes,
		limiter: ratelimit.NewIPLimiter(loginAttempts, loginWindow),
		oidc:    &oidcState{pending: map[string]pendingLogin{}},
	}, nil
}

// Close stops the limiter's cleanup goroutine.
func (m *Manager) Close() { m.limiter.Stop() }

// Enabled reports whether a password is set. The answer is cached for a few
// seconds so a CLI change (set-password, disable) is seen without a restart.
func (m *Manager) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabledSeen && time.Since(m.enabledAt) < enabledTTL {
		return m.enabled
	}
	h, err := m.db.AuthPasswordHash()
	if err != nil {
		// Fail closed once auth has been seen on; open only if it was never on.
		return m.enabledSeen && m.enabled
	}
	m.enabled, m.enabledAt, m.enabledSeen = h != "", time.Now(), true
	return m.enabled
}

func (m *Manager) setEnabled(v bool) {
	m.mu.Lock()
	m.enabled, m.enabledAt, m.enabledSeen = v, time.Now(), true
	m.mu.Unlock()
}

func (m *Manager) fromTrustedProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return m.trusted(a.Unmap())
}

func (m *Manager) trusted(a netip.Addr) bool {
	for _, p := range m.proxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ClientIP is the caller's address. Forwarding headers count only when the
// direct peer is a trusted proxy; X-Forwarded-For is read from the right,
// skipping trusted hops, so a client cannot forge its own entry.
func (m *Manager) ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !m.fromTrustedProxy(r) {
		return host
	}
	if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
		if a, err := netip.ParseAddr(cf); err == nil {
			return a.Unmap().String()
		}
	}
	parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			break
		}
		a = a.Unmap()
		if !m.trusted(a) {
			return a.String()
		}
	}
	return host
}

// IsSecure reports whether the browser reached us over HTTPS.
func (m *Manager) IsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return m.fromTrustedProxy(r) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// SameOrigin is the cross-site check for cookie-authenticated writes and the
// WebSocket upgrade. A request with no Origin and no Sec-Fetch-Site is not
// from a browser page and passes; the cookie is not attached to such calls
// by a cross-site page.
func (m *Manager) SameOrigin(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		if strings.EqualFold(u.Host, r.Host) {
			return true
		}
		if m.fromTrustedProxy(r) {
			if fh := r.Header.Get("X-Forwarded-Host"); fh != "" && strings.EqualFold(u.Host, strings.TrimSpace(strings.Split(fh, ",")[0])) {
				return true
			}
		}
		return false
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	return false
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func randomToken(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// Login checks the password and starts a session. It returns the cookie value.
func (m *Manager) Login(ip, userAgent, password string) (string, error) {
	if !m.limiter.Allow(ip) {
		return "", ErrRateLimited
	}
	hash, err := m.db.AuthPasswordHash()
	if err != nil {
		return "", err
	}
	if hash == "" {
		return "", errors.New("login is not enabled")
	}
	if !VerifyPassword(hash, password) {
		return "", ErrBadCredentials
	}
	return m.startSession(ip, userAgent)
}

func (m *Manager) startSession(ip, userAgent string) (string, error) {
	tok, err := randomToken("")
	if err != nil {
		return "", err
	}
	if err := m.db.CreateAuthSession(hashToken(tok), time.Now().Add(SessionLifetime), ip, userAgent); err != nil {
		return "", err
	}
	_ = m.db.PruneAuthSessions()
	return tok, nil
}

// SetPassword stores a new password. That signs out every session, then a
// fresh one is started and returned so the caller stays logged in.
func (m *Manager) SetPassword(ip, userAgent, newPassword string) (string, error) {
	if err := ValidatePassword(newPassword); err != nil {
		return "", err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return "", err
	}
	if err := m.db.SetAuthPasswordHash(hash); err != nil {
		return "", err
	}
	m.setEnabled(true)
	return m.startSession(ip, userAgent)
}

// CheckPassword verifies a password without starting a session (used to
// confirm a password change). It shares the login rate limit.
func (m *Manager) CheckPassword(ip, password string) error {
	if !m.limiter.Allow(ip) {
		return ErrRateLimited
	}
	hash, err := m.db.AuthPasswordHash()
	if err != nil {
		return err
	}
	if !VerifyPassword(hash, password) {
		return ErrBadCredentials
	}
	return nil
}

// Logout ends the session behind a cookie value.
func (m *Manager) Logout(cookieValue string) {
	if cookieValue != "" {
		_ = m.db.DeleteAuthSession(hashToken(cookieValue))
	}
}

// Kind says how a request was authenticated.
type Kind int

const (
	None Kind = iota
	Session
	Token
)

// Authenticate reads a bearer token or the session cookie.
func (m *Manager) Authenticate(r *http.Request) Kind {
	if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		tok := strings.TrimSpace(h[7:])
		if strings.HasPrefix(tok, TokenPrefix) {
			if id, err := m.db.AuthTokenIDByHash(hashToken(tok)); err == nil && id != 0 {
				m.db.TouchAuthToken(id)
				return Token
			}
		}
		return None
	}
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		if s, err := m.db.AuthSessionByHash(hashToken(c.Value)); err == nil && s != nil {
			return Session
		}
	}
	return None
}

// NewToken creates an API token. The value is returned once and never stored.
func (m *Manager) NewToken(name string) (string, configdb.AuthToken, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return "", configdb.AuthToken{}, errors.New("token name must be 1 to 80 characters")
	}
	tok, err := randomToken(TokenPrefix)
	if err != nil {
		return "", configdb.AuthToken{}, err
	}
	rec, err := m.db.CreateAuthToken(name, hashToken(tok))
	return tok, rec, err
}

// Cookie builds the session cookie for a request.
func (m *Manager) Cookie(r *http.Request, value string) *http.Cookie {
	c := &http.Cookie{
		Name: CookieName, Value: value, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: m.IsSecure(r),
	}
	if value == "" {
		c.MaxAge = -1
	} else {
		c.MaxAge = int(SessionLifetime.Seconds())
	}
	return c
}
