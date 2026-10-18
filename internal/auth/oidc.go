package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"golang.org/x/oauth2"
)

// Sign in with Authelia, or any OpenID Connect provider (tracker #22).
// Authorization-code flow with PKCE. The ID token's signature, issuer,
// audience, expiry and nonce are all checked. A successful sign-in starts the
// same session as the password login. The local password stays as a fallback,
// so an Authelia outage cannot lock the owner out.

const (
	// OIDCCookieName binds a sign-in attempt to the browser that started it.
	OIDCCookieName = "comic_server_oidc"
	// OIDCCallbackPath is the redirect URI path to register with the provider.
	OIDCCallbackPath = "/auth/oidc/callback"

	oidcPendingTTL   = 10 * time.Minute
	oidcPendingMax   = 500
	oidcProviderTTL  = time.Hour
	oidcHTTPTimeout  = 10 * time.Second
	oidcStateTooMany = "too many sign-ins in progress"
)

var (
	// ErrOIDCDenied means the provider vouched for the user but the allow
	// lists do not include them.
	ErrOIDCDenied = errors.New("this account is not allowed")
	// ErrOIDCUnavailable means SSO is not switched on or not complete.
	ErrOIDCUnavailable = errors.New("single sign-on is not available")
)

// Identity is what the provider says about the signed-in person.
type Identity struct {
	Subject       string
	Username      string
	Email         string
	EmailVerified bool
	Groups        []string
}

// Label is a short name for logs.
func (i Identity) Label() string {
	switch {
	case i.Username != "":
		return i.Username
	case i.Email != "":
		return i.Email
	}
	return i.Subject
}

type pendingLogin struct {
	nonce    string
	verifier string
	binding  [32]byte
	expires  time.Time
}

type oidcState struct {
	mu          sync.Mutex
	provider    *oidc.Provider
	providerKey string
	providerAt  time.Time
	pending     map[string]pendingLogin
}

// ValidateOIDCSettings checks a settings change and tidies it in place.
func ValidateOIDCSettings(s *configdb.OIDCSettings) error {
	s.Issuer = strings.TrimSpace(s.Issuer)
	s.ClientID = strings.TrimSpace(s.ClientID)
	s.PublicURL = strings.TrimRight(strings.TrimSpace(s.PublicURL), "/")
	s.AllowedGroups = cleanList(s.AllowedGroups)
	s.AllowedUsers = cleanList(s.AllowedUsers)
	if !s.Enabled {
		return nil
	}
	if s.Issuer == "" || s.ClientID == "" || s.ClientSecret == "" || s.PublicURL == "" {
		return errors.New("issuer URL, client ID, client secret and this server's public URL are all required")
	}
	if err := checkURL("issuer URL", s.Issuer, false); err != nil {
		return err
	}
	if err := checkURL("public URL", s.PublicURL, true); err != nil {
		return err
	}
	if len(s.AllowedGroups) == 0 && len(s.AllowedUsers) == 0 {
		return errors.New("name at least one allowed group or user; otherwise everyone in your single sign-on could get in")
	}
	return nil
}

func cleanList(in []string) []string {
	out := []string{}
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}

func checkURL(what, raw string, bare bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("%s must be a full address such as https://example.com", what)
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return fmt.Errorf("%s must use https (http is only allowed for localhost)", what)
	}
	if u.RawQuery != "" || u.Fragment != "" || (bare && u.Path != "") {
		return fmt.Errorf("%s must not have a path, query or fragment", what)
	}
	return nil
}

// OIDCRedirectURI is the address to register at the provider.
func OIDCRedirectURI(publicURL string) string {
	return strings.TrimRight(publicURL, "/") + OIDCCallbackPath
}

// OIDCSettings returns the saved sign-in settings.
func (m *Manager) OIDCSettings() (configdb.OIDCSettings, error) { return m.db.GetOIDCSettings() }

// OIDCAvailable reports whether the sign-in button should show: SSO is on and
// complete, and login itself is on (SSO never works without a local password).
func (m *Manager) OIDCAvailable() bool {
	if !m.Enabled() {
		return false
	}
	s, err := m.db.GetOIDCSettings()
	if err != nil {
		return false
	}
	return ValidateOIDCSettings(&s) == nil && s.Enabled
}

