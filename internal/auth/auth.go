// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package auth identifies the user behind each request. There are three
// modes (see docs/DESIGN.md §6.1): oidc, header (trusted forward-auth) and dev.
// Each mode is an Authenticator; the rest of the app only sees the User that
// the middleware puts in the request context.
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// User is the signed-in person, as handlers see them.
type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

// Identity is what an identity source (OIDC token, proxy headers, dev mode)
// tells us about a person.
type Identity struct {
	Subject string
	Email   string
	Name    string
}

// Authenticator is implemented by each auth mode.
type Authenticator interface {
	// Middleware puts the current User in the request context, or responds
	// with 401/403 (or a login redirect) and doesn't call next.
	Middleware(next http.Handler) http.Handler
	// Register adds the mode's own routes, such as /auth/login.
	Register(mux *http.ServeMux)
}

// ctxKey is an unexported type, so no other package can collide with
// (or forge) our context value.
type ctxKey struct{}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom returns the signed-in user. ok is false outside the auth middleware.
func UserFrom(ctx context.Context) (u User, ok bool) {
	u, ok = ctx.Value(ctxKey{}).(User)
	return u, ok
}

func userFromRow(r db.User) User {
	return User{ID: r.ID, Email: r.Email, DisplayName: r.DisplayName}
}

// Users creates users just-in-time on first sight and keeps their profile
// fields current.
//
// Header and dev modes resolve the user on every request, so Users remembers
// recent results and only writes to the database when a profile field
// changes or lastSeenEvery has passed.
type Users struct {
	q   *db.Queries
	now func() time.Time

	mu   sync.Mutex
	seen map[string]seenUser
}

type seenUser struct {
	user    User
	written time.Time
}

const lastSeenEvery = 5 * time.Minute

func NewUsers(q *db.Queries, now func() time.Time) *Users {
	return &Users{q: q, now: now, seen: map[string]seenUser{}}
}

// Resolve returns the user for id, creating or updating them as needed.
func (u *Users) Resolve(ctx context.Context, id Identity) (User, error) {
	now := u.now()
	name := id.Name
	if name == "" {
		name = id.Email
	}
	if name == "" {
		name = id.Subject
	}

	u.mu.Lock()
	s, ok := u.seen[id.Subject]
	u.mu.Unlock()
	if ok && s.user.Email == id.Email && s.user.DisplayName == name && now.Sub(s.written) < lastSeenEvery {
		return s.user, nil
	}

	// On conflict the row keeps its original id, so the new ULID here is
	// only used the first time this subject signs in.
	row, err := u.q.UpsertUser(ctx, db.UpsertUserParams{
		ID:          ulid.Make().String(),
		OidcSub:     id.Subject,
		Email:       id.Email,
		DisplayName: name,
		CreatedAt:   store.FormatTime(now),
		LastSeenAt:  store.FormatTime(now),
	})
	if err != nil {
		return User{}, err
	}
	user := userFromRow(row)
	u.mu.Lock()
	u.seen[id.Subject] = seenUser{user: user, written: now}
	u.mu.Unlock()
	return user, nil
}

// unauthenticated answers a request that has no usable identity. API calls
// get JSON; page loads can be redirected to a login URL when one exists.
func unauthenticated(w http.ResponseWriter, r *http.Request, loginURL string) {
	if loginURL != "" && r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") {
		http.Redirect(w, r, loginURL, http.StatusSeeOther)
		return
	}
	WriteError(w, http.StatusUnauthorized, "not signed in")
}

// WriteError writes a small JSON error body: {"error": msg}.
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
