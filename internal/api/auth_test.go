package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/auth"
	"github.com/duckpuppy/comic-server/internal/auth/authtest"
	"github.com/duckpuppy/comic-server/internal/library"
)

func authServer(t *testing.T) *Server {
	t.Helper()
	s := newDuplicatesTestServer(t, t.TempDir(), []library.ComicBook{{ID: "b1", Series: "S", FilePath: "a.cbz"}})
	m, err := auth.New(s.configDB, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	s.auth = m
	s.mux = http.NewServeMux()
	s.registerRoutes()
	s.upgrader.CheckOrigin = func(r *http.Request) bool { return s.auth.SameOrigin(r) }
	return s
}

// do sends one request through the whole server. This test server has no
// sync manager, registry and so on, so a handler that is reached may panic;
// that is reported as 599 ("got past the gate") rather than failing the test.
func do(s *Server, method, target, body string, hdr map[string]string, cookie *http.Cookie) (w *httptest.ResponseRecorder) {
	w = httptest.NewRecorder()
	defer func() {
		if recover() != nil {
			w = httptest.NewRecorder()
			w.WriteHeader(599)
		}
	}()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.10:5555"
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	s.ServeHTTP(w, r)
	return w
}

func sessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.CookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

// registeredRoutes reads every path handed to the mux in this package, so a
// route added later is covered without touching this test.
func registeredRoutes(t *testing.T) []string {
	t.Helper()
	files, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`mux\.Handle(?:Func)?\("([^"]+)"`)
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			p := m[1]
			if strings.HasSuffix(p, "/") && p != "/" {
				p += "x"
			}
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	if len(out) < 50 {
		t.Fatalf("found only %d routes; the regexp is stale", len(out))
	}
	return out
}

func TestAuthOffLeavesEverythingOpen(t *testing.T) {
	s := authServer(t)
	if w := do(s, http.MethodGet, "/api/stats", "", nil, nil); w.Code == http.StatusUnauthorized {
		t.Fatalf("auth is off but /api/stats gave 401")
	}
	w := do(s, http.MethodGet, "/api/auth/status", "", nil, nil)
	if !strings.Contains(w.Body.String(), `"enabled":false`) || !strings.Contains(w.Body.String(), `"authenticated":true`) {
		t.Errorf("status = %s", w.Body.String())
	}
}

func TestEveryRouteNeedsLoginOnceEnabled(t *testing.T) {
	s := authServer(t)
	if _, err := s.auth.SetPassword("192.0.2.10", "", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	for _, p := range registeredRoutes(t) {
		w := do(s, http.MethodGet, p, "", nil, nil)
		switch {
		case !isProtectedPath(p):
			if w.Code == http.StatusUnauthorized {
				t.Errorf("%s: static route gave 401", p)
			}
		case openAPIPaths[p]:
			if w.Code == http.StatusUnauthorized {
				t.Errorf("%s: open route gave 401", p)
			}
		default:
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: anonymous GET gave %d, want 401", p, w.Code)
			}
		}
		// A write must not get through either.
		if isProtectedPath(p) && !openAPIPaths[p] {
			if w := do(s, http.MethodPost, p, "{}", nil, nil); w.Code != http.StatusUnauthorized {
				t.Errorf("%s: anonymous POST gave %d, want 401", p, w.Code)
			}
		}
	}
	// Dotted and doubled paths do not dodge the gate.
	for _, p := range []string{"/api/../api/stats", "/x/../api/stats", "//api/stats", "/api/./stats"} {
		if w := do(s, http.MethodGet, p, "", nil, nil); w.Code == http.StatusOK {
			t.Errorf("%s reached the handler without login", p)
		}
	}
}

func TestLoginFlowAndCookieFlags(t *testing.T) {
	s := authServer(t)
	// First password while auth is off.
	w := do(s, http.MethodPost, "/api/auth/password", `{"new_password":"a long enough password"}`, nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("set password: %d %s", w.Code, w.Body.String())
	}
	c := sessionCookie(w)
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie = %+v", c)
	}
	if c.Secure {
		t.Error("Secure set on plain HTTP")
	}
	if w := do(s, http.MethodGet, "/api/stats", "", nil, c); w.Code == http.StatusUnauthorized {
		t.Error("session not accepted")
	}
	// Wrong and right login.
	if w := do(s, http.MethodPost, "/api/auth/login", `{"password":"wrong wrong wrong"}`, nil, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong password gave %d", w.Code)
	}
	w = do(s, http.MethodPost, "/api/auth/login", `{"password":"a long enough password"}`, nil, nil)
	if w.Code != http.StatusOK || sessionCookie(w) == nil {
		t.Fatalf("login: %d", w.Code)
	}
	// Changing the password needs the current one.
	if w := do(s, http.MethodPost, "/api/auth/password", `{"current_password":"nope nope nope","new_password":"brand new password"}`, nil, c); w.Code != http.StatusForbidden {
		t.Errorf("change with wrong current gave %d", w.Code)
	}
	// Logout ends the session.
	do(s, http.MethodPost, "/api/auth/logout", "", nil, c)
	if w := do(s, http.MethodGet, "/api/stats", "", nil, c); w.Code != http.StatusUnauthorized {
		t.Errorf("after logout gave %d", w.Code)
	}
}

