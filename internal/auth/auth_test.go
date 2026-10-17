package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/configdb"
)

func newManager(t *testing.T, proxies ...string) *Manager {
	t.Helper()
	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	m, err := New(db, proxies)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("hash = %q", h)
	}
	if !VerifyPassword(h, "correct horse battery") {
		t.Error("right password rejected")
	}
	if VerifyPassword(h, "wrong horse battery!") {
		t.Error("wrong password accepted")
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=1,t=1,p=1$!!$!!", "$argon2id$v=19$m=99999999,t=1,p=1$AAAA$AAAA"} {
		if VerifyPassword(bad, "x") {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Error("short password accepted")
	}
	if ValidatePassword(strings.Repeat("a", 300)) == nil {
		t.Error("huge password accepted")
	}
	if err := ValidatePassword(strings.Repeat("a", MinPasswordLength)); err != nil {
		t.Error(err)
	}
}

func TestLoginSessionAndLogout(t *testing.T) {
	m := newManager(t)
	if m.Enabled() {
		t.Fatal("enabled before a password is set")
	}
	cookie, err := m.SetPassword("1.2.3.4", "ua", "a long enough password")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Enabled() {
		t.Fatal("not enabled after set")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	if m.Authenticate(req) != None {
		t.Error("anonymous request authenticated")
	}
	req.AddCookie(&http.Cookie{Name: CookieName, Value: cookie})
	if m.Authenticate(req) != Session {
		t.Error("session cookie rejected")
	}
	if _, err := m.Login("1.2.3.4", "ua", "nope nope nope nope"); err != ErrBadCredentials {
		t.Errorf("wrong password err = %v", err)
	}
	c2, err := m.Login("1.2.3.4", "ua", "a long enough password")
	if err != nil {
		t.Fatal(err)
	}
	m.Logout(c2)
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.AddCookie(&http.Cookie{Name: CookieName, Value: c2})
	if m.Authenticate(r2) != None {
		t.Error("logged-out cookie still works")
	}
	// A new password ends every old session.
	if _, err := m.SetPassword("1.2.3.4", "ua", "another long password"); err != nil {
		t.Fatal(err)
	}
	if m.Authenticate(req) != None {
		t.Error("old session survived a password change")
	}
}

func TestLoginRateLimit(t *testing.T) {
	m := newManager(t)
	if _, err := m.SetPassword("9.9.9.9", "", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	var last error
	for i := 0; i < loginAttempts+2; i++ {
		_, last = m.Login("5.5.5.5", "", "wrong wrong wrong")
	}
	if last != ErrRateLimited {
		t.Fatalf("err after many tries = %v", last)
	}
	// Another address is not affected.
	if _, err := m.Login("6.6.6.6", "", "a long enough password"); err != nil {
		t.Errorf("other IP blocked: %v", err)
	}
}

func TestAPITokens(t *testing.T) {
	m := newManager(t)
	tok, rec, err := m.NewToken("prometheus")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, TokenPrefix) {
		t.Fatalf("token = %q", tok)
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	if m.Authenticate(req) != Token {
		t.Error("token rejected")
	}
	req.Header.Set("Authorization", "Bearer "+tok+"x")
	if m.Authenticate(req) != None {
		t.Error("altered token accepted")
	}
	if ok, err := m.db.DeleteAuthToken(rec.ID); err != nil || !ok {
		t.Fatalf("revoke: %v %v", ok, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if m.Authenticate(req) != None {
		t.Error("revoked token accepted")
	}
	if _, _, err := m.NewToken("  "); err == nil {
		t.Error("empty name accepted")
	}
}

func TestClientIPAndSecure(t *testing.T) {
	m := newManager(t, "10.0.0.0/8")
	mk := func(remote string, hdr map[string]string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		return r
	}
	// Untrusted peer: headers are ignored.
	r := mk("203.0.113.5:1234", map[string]string{"X-Forwarded-For": "1.1.1.1", "X-Forwarded-Proto": "https"})
	if ip := m.ClientIP(r); ip != "203.0.113.5" {
		t.Errorf("untrusted ip = %s", ip)
	}
	if m.IsSecure(r) {
		t.Error("untrusted proxy set Secure")
	}
	// Trusted peer: rightmost untrusted hop wins, a forged left entry does not.
	r = mk("10.1.1.1:99", map[string]string{"X-Forwarded-For": "6.6.6.6, 198.51.100.7, 10.2.2.2", "X-Forwarded-Proto": "https"})
	if ip := m.ClientIP(r); ip != "198.51.100.7" {
		t.Errorf("trusted ip = %s", ip)
	}
	if !m.IsSecure(r) {
		t.Error("trusted proxy https not honoured")
	}
	r = mk("10.1.1.1:99", map[string]string{"CF-Connecting-IP": "198.51.100.9"})
	if ip := m.ClientIP(r); ip != "198.51.100.9" {
		t.Errorf("cf ip = %s", ip)
	}
}

func TestSameOrigin(t *testing.T) {
	m := newManager(t, "10.0.0.1")
	mk := func(remote string, hdr map[string]string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "http://comic.example/api/x", nil)
		r.RemoteAddr = remote
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		return r
	}
	cases := []struct {
		name string
		r    *http.Request
		want bool
	}{
		{"no browser headers", mk("1.1.1.1:1", nil), true},
		{"same origin", mk("1.1.1.1:1", map[string]string{"Origin": "https://comic.example"}), true},
		{"other origin", mk("1.1.1.1:1", map[string]string{"Origin": "https://evil.example"}), false},
		{"null origin", mk("1.1.1.1:1", map[string]string{"Origin": "null"}), false},
		{"cross-site fetch metadata", mk("1.1.1.1:1", map[string]string{"Sec-Fetch-Site": "cross-site"}), false},
		{"forwarded host from untrusted peer", mk("1.1.1.1:1", map[string]string{"Origin": "https://front.example", "X-Forwarded-Host": "front.example"}), false},
		{"forwarded host from trusted proxy", mk("10.0.0.1:1", map[string]string{"Origin": "https://front.example", "X-Forwarded-Host": "front.example"}), true},
	}
	for _, c := range cases {
		if got := m.SameOrigin(c.r); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