// SaveOIDCSettings validates and stores new settings. An empty client secret
// keeps the saved one. Turning SSO on needs a local password first.
func (m *Manager) SaveOIDCSettings(in configdb.OIDCSettings) (configdb.OIDCSettings, error) {
	old, err := m.db.GetOIDCSettings()
	if err != nil {
		return in, err
	}
	if in.ClientSecret == "" {
		in.ClientSecret = old.ClientSecret
	}
	if err := ValidateOIDCSettings(&in); err != nil {
		return in, err
	}
	if in.Enabled && !m.Enabled() {
		return in, errors.New("set a local password first; it stays as your way in if single sign-on is down")
	}
	if err := m.db.SaveOIDCSettings(in); err != nil {
		return in, err
	}
	m.oidc.mu.Lock()
	m.oidc.provider = nil
	m.oidc.mu.Unlock()
	return in, nil
}

func (m *Manager) oidcContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, oidcHTTPTimeout)
	return oidc.ClientContext(ctx, &http.Client{Timeout: oidcHTTPTimeout}), cancel
}

func (m *Manager) oidcProvider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	o := m.oidc
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider != nil && o.providerKey == issuer && time.Since(o.providerAt) < oidcProviderTTL {
		return o.provider, nil
	}
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("reach the sign-in provider: %w", err)
	}
	o.provider, o.providerKey, o.providerAt = p, issuer, time.Now()
	return p, nil
}

// CheckOIDC tries provider discovery with the saved settings, for the
// Settings page.
func (m *Manager) CheckOIDC(ctx context.Context) error {
	s, err := m.db.GetOIDCSettings()
	if err != nil {
		return err
	}
	if s.Issuer == "" {
		return errors.New("no issuer URL saved")
	}
	ctx, cancel := m.oidcContext(ctx)
	defer cancel()
	m.oidc.mu.Lock()
	m.oidc.provider = nil // force a fresh look
	m.oidc.mu.Unlock()
	_, err = m.oidcProvider(ctx, s.Issuer)
	return err
}

func (m *Manager) oauthConfig(s configdb.OIDCSettings, p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.ClientID,
		ClientSecret: s.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  OIDCRedirectURI(s.PublicURL),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}
}

// BeginOIDC starts a sign-in. It returns the provider address to send the
// browser to and the value for the binding cookie.
func (m *Manager) BeginOIDC(ctx context.Context) (authURL, binding string, err error) {
	s, err := m.db.GetOIDCSettings()
	if err != nil {
		return "", "", err
	}
	if !m.OIDCAvailable() {
		return "", "", ErrOIDCUnavailable
	}
	ctx, cancel := m.oidcContext(ctx)
	defer cancel()
	p, err := m.oidcProvider(ctx, s.Issuer)
	if err != nil {
		return "", "", err
	}
	state, err := randomToken("")
	if err != nil {
		return "", "", err
	}
	nonce, err := randomToken("")
	if err != nil {
		return "", "", err
	}
	binding, err = randomToken("")
	if err != nil {
		return "", "", err
	}
	verifier := oauth2.GenerateVerifier()

	o := m.oidc
	o.mu.Lock()
	now := time.Now()
	for k, v := range o.pending {
		if now.After(v.expires) {
			delete(o.pending, k)
		}
	}
	if len(o.pending) >= oidcPendingMax {
		o.mu.Unlock()
		return "", "", errors.New(oidcStateTooMany)
	}
	o.pending[state] = pendingLogin{nonce: nonce, verifier: verifier, binding: sha256.Sum256([]byte(binding)), expires: now.Add(oidcPendingTTL)}
	o.mu.Unlock()

	url := m.oauthConfig(s, p).AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
	return url, binding, nil
}

