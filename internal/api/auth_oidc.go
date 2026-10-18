package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/duckpuppy/comic-server/internal/auth"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Sign in with Authelia / OpenID Connect (tracker #22). The two /auth/oidc
// routes sit outside /api/, so the login gate leaves them open: they are how
// a person gets a session in the first place.
//
//	GET /auth/oidc/start      send the browser to the provider
//	GET /auth/oidc/callback   the provider sends it back here with a code
//	GET|PUT /api/auth/oidc    settings (behind the login); the secret is never returned

// oidcSettingsView is the settings as the API shows them.
type oidcSettingsView struct {
	Enabled       bool     `json:"enabled"`
	Issuer        string   `json:"issuer"`
	ClientID      string   `json:"client_id"`
	HasSecret     bool     `json:"has_secret"`
	PublicURL     string   `json:"public_url"`
	AllowedGroups []string `json:"allowed_groups"`
	AllowedUsers  []string `json:"allowed_users"`
	RedirectURI   string   `json:"redirect_uri,omitempty"`
	Check         *struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	} `json:"check,omitempty"`
}

func viewOIDC(s configdb.OIDCSettings) oidcSettingsView {
	v := oidcSettingsView{
		Enabled: s.Enabled, Issuer: s.Issuer, ClientID: s.ClientID, HasSecret: s.ClientSecret != "",
		PublicURL: s.PublicURL, AllowedGroups: append([]string{}, s.AllowedGroups...), AllowedUsers: append([]string{}, s.AllowedUsers...),
	}
	if s.PublicURL != "" {
		v.RedirectURI = auth.OIDCRedirectURI(s.PublicURL)
	}
	return v
}

func (s *Server) handleOIDCSettings(w http.ResponseWriter, r *http.Request) {
	if !s.authReady(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		cur, err := s.auth.OIDCSettings()
		if err != nil {
			http.Error(w, "could not read settings", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		s.writeJSON(w, http.StatusOK, viewOIDC(cur))
	case http.MethodPut:
		var body struct {
			Enabled       bool     `json:"enabled"`
			Issuer        string   `json:"issuer"`
			ClientID      string   `json:"client_id"`
			ClientSecret  string   `json:"client_secret"`
			PublicURL     string   `json:"public_url"`
			AllowedGroups []string `json:"allowed_groups"`
			AllowedUsers  []string `json:"allowed_users"`
		}
		if !decodeSmallJSON(w, r, &body) {
			return
		}
		saved, err := s.auth.SaveOIDCSettings(configdb.OIDCSettings{
			Enabled: body.Enabled, Issuer: body.Issuer, ClientID: body.ClientID, ClientSecret: body.ClientSecret,
			PublicURL: body.PublicURL, AllowedGroups: body.AllowedGroups, AllowedUsers: body.AllowedUsers,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Info().Str("ip", s.auth.ClientIP(r)).Bool("enabled", saved.Enabled).Msg("Single sign-on settings saved")
		v := viewOIDC(saved)
		if saved.Enabled {
			v.Check = &struct {
				OK    bool   `json:"ok"`
				Error string `json:"error,omitempty"`
			}{OK: true}
			if err := s.auth.CheckOIDC(r.Context()); err != nil {
				v.Check.OK, v.Check.Error = false, err.Error()
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		s.writeJSON(w, http.StatusOK, v)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// loginFail sends the browser back to the app with a short reason code the
// login form turns into a sentence. No provider detail goes in the address.
func loginFail(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/?login_error="+url.QueryEscape(code), http.StatusSeeOther)
}

func (s *Server) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authReady(w) {
		return
	}
	authURL, binding, err := s.auth.BeginOIDC(r.Context())
	if err != nil {
		log.Warn().Err(err).Msg("Single sign-on could not start")
		if errors.Is(err, auth.ErrOIDCUnavailable) {
			loginFail(w, r, "sso_off")
		} else {
			loginFail(w, r, "sso_unreachable")
		}
		return
	}
	http.SetCookie(w, s.auth.BindingCookie(r, binding))
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authReady(w) {
		return
	}
	ip := s.auth.ClientIP(r)
	// The attempt is over whatever happens, so the binding cookie goes now.
	http.SetCookie(w, s.auth.BindingCookie(r, ""))
	if !s.auth.AllowLoginAttempt(ip) {
		log.Warn().Str("ip", ip).Msg("Single sign-on rate limited")
		loginFail(w, r, "rate_limited")
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		log.Warn().Str("ip", ip).Str("error", e).Str("description", q.Get("error_description")).Msg("Single sign-on refused by the provider")
		loginFail(w, r, "sso_refused")
		return
	}
	var binding string
	if c, err := r.Cookie(auth.OIDCCookieName); err == nil {
		binding = c.Value
	}
	id, err := s.auth.FinishOIDC(r.Context(), q.Get("state"), binding, q.Get("code"))
	if errors.Is(err, auth.ErrOIDCDenied) {
		log.Warn().Str("ip", ip).Str("user", id.Label()).Msg("Single sign-on denied: account not on the allow lists")
		loginFail(w, r, "not_allowed")
		return
	}
	if err != nil {
		log.Warn().Err(err).Str("ip", ip).Msg("Single sign-on failed")
		loginFail(w, r, "sso_failed")
		return
	}
	value, err := s.auth.StartSession(ip, r.UserAgent())
	if err != nil {
		log.Error().Err(err).Msg("Single sign-on could not start a session")
		loginFail(w, r, "sso_failed")
		return
	}
	log.Info().Str("ip", ip).Str("user", id.Label()).Msg("Login succeeded (single sign-on)")
	http.SetCookie(w, s.auth.Cookie(r, value))
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
