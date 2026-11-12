// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionsRollAndExpire(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	clock := newClock()
	ttl := 30 * 24 * time.Hour
	users := NewUsers(st.Queries, clock.Now)
	sessions := NewSessions(st.Queries, ttl, true, clock.Now)

	u, err := users.Resolve(ctx, Identity{Subject: "alice", Email: "alice@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	if err := sessions.Create(ctx, w, httptest.NewRequest("GET", "/", nil), u.ID); err != nil {
		t.Fatal(err)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "__Host-chrysalis_session" || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatalf("cookie = %+v", cookies)
	}
	tok := cookies[0]

	lookup := func() (ok bool, setCookie bool) {
		t.Helper()
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(tok)
		w := httptest.NewRecorder()
		got, ok, err := sessions.Lookup(ctx, w, r)
		if err != nil {
			t.Fatal(err)
		}
		if ok && got.ID != u.ID {
			t.Fatalf("wrong user %q", got.ID)
		}
		return ok, len(w.Result().Cookies()) > 0
	}

	// Shortly after creation: valid, and not worth a write yet.
	clock.Advance(30 * time.Minute)
	if ok, set := lookup(); !ok || set {
		t.Fatalf("after 30m: ok=%v setCookie=%v, want true,false", ok, set)
	}
	// 29 days in: still valid, and the expiry rolls forward.
	clock.Advance(29 * 24 * time.Hour)
	if ok, set := lookup(); !ok || !set {
		t.Fatalf("after 29d: ok=%v setCookie=%v, want true,true", ok, set)
	}
	// Another 29 days: only valid because it rolled.
	clock.Advance(29 * 24 * time.Hour)
	if ok, _ := lookup(); !ok {
		t.Fatal("rolled session expired early")
	}
	// Unused for longer than the TTL: gone.
	clock.Advance(ttl + time.Minute)
	if ok, _ := lookup(); ok {
		t.Fatal("expired session still valid")
	}
}

func TestSessionsDestroyAndUnknownCookie(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	clock := newClock()
	sessions := NewSessions(st.Queries, time.Hour*24, false, clock.Now)
	u, err := NewUsers(st.Queries, clock.Now).Resolve(ctx, Identity{Subject: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	if err := sessions.Create(ctx, w, httptest.NewRequest("GET", "/", nil), u.ID); err != nil {
		t.Fatal(err)
	}
	tok := w.Result().Cookies()[0]
	if tok.Name != "chrysalis_session" {
		t.Fatalf("non-HTTPS cookie name = %q", tok.Name)
	}

	r := httptest.NewRequest("POST", "/auth/logout", nil)
	r.AddCookie(tok)
	if err := sessions.Destroy(ctx, httptest.NewRecorder(), r); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*http.Cookie{tok, {Name: tok.Name, Value: "made-up"}} {
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(c)
		if _, ok, err := sessions.Lookup(ctx, httptest.NewRecorder(), r); ok || err != nil {
			t.Errorf("cookie %q: ok=%v err=%v, want false,nil", c.Value, ok, err)
		}
	}
}
