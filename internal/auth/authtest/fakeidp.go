// Package authtest is a small fake OpenID Connect provider for tests of the
// sign-in flow: discovery, keys, token (with PKCE check) and userinfo
// endpoints, signing real RS256 ID tokens. Test code only.
package authtest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// FakeIdP is a tiny OpenID Connect provider: discovery, keys, token and
// userinfo endpoints, signing real RS256 ID tokens.
type FakeIdP struct {
	srv  *httptest.Server
	key  *rsa.PrivateKey
	mu   sync.Mutex
	req  url.Values // query of the last authorize request, set by the test
	code string

	// What the next token carries; tests change these.
	Aud         string
	Nonce       string // empty = use the request's nonce
	Expires     time.Duration
	TokenGroups []string
	InfoGroups  []string
	Username    string
	Email       string
	EmailOK     bool
	Sub         string
}

func NewFakeIdP(t *testing.T) *FakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &FakeIdP{key: key, code: "the-code", Expires: time.Hour, Username: "paikens", Sub: "user-1"}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.URL(), "authorization_endpoint": f.URL() + "/authorize", "token_endpoint": f.URL() + "/token",
			"jwks_uri": f.URL() + "/jwks", "userinfo_endpoint": f.URL() + "/userinfo",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &f.key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = r.ParseForm()
		if r.PostForm.Get("code") != f.code || r.PostForm.Get("grant_type") != "authorization_code" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		// PKCE: the verifier must hash to the challenge sent on authorize.
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != f.req.Get("code_challenge") || f.req.Get("code_challenge_method") != "S256" {
			http.Error(w, `{"error":"invalid_grant","error_description":"pkce"}`, http.StatusBadRequest)
			return
		}
		nonce := f.Nonce
		if nonce == "" {
			nonce = f.req.Get("nonce")
		}
		aud := f.Aud
		if aud == "" {
			aud = "cs-client"
		}
		claims := map[string]any{
			"iss": f.URL(), "sub": f.Sub, "aud": aud, "nonce": nonce,
			"iat": time.Now().Unix(), "exp": time.Now().Add(f.Expires).Unix(),
			"preferred_username": f.Username, "email": f.Email, "email_verified": f.EmailOK,
		}
		if f.TokenGroups != nil {
			claims["groups"] = f.TokenGroups
		}
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: f.key, KeyID: "k1"}}, (&jose.SignerOptions{}).WithType("JWT"))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		payload, _ := json.Marshal(claims)
		jws, err := signer.Sign(payload)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		raw, _ := jws.CompactSerialize()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": raw})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		out := map[string]any{"sub": f.Sub, "preferred_username": f.Username}
		if f.InfoGroups != nil {
			out["groups"] = f.InfoGroups
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return f
}

// URL is the provider's address, which is also its issuer.
func (f *FakeIdP) URL() string { return f.srv.URL }

// Capture records the query of the authorize request the browser would have
// sent, so the token endpoint can check PKCE and echo the nonce.
func (f *FakeIdP) Capture(q url.Values) {
	f.mu.Lock()
	f.req = q
	f.mu.Unlock()
}

// Code is the authorization code the token endpoint accepts.
func (f *FakeIdP) Code() string { return f.code }
