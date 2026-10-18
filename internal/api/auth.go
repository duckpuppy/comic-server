package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/auth"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Built-in login (tracker #21). One gate in ServeHTTP covers every route:
// anything under /api/, /ws or /metrics needs a session cookie or an API
// token once a password is set, except the short open list below. The web
// UI's own files stay open (they hold no data); the UI shows its login form
// when /api/auth/status says so. Until a password is set the gate does
// nothing, so upgrading cannot lock anyone out.
//
//	GET    /api/auth/status          {enabled, authenticated}        (open)
//	POST   /api/auth/login           {password}                      (open, rate limited)
//	POST   /api/auth/logout                                          (open)
//	POST   /api/auth/password        {current_password, new_password} first one turns auth on
//	GET    /api/auth/tokens          list API tokens
//	POST   /api/auth/tokens          {name} -> the token, shown once
//	DELETE /api/auth/tokens/:id      revoke

// openAPIPaths are reachable without logging in. /api/health and
// /api/version are for monitors and the deploy check.
var openAPIPaths = map[string]bool{
	"/api/health":      true,
	"/api/version":     true,
	"/api/auth/status": true,
	"/api/auth/login":  true,
	"/api/auth/logout": true,
}

func isProtectedPath(p string) bool {
	return strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/ws") || strings.HasPrefix(p, "/metrics")
}

