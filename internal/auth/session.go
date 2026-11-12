// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// Sessions are server-side: the cookie holds a random token and the database
// holds its SHA-256 hash, so a copy of the database can't be replayed as a
// cookie. Sessions roll: each use pushes the expiry out to now+TTL (written
// at most every extendEvery, to avoid a write per request).
type Sessions struct {
	q      *db.Queries
	ttl    time.Duration
	now    func() time.Time
	secure bool
	cookie string
}

const extendEvery = time.Hour

func NewSessions(q *db.Queries, ttl time.Duration, secure bool, now func() time.Time) *Sessions {
	return &Sessions{q: q, ttl: ttl, now: now, secure: secure, cookie: cookieName("chrysalis_session", secure)}
}

// cookieName adds the __Host- prefix over HTTPS. Browsers only accept such a
// cookie if it's Secure, has Path=/ and no Domain, which stops a sibling
// subdomain from planting or overwriting it.
func cookieName(base string, secure bool) string {
	if secure {
		return "__Host-" + base
	}
	return base
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails on supported platforms
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Sessions) setCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// Create starts a session for userID and sets the cookie.
func (s *Sessions) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string) error {
	tok := randomToken()
	now := s.now()
	err := s.q.CreateSession(ctx, db.CreateSessionParams{
		ID:        hashToken(tok),
		UserID:    userID,
		CreatedAt: store.FormatTime(now),
		ExpiresAt: store.FormatTime(now.Add(s.ttl)),
		UserAgent: truncate(r.UserAgent(), 256),
	})
	if err != nil {
		return err
	}
	s.setCookie(w, tok, int(s.ttl.Seconds()))
	return nil
}

// Lookup returns the user for the request's session cookie. ok is false if
// there's no cookie or the session is unknown or expired.
func (s *Sessions) Lookup(ctx context.Context, w http.ResponseWriter, r *http.Request) (u User, ok bool, err error) {
	c, err := r.Cookie(s.cookie)
	if err != nil || c.Value == "" {
		return User{}, false, nil
	}
	id := hashToken(c.Value)
	now := s.now()
	row, err := s.q.GetSessionUser(ctx, db.GetSessionUserParams{ID: id, ExpiresAt: store.FormatTime(now)})
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}

	if exp, perr := store.ParseTime(row.Session.ExpiresAt); perr == nil && exp.Sub(now) < s.ttl-extendEvery {
		if err := s.q.ExtendSession(ctx, db.ExtendSessionParams{ID: id, ExpiresAt: store.FormatTime(now.Add(s.ttl))}); err != nil {
			return User{}, false, err
		}
		if err := s.q.TouchUser(ctx, db.TouchUserParams{ID: row.User.ID, LastSeenAt: store.FormatTime(now)}); err != nil {
			return User{}, false, err
		}
		s.setCookie(w, c.Value, int(s.ttl.Seconds()))
	}
	return userFromRow(row.User), true, nil
}

// Destroy ends the request's session (if any) and clears the cookie.
func (s *Sessions) Destroy(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	s.setCookie(w, "", -1)
	c, err := r.Cookie(s.cookie)
	if err != nil || c.Value == "" {
		return nil
	}
	return s.q.DeleteSession(ctx, hashToken(c.Value))
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