func TestCrossSiteWriteRefusedForCookieButNotToken(t *testing.T) {
	s := authServer(t)
	w := do(s, http.MethodPost, "/api/auth/password", `{"new_password":"a long enough password"}`, nil, nil)
	c := sessionCookie(w)
	evil := map[string]string{"Origin": "https://evil.example"}
	if w := do(s, http.MethodPost, "/api/library/browse", "{}", evil, c); w.Code != http.StatusForbidden {
		t.Errorf("cross-site cookie write gave %d, want 403", w.Code)
	}
	if w := do(s, http.MethodPost, "/api/library/browse", "{}", map[string]string{"Origin": "http://example.com"}, c); w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
		t.Errorf("same-origin cookie write gave %d", w.Code)
	}
	// A token is not a browser credential, so it has no cross-site check.
	tok, _, err := s.auth.NewToken("script")
	if err != nil {
		t.Fatal(err)
	}
	h := map[string]string{"Authorization": "Bearer " + tok, "Origin": "https://evil.example"}
	if w := do(s, http.MethodPost, "/api/library/browse", "{}", h, nil); w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
		t.Errorf("token write gave %d", w.Code)
	}
	if w := do(s, http.MethodGet, "/metrics", "", map[string]string{"Authorization": "Bearer " + tok}, nil); w.Code != http.StatusOK {
		t.Errorf("metrics with token gave %d", w.Code)
	}
}

func TestWebSocketOriginIsChecked(t *testing.T) {
	s := authServer(t)
	r := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	r.Header.Set("Origin", "https://evil.example")
	if s.upgrader.CheckOrigin(r) {
		t.Error("cross-site WebSocket origin allowed")
	}
	r.Header.Set("Origin", "http://example.com")
	if !s.upgrader.CheckOrigin(r) {
		t.Error("same-origin WebSocket refused")
	}
}

func TestPasswordTooShortAndTokenAPI(t *testing.T) {
	s := authServer(t)
	if w := do(s, http.MethodPost, "/api/auth/password", `{"new_password":"short"}`, nil, nil); w.Code != http.StatusBadRequest {
		t.Errorf("short password gave %d", w.Code)
	}
	w := do(s, http.MethodPost, "/api/auth/password", `{"new_password":"a long enough password"}`, nil, nil)
	c := sessionCookie(w)
	w = do(s, http.MethodPost, "/api/auth/tokens", `{"name":"prom"}`, nil, c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"token":"cs_`) {
		t.Fatalf("create token: %d %s", w.Code, w.Body.String())
	}
	w = do(s, http.MethodGet, "/api/auth/tokens", "", nil, c)
	if strings.Contains(w.Body.String(), "cs_") {
		t.Error("token list leaks a secret")
	}
	if w := do(s, http.MethodDelete, "/api/auth/tokens/1", "", nil, c); w.Code != http.StatusOK {
		t.Errorf("revoke gave %d", w.Code)
	}
}