// authGate returns false when it has already answered the request.
func (s *Server) authGate(w http.ResponseWriter, r *http.Request) bool {
	if s.auth == nil {
		return true
	}
	// Decide on the cleaned path; the mux cleans it too, so a dotted path
	// cannot dodge the check.
	clean := path.Clean(r.URL.Path)
	if !isProtectedPath(clean) || (openAPIPaths[clean] && clean == r.URL.Path) {
		return true
	}
	if !s.auth.Enabled() {
		return true
	}
	kind := s.auth.Authenticate(r)
	if kind == auth.None {
		w.Header().Set("Cache-Control", "no-store")
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login required"})
		return false
	}
	if kind == auth.Session && !isSafeMethod(r.Method) && !s.auth.SameOrigin(r) {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-site request refused"})
		return false
	}
	return true
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func (s *Server) authReady(w http.ResponseWriter) bool {
	if s.auth == nil {
		http.Error(w, "login is not available", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func decodeSmallJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authReady(w) {
		return
	}
	enabled := s.auth.Enabled()
	w.Header().Set("Cache-Control", "no-store")
	s.writeJSON(w, http.StatusOK, map[string]bool{
		"enabled":       enabled,
		"authenticated": !enabled || s.auth.Authenticate(r) != auth.None,
		"oidc":          enabled && s.auth.OIDCAvailable(),
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authReady(w) {
		return
	}
	if !s.auth.SameOrigin(r) {
		http.Error(w, "cross-site request refused", http.StatusForbidden)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !decodeSmallJSON(w, r, &body) {
		return
	}
	if !s.auth.Enabled() {
		http.Error(w, "login is not enabled", http.StatusConflict)
		return
	}
	ip := s.auth.ClientIP(r)
	value, err := s.auth.Login(ip, r.UserAgent(), body.Password)
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		log.Warn().Str("ip", ip).Msg("Login rate limited")
		w.Header().Set("Retry-After", "900")
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	case errors.Is(err, auth.ErrBadCredentials):
		log.Warn().Str("ip", ip).Msg("Login failed: wrong password")
		http.Error(w, "wrong password", http.StatusUnauthorized)
		return
	case err != nil:
		log.Error().Err(err).Msg("Login failed")
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}
	log.Info().Str("ip", ip).Msg("Login succeeded")
	http.SetCookie(w, s.auth.Cookie(r, value))
	w.Header().Set("Cache-Control", "no-store")
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authReady(w) {
		return
	}
	if !s.auth.SameOrigin(r) {
		http.Error(w, "cross-site request refused", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(auth.CookieName); err == nil {
		s.auth.Logout(c.Value)
	}
	http.SetCookie(w, s.auth.Cookie(r, ""))
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAuthPassword sets or changes the password. With auth off it sets the
// first one (which turns login on); with auth on the gate has already
// required a login and the current password is checked again here.
func (s *Server) handleAuthPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authReady(w) {
		return
	}
	if !s.auth.SameOrigin(r) {
		http.Error(w, "cross-site request refused", http.StatusForbidden)
		return
	}
	var body struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if !decodeSmallJSON(w, r, &body) {
		return
	}
	ip := s.auth.ClientIP(r)
	if s.auth.Enabled() {
		if err := s.auth.CheckPassword(ip, body.Current); err != nil {
			if errors.Is(err, auth.ErrRateLimited) {
				http.Error(w, err.Error(), http.StatusTooManyRequests)
			} else if errors.Is(err, auth.ErrBadCredentials) {
				http.Error(w, "current password is wrong", http.StatusForbidden)
			} else {
				http.Error(w, "could not check password", http.StatusInternalServerError)
			}
			return
		}
	}
	wasEnabled := s.auth.Enabled()
	value, err := s.auth.SetPassword(ip, r.UserAgent(), body.New)
	if err != nil {
		if strings.HasPrefix(err.Error(), "password must be") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Error().Err(err).Msg("Set password failed")
		http.Error(w, "could not save password", http.StatusInternalServerError)
		return
	}
	log.Info().Str("ip", ip).Bool("first_password", !wasEnabled).Msg("Password set")
	http.SetCookie(w, s.auth.Cookie(r, value))
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "enabled": true})
}

func (s *Server) handleAuthTokens(w http.ResponseWriter, r *http.Request) {
	if !s.authReady(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.configDB.ListAuthTokens()
		if err != nil {
			http.Error(w, "could not list tokens", http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"tokens": list})
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if !decodeSmallJSON(w, r, &body) {
			return
		}
		tok, rec, err := s.auth.NewToken(body.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Info().Str("ip", s.auth.ClientIP(r)).Str("name", rec.Name).Msg("API token created")
		w.Header().Set("Cache-Control", "no-store")
		s.writeJSON(w, http.StatusOK, map[string]any{"token": tok, "record": rec})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAuthTokenItem(w http.ResponseWriter, r *http.Request) {
	if !s.authReady(w) {
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/auth/tokens/"), 10, 64)
	if err != nil {
		http.Error(w, "bad token id", http.StatusBadRequest)
		return
	}
	ok, err := s.configDB.DeleteAuthToken(id)
	if err != nil {
		http.Error(w, "could not revoke token", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	log.Info().Str("ip", s.auth.ClientIP(r)).Int64("token_id", id).Msg("API token revoked")
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// trustedProxiesView is the body of GET /api/auth/trusted-proxies.
type trustedProxiesView struct {
	Proxies []string  `json:"proxies"`
	Source  string    `json:"source"` // "saved" (Settings) or "config" (config.yaml / env)
	Config  []string  `json:"config"` // what config.yaml / env say, used when nothing is saved
	Seen    auth.Peek `json:"seen"`   // how THIS request looks to the server
}

// handleTrustedProxies GET|PUT|DELETE /api/auth/trusted-proxies (tracker #24).
// PUT {proxies:[...]} saves a list (an empty one means trust no proxy) and it
// applies at once; DELETE forgets the saved list so config.yaml / env apply.
func (s *Server) handleTrustedProxies(w http.ResponseWriter, r *http.Request) {
	if !s.authReady(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPut:
		var body struct {
			Proxies []string `json:"proxies"`
		}
		if !decodeSmallJSON(w, r, &body) {
			return
		}
		saved, err := s.auth.SaveTrustedProxies(body.Proxies)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Info().Str("ip", s.auth.ClientIP(r)).Strs("proxies", saved).Msg("Trusted proxy list saved")
	case http.MethodDelete:
		if err := s.auth.ClearTrustedProxies(); err != nil {
			http.Error(w, "could not clear the saved list", http.StatusInternalServerError)
			return
		}
		log.Info().Str("ip", s.auth.ClientIP(r)).Msg("Saved trusted proxy list cleared; config.yaml / env apply")
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, source := s.auth.TrustedProxies()
	w.Header().Set("Cache-Control", "no-store")
	s.writeJSON(w, http.StatusOK, trustedProxiesView{Proxies: list, Source: source, Config: s.auth.ConfigProxies(), Seen: s.auth.Peek(r)})
}
