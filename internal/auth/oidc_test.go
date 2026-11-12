// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/runlevel-six/chrysalis/internal/config"
)

// mockIssuer is a minimal OpenID provider: discovery, JWKS, and a token
// endpoint that enforces PKCE. Tests play the browser, so there's no real
// authorization page; authorize() stands in for the user signing in.
type mockIssuer struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey

	mu    sync.Mutex
	codes map[string]pendingCode
}

type pendingCode struct {
	challenge string
	claims    map[string]any
}

func newMockIssuer(t *testing.T) *mockIssuer {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := &mockIssuer{t: t, key: key, codes: map[string]pendingCode{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                m.srv.URL,
			"authorization_endpoint":                m.srv.URL + "/authorize",
			"token_endpoint":                        m.srv.URL + "/token",
			"jwks_uri":                              m.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
		}})
	})
	mux.HandleFunc("POST /token", m.token)
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

// authorize simulates the user approving sign-in at authURL. It returns the
// callback URL the provider would redirect to. extra claims override defaults.
func (m *mockIssuer) authorize(authURL string, extra map[string]any) string {
	u, err := url.Parse(authURL)
	if err != nil {
		m.t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		m.t.Fatalf("authorize request lacks PKCE: %s", authURL)
	}
	claims := map[string]any{
		"iss":   m.srv.URL,
		"sub":   "user-123",
		"aud":   q.Get("client_id"),
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
		"nonce": q.Get("nonce"),
		"email": "alice@example.com",
		"name":  "Alice",
	}
	for k, v := range extra {
		claims[k] = v
	}
	code := randomToken()
	m.mu.Lock()
	m.codes[code] = pendingCode{challenge: q.Get("code_challenge"), claims: claims}
	m.mu.Unlock()

	cb, _ := url.Parse(q.Get("redirect_uri"))
	cq := url.Values{"code": {code}, "state": {q.Get("state")}}
	cb.RawQuery = cq.Encode()
	return cb.String()
}

func (m *mockIssuer) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	m.mu.Lock()
	pc, ok := m.codes[r.PostForm.Get("code")]
	delete(m.codes, r.PostForm.Get("code"))
	m.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != pc.challenge {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
		return
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: m.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		m.t.Fatal(err)
	}
	payload, _ := json.Marshal(pc.claims)
	jws, err := signer.Sign(payload)
	if err != nil {
		m.t.Fatal(err)
	}
	idToken, _ := jws.CompactSerialize()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token": "at", "token_type": "Bearer", "expires_in": 300, "id_token": idToken,
	})
}

const appURL = "https://board.example.home"

// oidcApp is the relying party under test, plus a "browser" with a cookie jar.
type oidcApp struct {
	t       *testing.T
	handler http.Handler
	jar     http.CookieJar
}

func newOIDCApp(t *testing.T, iss *mockIssuer, allowedGroups []string) *oidcApp {
	st := newStore(t)
	clock := newClock()
	o := NewOIDCAuth(config.OIDC{
		Issuer: iss.srv.URL, ClientID: "chrysalis", ClientSecret: "test-secret",
		RedirectURL: appURL + "/auth/callback", Scopes: []string{"openid", "email", "profile"},
		AllowedGroups: allowedGroups,
	}, NewUsers(st.Queries, clock.Now), NewSessions(st.Queries, 24*time.Hour, true, clock.Now), true, discardLogger())
	o.SetHTTPClient(iss.srv.Client())

	mux := http.NewServeMux()
	o.Register(mux)
	mux.Handle("/", o.Middleware(whoami))
	jar, _ := cookiejar.New(nil)
	return &oidcApp{t: t, handler: mux, jar: jar}
}