func TestOIDCRoutesAndSettingsAPI(t *testing.T) {
	s := authServer(t)
	// Off: start sends the browser back with a reason, never an error page.
	w := do(s, http.MethodGet, "/auth/oidc/start", "", nil, nil)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "login_error=sso_off") {
		t.Errorf("start while off: %d %s", w.Code, w.Header().Get("Location"))
	}
	// A callback with no state/cookie fails cleanly.
	w = do(s, http.MethodGet, "/auth/oidc/callback?state=x&code=y", "", nil, nil)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "login_error=sso_failed") {
		t.Errorf("bad callback: %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := do(s, http.MethodGet, "/auth/oidc/callback?error=access_denied", "", nil, nil); !strings.Contains(w.Header().Get("Location"), "sso_refused") {
		t.Errorf("provider error: %s", w.Header().Get("Location"))
	}

	// Settings need a login, and the secret is never returned.
	w = do(s, http.MethodPost, "/api/auth/password", `{"new_password":"a long enough password"}`, nil, nil)
	c := sessionCookie(w)
	if w := do(s, http.MethodGet, "/api/auth/oidc", "", nil, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("settings without login: %d", w.Code)
	}
	body := `{"enabled":true,"issuer":"http://127.0.0.1:1","client_id":"cs","client_secret":"TOP-SECRET","public_url":"https://comic.example.com","allowed_groups":["admins"]}`
	w = do(s, http.MethodPut, "/api/auth/oidc", body, nil, c)
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "TOP-SECRET") || !strings.Contains(w.Body.String(), `"has_secret":true`) ||
		!strings.Contains(w.Body.String(), "https://comic.example.com/auth/oidc/callback") {
		t.Errorf("save response = %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"check":{"ok":false`) {
		t.Errorf("unreachable issuer should report a failed check: %s", w.Body.String())
	}
	w = do(s, http.MethodGet, "/api/auth/oidc", "", nil, c)
	if strings.Contains(w.Body.String(), "TOP-SECRET") {
		t.Error("GET leaks the client secret")
	}
	w = do(s, http.MethodGet, "/api/auth/status", "", nil, nil)
	if !strings.Contains(w.Body.String(), `"oidc":true`) {
		t.Errorf("status should offer SSO: %s", w.Body.String())
	}
	// Start with an unreachable provider goes back with a reason too.
	w = do(s, http.MethodGet, "/auth/oidc/start", "", nil, nil)
	if !strings.Contains(w.Header().Get("Location"), "login_error=sso_unreachable") {
		t.Errorf("unreachable start: %d %s", w.Code, w.Header().Get("Location"))
	}
	// Bad settings are a 400 with a reason.
	w = do(s, http.MethodPut, "/api/auth/oidc", `{"enabled":true,"issuer":"https://x.example","client_id":"a","client_secret":"b","public_url":"https://c.example"}`, nil, c)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "allowed group or user") {
		t.Errorf("no allow list: %d %s", w.Code, w.Body.String())
	}
}

// The whole browser trip against a fake provider: start, redirect, callback,
// then the new session works. Also: a person the lists exclude gets no session.
func TestOIDCFullTripThroughHTTP(t *testing.T) {
	idp := authtest.NewFakeIdP(t)
	idp.InfoGroups = []string{"admins"}
	s := authServer(t)
	w := do(s, http.MethodPost, "/api/auth/password", `{"new_password":"a long enough password"}`, nil, nil)
	c := sessionCookie(w)
	cfg := `{"enabled":true,"issuer":"` + idp.URL() + `","client_id":"cs-client","client_secret":"s","public_url":"http://localhost:7620","allowed_groups":["admins"]}`
	if w := do(s, http.MethodPut, "/api/auth/oidc", cfg, nil, c); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"check":{"ok":true`) {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}

	trip := func() *httptest.ResponseRecorder {
		w := do(s, http.MethodGet, "/auth/oidc/start", "", nil, nil)
		if w.Code != http.StatusFound {
			t.Fatalf("start: %d %s", w.Code, w.Header().Get("Location"))
		}
		var binding *http.Cookie
		for _, ck := range w.Result().Cookies() {
			if ck.Name == auth.OIDCCookieName {
				binding = ck
			}
		}
		if binding == nil || !binding.HttpOnly || binding.Path != "/auth/oidc" {
			t.Fatalf("binding cookie = %+v", binding)
		}
		loc, _ := url.Parse(w.Header().Get("Location"))
		idp.Capture(loc.Query())
		cb := "/auth/oidc/callback?code=" + idp.Code() + "&state=" + url.QueryEscape(loc.Query().Get("state"))
		return do(s, http.MethodGet, cb, "", nil, binding)
	}

	w = trip()
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("callback: %d %s", w.Code, w.Header().Get("Location"))
	}
	sso := sessionCookie(w)
	if sso == nil || !sso.HttpOnly {
		t.Fatalf("no session cookie after sign-in")
	}
	if w := do(s, http.MethodGet, "/api/stats", "", nil, sso); w.Code == http.StatusUnauthorized {
		t.Error("single sign-on session not accepted")
	}

	idp.InfoGroups = []string{"guests"}
	w = trip()
	if !strings.Contains(w.Header().Get("Location"), "login_error=not_allowed") || sessionCookie(w) != nil {
		t.Errorf("excluded user: %d %s", w.Code, w.Header().Get("Location"))
	}
}
