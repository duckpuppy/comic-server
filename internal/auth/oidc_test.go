package auth

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/auth/authtest"
	"github.com/duckpuppy/comic-server/internal/configdb"
)

// ssoManager returns a Manager with a local password set and SSO pointed at f.
func ssoManager(t *testing.T, f *authtest.FakeIdP, groups, users []string) *Manager {
	t.Helper()
	m := newManager(t)
	if _, err := m.SetPassword("1.1.1.1", "", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	_, err := m.SaveOIDCSettings(configdb.OIDCSettings{
		Enabled: true, Issuer: f.URL(), ClientID: "cs-client", ClientSecret: "s3cret",
		PublicURL: "http://localhost:7620", AllowedGroups: groups, AllowedUsers: users,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// begin starts a sign-in and records the authorize query on the provider.
func begin(t *testing.T, m *Manager, f *authtest.FakeIdP) (state, binding string) {
	t.Helper()
	authURL, binding, err := m.BeginOIDC(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authURL)
	if err != nil || !strings.HasPrefix(authURL, f.URL()+"/authorize") {
		t.Fatalf("auth url = %q (%v)", authURL, err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" || q.Get("state") == "" || q.Get("response_type") != "code" ||
		q.Get("redirect_uri") != "http://localhost:7620/auth/oidc/callback" || !strings.Contains(q.Get("scope"), "openid") {
		t.Fatalf("authorize query = %v", q)
	}
	f.Capture(q)
	return q.Get("state"), binding
}

func TestOIDCSuccessGroupsFromUserinfo(t *testing.T) {
	f := authtest.NewFakeIdP(t)
	f.InfoGroups = []string{"admins"} // Authelia's default: groups only via userinfo
	m := ssoManager(t, f, []string{"Admins"}, nil)
	state, binding := begin(t, m, f)
	id, err := m.FinishOIDC(context.Background(), state, binding, f.Code())
	if err != nil {
		t.Fatal(err)
	}
	if id.Username != "paikens" || id.Subject != "user-1" {
		t.Errorf("identity = %+v", id)
	}
}

func TestOIDCSuccessGroupsInIDToken(t *testing.T) {
	f := authtest.NewFakeIdP(t)
	f.TokenGroups = []string{"comic"}
	m := ssoManager(t, f, []string{"comic"}, nil)
	state, binding := begin(t, m, f)
	if _, err := m.FinishOIDC(context.Background(), state, binding, f.Code()); err != nil {
		t.Fatal(err)
	}
}

func TestOIDCAllowedUserByNameAndVerifiedEmail(t *testing.T) {
	f := authtest.NewFakeIdP(t)
	m := ssoManager(t, f, nil, []string{"PAIKENS"})
	state, binding := begin(t, m, f)
	if _, err := m.FinishOIDC(context.Background(), state, binding, f.Code()); err != nil {
		t.Fatalf("by username: %v", err)
	}

	f2 := authtest.NewFakeIdP(t)
	f2.Username, f2.Email, f2.EmailOK = "other", "me@example.com", false
	m2 := ssoManager(t, f2, nil, []string{"me@example.com"})
	state, binding = begin(t, m2, f2)
	if _, err := m2.FinishOIDC(context.Background(), state, binding, f.Code()); err != ErrOIDCDenied {
		t.Fatalf("unverified email err = %v, want denied", err)
	}
	f2.EmailOK = true
	state, binding = begin(t, m2, f2)
	if _, err := m2.FinishOIDC(context.Background(), state, binding, f.Code()); err != nil {
		t.Fatalf("verified email: %v", err)
	}
}

func TestOIDCDeniedWhenNotOnLists(t *testing.T) {
	f := authtest.NewFakeIdP(t)
	f.InfoGroups = []string{"family"}
	m := ssoManager(t, f, []string{"admins"}, []string{"someone-else"})
	state, binding := begin(t, m, f)
	if _, err := m.FinishOIDC(context.Background(), state, binding, f.Code()); err != ErrOIDCDenied {
		t.Fatalf("err = %v, want denied", err)
	}
}

func TestOIDCRejectsBadTokens(t *testing.T) {
	cases := map[string]func(f *authtest.FakeIdP){
		"wrong nonce":    func(f *authtest.FakeIdP) { f.Nonce = "not-the-nonce" },
		"wrong audience": func(f *authtest.FakeIdP) { f.Aud = "someone-elses-client" },
		"expired":        func(f *authtest.FakeIdP) { f.Expires = -time.Hour },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := authtest.NewFakeIdP(t)
			f.TokenGroups = []string{"admins"}
			mutate(f)
			m := ssoManager(t, f, []string{"admins"}, nil)
			state, binding := begin(t, m, f)
			if _, err := m.FinishOIDC(context.Background(), state, binding, f.Code()); err == nil || err == ErrOIDCDenied {
				t.Fatalf("err = %v, want a verification error", err)
			}
		})
	}
}

func TestOIDCStateRules(t *testing.T) {
	f := authtest.NewFakeIdP(t)
	f.TokenGroups = []string{"admins"}
	m := ssoManager(t, f, []string{"admins"}, nil)

	// A state is single use.
	state, binding := begin(t, m, f)
	if _, err := m.FinishOIDC(context.Background(), state, binding, f.Code()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FinishOIDC(context.Background(), state, binding, f.Code()); err == nil {
		t.Error("replayed state accepted")
	}
	// Unknown state.
	if _, err := m.FinishOIDC(context.Background(), "made-up", binding, f.Code()); err == nil {
		t.Error("unknown state accepted")
	}
	// Another browser (wrong or missing binding cookie).
	state, _ = begin(t, m, f)
	if _, err := m.FinishOIDC(context.Background(), state, "someone-elses-cookie", f.Code()); err == nil {
		t.Error("wrong binding accepted")
	}
	state, _ = begin(t, m, f)
	if _, err := m.FinishOIDC(context.Background(), state, "", f.Code()); err == nil {
		t.Error("missing binding accepted")
	}
	// The provider refuses the code.
	state, binding = begin(t, m, f)
	if _, err := m.FinishOIDC(context.Background(), state, binding, "wrong-code"); err == nil {
		t.Error("bad code accepted")
	}
}

func TestOIDCSettingsRules(t *testing.T) {
	f := authtest.NewFakeIdP(t)
	m := newManager(t)
	good := configdb.OIDCSettings{Enabled: true, Issuer: f.URL(), ClientID: "c", ClientSecret: "s",
		PublicURL: "https://comic.example.com/", AllowedGroups: []string{"g"}}

	// Needs a local password first.
	if _, err := m.SaveOIDCSettings(good); err == nil || !strings.Contains(err.Error(), "local password") {
		t.Fatalf("without password err = %v", err)
	}
	if _, err := m.SetPassword("1.1.1.1", "", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	saved, err := m.SaveOIDCSettings(good)
	if err != nil {
		t.Fatal(err)
	}
	if saved.PublicURL != "https://comic.example.com" {
		t.Errorf("public url = %q", saved.PublicURL)
	}
	// An empty secret keeps the saved one.
	keep := good
	keep.ClientSecret = ""
	keep.ClientID = "c2"
	got, err := m.SaveOIDCSettings(keep)
	if err != nil || got.ClientSecret != "s" {
		t.Fatalf("keep secret: %v %q", err, got.ClientSecret)
	}
	bad := map[string]func(s *configdb.OIDCSettings){
		"no allow list":   func(s *configdb.OIDCSettings) { s.AllowedGroups = nil },
		"http public url": func(s *configdb.OIDCSettings) { s.PublicURL = "http://comic.example.com" },
		"http issuer":     func(s *configdb.OIDCSettings) { s.Issuer = "http://login.example.com" },
		"path in public":  func(s *configdb.OIDCSettings) { s.PublicURL = "https://comic.example.com/app" },
		"no issuer":       func(s *configdb.OIDCSettings) { s.Issuer = "" },
		"not a url":       func(s *configdb.OIDCSettings) { s.Issuer = "login" },
	}
	for name, mutate := range bad {
		s := good
		mutate(&s)
		if _, err := m.SaveOIDCSettings(s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Switching it off needs none of that.
	if _, err := m.SaveOIDCSettings(configdb.OIDCSettings{}); err != nil {
		t.Errorf("turning off: %v", err)
	}
	if m.OIDCAvailable() {
		t.Error("available after turning off")
	}
}

func TestOIDCUnavailableWithoutLocalLogin(t *testing.T) {
	f := authtest.NewFakeIdP(t)
	m := ssoManager(t, f, []string{"g"}, nil)
	if !m.OIDCAvailable() {
		t.Fatal("should be available")
	}
	if err := m.db.DisableAuth(); err != nil {
		t.Fatal(err)
	}
	m.setEnabled(false)
	if m.OIDCAvailable() {
		t.Error("SSO offered although login is off")
	}
	if _, _, err := m.BeginOIDC(context.Background()); err != ErrOIDCUnavailable {
		t.Errorf("begin err = %v", err)
	}
}
