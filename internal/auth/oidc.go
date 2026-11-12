// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/runlevel-six/chrysalis/internal/config"
)

// OIDCAuth makes the app an OpenID Connect relying party using the
// authorization-code flow with PKCE:
//
//  1. GET /auth/login stores a random state, PKCE verifier and nonce in a
//     short-lived cookie and redirects to the identity provider.
//  2. The provider sends the browser back to GET /auth/callback with a code.
//     We check state against the cookie, exchange the code (proving we hold
//     the PKCE verifier), verify the ID token's signature and nonce, then
//     create the user (if new) and a session.
//
// Provider discovery happens on first use rather than at startup, so the app
// still starts (and serves /healthz) while the identity provider is down.
type OIDCAuth struct {
	cfg      config.OIDC
	users    *Users
	sessions *Sessions
	secure   bool
	log      *slog.Logger
	// client is used for all calls to the provider; tests swap in their own.
	client *http.Client

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
}

const loginCookieTTL = 10 * time.Minute

func NewOIDCAuth(cfg config.OIDC, users *Users, sessions *Sessions, secure bool, log *slog.Logger) *OIDCAuth {
	return &OIDCAuth{
		cfg: cfg, users: users, sessions: sessions, secure: secure, log: log,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// SetHTTPClient replaces the client used to talk to the provider (for tests).
func (o *OIDCAuth) SetHTTPClient(c *http.Client) { o.client = c }

func (o *OIDCAuth) providerCtx(ctx context.Context) context.Context {
	// go-oidc and oauth2 both pick up a custom *http.Client from the context.
	return oidc.ClientContext(ctx, o.client)
}

func (o *OIDCAuth) setup(ctx context.Context) (*oidc.IDTokenVerifier, *oauth2.Config, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.verifier != nil {
		return o.verifier, o.oauth, nil
	}
	// Discovery must outlive this request's context: go-oidc keeps using the
	// context it was given to fetch signing keys later.
	p, err := oidc.NewProvider(oidc.ClientContext(context.Background(), o.client), o.cfg.Issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("oidc discovery for %s: %w", o.cfg.Issuer, err)
	}
	o.verifier = p.Verifier(&oidc.Config{ClientID: o.cfg.ClientID})
	o.oauth = &oauth2.Config{
		ClientID:     o.cfg.ClientID,
		ClientSecret: o.cfg.ClientSecret,
		RedirectURL:  o.cfg.RedirectURL,
		Endpoint:     p.Endpoint(),
		Scopes:       o.cfg.Scopes,
	}
	return o.verifier, o.oauth, nil
}

func (o *OIDCAuth) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", o.login)
	mux.HandleFunc("GET /auth/callback", o.callback)
	mux.HandleFunc("POST /auth/logout", o.logout)
}

func (o *OIDCAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok, err := o.sessions.Lookup(r.Context(), w, r)
		if err != nil {
			o.log.Error("session lookup", "err", err)
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !ok {
			unauthenticated(w, r, "/auth/login?next="+url.QueryEscape(r.URL.RequestURI()))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

// loginState rides in a cookie between /auth/login and /auth/callback.
type loginState struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
	Nonce    string `json:"n"`
	Next     string `json:"x"`
}

func (o *OIDCAuth) loginCookieName() string { return cookieName("chrysalis_login", o.secure) }

func (o *OIDCAuth) setLoginCookie(w http.ResponseWriter, value string, maxAge int) {
	//nolint:gosec // G124 false positive: Secure follows the base URL scheme (false only for plain-HTTP dev).
	http.SetCookie(w, &http.Cookie{
		Name:     o.loginCookieName(),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   o.secure,
		// Lax, not Strict: the callback is a top-level navigation coming from
		// the provider's site, and Strict cookies aren't sent on those.
		SameSite: http.SameSiteLaxMode,
	})
}

func (o *OIDCAuth) login(w http.ResponseWriter, r *http.Request) {
	_, oc, err := o.setup(r.Context())
	if err != nil {
		o.log.Error("oidc setup", "err", err)
		loginError(w, http.StatusBadGateway, "The sign-in service isn't reachable right now. Please try again in a moment.")
		return
	}
	st := loginState{
		State:    randomToken(),
		Verifier: oauth2.GenerateVerifier(),
		Nonce:    randomToken(),
		Next:     safeNext(r.URL.Query().Get("next")),
	}
	raw, _ := json.Marshal(st)
	o.setLoginCookie(w, base64.RawURLEncoding.EncodeToString(raw), int(loginCookieTTL.Seconds()))
	http.Redirect(w, r, oc.AuthCodeURL(st.State, oauth2.S256ChallengeOption(st.Verifier), oidc.Nonce(st.Nonce)), http.StatusFound)
}

// idClaims are the ID token claims we use. Groups accepts either a JSON
// array or a single string, since providers differ.
type idClaims struct {
	Subject           string      `json:"sub"`
	Email             string      `json:"email"`
	Name              string      `json:"name"`
	PreferredUsername string      `json:"preferred_username"`
	Nonce             string      `json:"nonce"`
	Groups            stringOrArr `json:"groups"`
}

type stringOrArr []string

func (s *stringOrArr) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*s = []string{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

func (o *OIDCAuth) callback(w http.ResponseWriter, r *http.Request) {
	st, err := o.readLoginCookie(r)
	o.setLoginCookie(w, "", -1) // single use, whatever happens next
	if err != nil {
		o.log.Info("oidc callback without valid login state", "err", err)
		loginError(w, http.StatusBadRequest, "That sign-in link has expired. Please start again.")
		return
	}
	q := r.URL.Query()
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(st.State)) != 1 {
		loginError(w, http.StatusBadRequest, "That sign-in link has expired. Please start again.")
		return
	}
	if e := q.Get("error"); e != "" {
		o.log.Info("oidc provider returned error", "error", e, "description", q.Get("error_description"))
		loginError(w, http.StatusUnauthorized, "Sign-in didn't complete. Please try again.")
		return
	}

	verifier, oc, err := o.setup(r.Context())
	if err != nil {
		o.log.Error("oidc setup", "err", err)
		loginError(w, http.StatusBadGateway, "The sign-in service isn't reachable right now. Please try again in a moment.")
		return
	}
	ctx := o.providerCtx(r.Context())
	tok, err := oc.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		o.log.Warn("oidc code exchange failed", "err", err)
		loginError(w, http.StatusUnauthorized, "Sign-in didn't complete. Please try again.")
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	if rawID == "" {
		o.log.Warn("oidc token response had no id_token")
		loginError(w, http.StatusUnauthorized, "Sign-in didn't complete. Please try again.")
		return
	}
	idTok, err := verifier.Verify(ctx, rawID)
	if err != nil {
		o.log.Warn("oidc id token verification failed", "err", err)
		loginError(w, http.StatusUnauthorized, "Sign-in didn't complete. Please try again.")
		return
	}
	var c idClaims
	if err := idTok.Claims(&c); err != nil || c.Subject == "" {
		o.log.Warn("oidc id token claims unusable", "err", err)
		loginError(w, http.StatusUnauthorized, "Sign-in didn't complete. Please try again.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(c.Nonce), []byte(st.Nonce)) != 1 {
		o.log.Warn("oidc nonce mismatch")
		loginError(w, http.StatusUnauthorized, "Sign-in didn't complete. Please try again.")
		return
	}
	if len(o.cfg.AllowedGroups) > 0 && !slices.ContainsFunc(c.Groups, func(g string) bool {
		return slices.Contains(o.cfg.AllowedGroups, g)
	}) {
		o.log.Info("oidc user not in allowed groups", "sub", c.Subject)
		loginError(w, http.StatusForbidden, "This account doesn't have access to this board app. Ask the person who runs it to add you.")
		return
	}

	name := c.Name
	if name == "" {
		name = c.PreferredUsername
	}
	u, err := o.users.Resolve(r.Context(), Identity{Subject: c.Subject, Email: c.Email, Name: name})
	if err == nil {
		err = o.sessions.Create(r.Context(), w, r, u.ID)
	}
	if err != nil {
		o.log.Error("oidc sign-in", "err", err)
		loginError(w, http.StatusInternalServerError, "Something went wrong signing you in. Please try again.")
		return
	}
	http.Redirect(w, r, st.Next, http.StatusSeeOther)
}

func (o *OIDCAuth) readLoginCookie(r *http.Request) (loginState, error) {
	var st loginState
	c, err := r.Cookie(o.loginCookieName())
	if err != nil {
		return st, err
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, err
	}
	if st.State == "" || st.Verifier == "" || st.Nonce == "" {
		return st, errors.New("incomplete login state")
	}
	st.Next = safeNext(st.Next)
	return st, nil
}

func (o *OIDCAuth) logout(w http.ResponseWriter, r *http.Request) {
	if err := o.sessions.Destroy(r.Context(), w, r); err != nil {
		o.log.Error("logout", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// safeNext only allows same-site relative paths, so /auth/login?next= can't
// be used to bounce someone to another site after they sign in.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") || strings.HasPrefix(next, "/auth/") {
		return "/"
	}
	return next
}

// loginError shows a short, calm, plain page. Sign-in failures are shown to
// people, not API clients.
func loginError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Chrysalis</title>
<body style="font-family:system-ui,sans-serif;max-width:32rem;margin:15vh auto;padding:0 1rem;line-height:1.5">
<p>%s</p><p><a href="/auth/login">Try signing in again</a></p></body>`, html.EscapeString(msg))
}