// FinishOIDC completes a sign-in from the provider's redirect. It checks the
// state and the browser binding, swaps the code (with the PKCE verifier),
// verifies the ID token, and applies the allow lists. ErrOIDCDenied carries
// the identity so the caller can log who was refused.
func (m *Manager) FinishOIDC(ctx context.Context, state, binding, code string) (Identity, error) {
	o := m.oidc
	o.mu.Lock()
	pl, ok := o.pending[state]
	delete(o.pending, state) // one use, whatever happens next
	o.mu.Unlock()
	if !ok || time.Now().After(pl.expires) {
		return Identity{}, errors.New("this sign-in expired or was already used; start again")
	}
	got := sha256.Sum256([]byte(binding))
	if binding == "" || subtle.ConstantTimeCompare(got[:], pl.binding[:]) != 1 {
		return Identity{}, errors.New("this sign-in did not start in this browser")
	}
	if code == "" {
		return Identity{}, errors.New("the provider sent no code")
	}
	s, err := m.db.GetOIDCSettings()
	if err != nil {
		return Identity{}, err
	}
	if !s.Enabled {
		return Identity{}, ErrOIDCUnavailable
	}
	ctx, cancel := m.oidcContext(ctx)
	defer cancel()
	p, err := m.oidcProvider(ctx, s.Issuer)
	if err != nil {
		return Identity{}, err
	}
	conf := m.oauthConfig(s, p)
	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(pl.verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("swap the code for a token: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return Identity{}, errors.New("the provider sent no ID token")
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: s.ClientID}).Verify(ctx, raw)
	if err != nil {
		return Identity{}, fmt.Errorf("check the ID token: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(pl.nonce)) != 1 {
		return Identity{}, errors.New("the ID token nonce does not match")
	}
	var claims struct {
		Username      string   `json:"preferred_username"`
		Email         string   `json:"email"`
		EmailVerified bool     `json:"email_verified"`
		Groups        []string `json:"groups"`
	}
	if err := idt.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("read the ID token claims: %w", err)
	}
	id := Identity{Subject: idt.Subject, Username: claims.Username, Email: claims.Email, EmailVerified: claims.EmailVerified, Groups: claims.Groups}

	// Authelia puts groups in the userinfo answer unless a claims policy adds
	// them to the ID token, so look there too. go-oidc checks that the
	// userinfo subject equals the token's.
	if ui, err := p.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err == nil {
		var extra struct {
			Username      string   `json:"preferred_username"`
			Email         string   `json:"email"`
			EmailVerified bool     `json:"email_verified"`
			Groups        []string `json:"groups"`
		}
		if ui.Claims(&extra) == nil {
			if id.Username == "" {
				id.Username = extra.Username
			}
			if id.Email == "" {
				id.Email, id.EmailVerified = extra.Email, extra.EmailVerified
			}
			id.Groups = append(id.Groups, extra.Groups...)
		}
	}
	if !oidcAllowed(s, id) {
		return id, ErrOIDCDenied
	}
	return id, nil
}

// oidcAllowed applies the allow lists. Users match by username, or by email
// only when the provider says it is verified. Matching ignores case.
func oidcAllowed(s configdb.OIDCSettings, id Identity) bool {
	for _, g := range id.Groups {
		for _, want := range s.AllowedGroups {
			if strings.EqualFold(g, want) {
				return true
			}
		}
	}
	for _, want := range s.AllowedUsers {
		if id.Username != "" && strings.EqualFold(id.Username, want) {
			return true
		}
		if id.Email != "" && id.EmailVerified && strings.EqualFold(id.Email, want) {
			return true
		}
	}
	return false
}

// StartSession opens a login session without a password check. Callers must
// have verified the person by other means (single sign-on).
func (m *Manager) StartSession(ip, userAgent string) (string, error) {
	return m.startSession(ip, userAgent)
}

// AllowLoginAttempt spends one try from the client's login budget.
func (m *Manager) AllowLoginAttempt(ip string) bool { return m.limiter.Allow(ip) }

// BindingCookie builds the cookie that ties a sign-in attempt to its browser.
func (m *Manager) BindingCookie(r *http.Request, value string) *http.Cookie {
	c := &http.Cookie{
		Name: OIDCCookieName, Value: value, Path: "/auth/oidc",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: m.IsSecure(r),
		MaxAge: int(oidcPendingTTL.Seconds()),
	}
	if value == "" {
		c.MaxAge = -1
	}
	return c
}