// do sends a request to the app the way a browser would, cookies included.
func (a *oidcApp) do(method, target string) *http.Response {
	u, err := url.Parse(target)
	if err != nil {
		a.t.Fatal(err)
	}
	if !u.IsAbs() {
		u, _ = url.Parse(appURL + target)
	}
	r := httptest.NewRequest(method, u.String(), nil)
	for _, c := range a.jar.Cookies(u) {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	a.handler.ServeHTTP(w, r)
	resp := w.Result()
	a.jar.SetCookies(u, resp.Cookies())
	return resp
}

// signIn runs /auth/login → provider → /auth/callback and returns the
// callback response.
func (a *oidcApp) signIn(iss *mockIssuer, next string, claims map[string]any) *http.Response {
	login := a.do("GET", "/auth/login?next="+url.QueryEscape(next))
	if login.StatusCode != http.StatusFound {
		a.t.Fatalf("login status = %d", login.StatusCode)
	}
	return a.do("GET", iss.authorize(login.Header.Get("Location"), claims))
}

func body(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestOIDCSignInFlow(t *testing.T) {
	iss := newMockIssuer(t)
	app := newOIDCApp(t, iss, nil)

	// Not signed in yet: API gets 401, pages redirect to login keeping the path.
	if resp := app.do("GET", "/api/v1/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("api before login: %d", resp.StatusCode)
	}
	resp := app.do("GET", "/boards/garden?view=1")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/auth/login?next=%2Fboards%2Fgarden%3Fview%3D1" {
		t.Fatalf("page before login: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	cb := app.signIn(iss, "/boards/garden?view=1", nil)
	if cb.StatusCode != http.StatusSeeOther || cb.Header.Get("Location") != "/boards/garden?view=1" {
		t.Fatalf("callback: %d %q %s", cb.StatusCode, cb.Header.Get("Location"), body(cb))
	}

	me := app.do("GET", "/api/v1/me")
	if me.StatusCode != http.StatusOK {
		t.Fatalf("me after login: %d", me.StatusCode)
	}
	if got := body(me); !strings.Contains(got, "alice@example.com Alice") {
		t.Errorf("me = %q", got)
	}

	// Logging out ends the session.
	if resp := app.do("POST", "/auth/logout"); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	if resp := app.do("GET", "/api/v1/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout: %d", resp.StatusCode)
	}
}

func TestOIDCRejectsBadCallbacks(t *testing.T) {
	iss := newMockIssuer(t)

	t.Run("no login cookie", func(t *testing.T) {
		app := newOIDCApp(t, iss, nil)
		if resp := app.do("GET", "/auth/callback?code=x&state=y"); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("state mismatch", func(t *testing.T) {
		app := newOIDCApp(t, iss, nil)
		login := app.do("GET", "/auth/login")
		cb, _ := url.Parse(iss.authorize(login.Header.Get("Location"), nil))
		q := cb.Query()
		q.Set("state", "forged")
		cb.RawQuery = q.Encode()
		if resp := app.do("GET", cb.String()); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("nonce mismatch", func(t *testing.T) {
		app := newOIDCApp(t, iss, nil)
		if resp := app.signIn(iss, "/", map[string]any{"nonce": "replayed"}); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		app := newOIDCApp(t, iss, nil)
		if resp := app.signIn(iss, "/", map[string]any{"aud": "some-other-app"}); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("login cookie is single use", func(t *testing.T) {
		app := newOIDCApp(t, iss, nil)
		login := app.do("GET", "/auth/login")
		cb := iss.authorize(login.Header.Get("Location"), nil)
		app.do("GET", "/auth/callback?code=bad&state=bad") // consumes the cookie
		if resp := app.do("GET", cb); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})
}

func TestOIDCAllowedGroups(t *testing.T) {
	iss := newMockIssuer(t)

	app := newOIDCApp(t, iss, []string{"family"})
	if resp := app.signIn(iss, "/", map[string]any{"groups": []string{"guests"}}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outside group: status = %d", resp.StatusCode)
	}
	if resp := app.do("GET", "/api/v1/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refused user got a session: %d", resp.StatusCode)
	}

	app = newOIDCApp(t, iss, []string{"family"})
	// Some providers send a single group as a plain string.
	if resp := app.signIn(iss, "/", map[string]any{"groups": "family"}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("in group: status = %d %s", resp.StatusCode, body(resp))
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"":                     "/",
		"/boards/1":            "/boards/1",
		"/boards/1?x=2":        "/boards/1?x=2",
		"https://evil.example": "/",
		"//evil.example/path":  "/",
		`/\evil.example`:       "/",
		"/auth/login?next=/":   "/",
		"boards":               "/",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}
