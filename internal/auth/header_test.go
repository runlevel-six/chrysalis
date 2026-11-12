// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/config"
)

func newHeaderAuth(t *testing.T) http.Handler {
	t.Helper()
	st := newStore(t)
	h := NewHeaderAuth(
		config.Header{User: "Remote-User", Email: "Remote-Email", Name: "Remote-Name", Groups: "Remote-Groups"},
		[]netip.Prefix{netip.MustParsePrefix("10.42.0.0/16")},
		NewUsers(st.Queries, newClock().Now),
		discardLogger(),
	)
	return h.Middleware(whoami)
}

func headerReq(remote, user, email string) *http.Request {
	r := httptest.NewRequest("GET", "/api/v1/me", nil)
	r.RemoteAddr = remote
	if user != "" {
		r.Header.Set("Remote-User", user)
		r.Header.Set("Remote-Email", email)
		r.Header.Set("Remote-Name", "Alice A.")
	}
	return r
}

func TestHeaderAuthRejectsUntrustedPeers(t *testing.T) {
	h := newHeaderAuth(t)
	for _, remote := range []string{"192.0.2.10:5000", "10.43.0.1:5000", "[2001:db8::1]:5000", "garbage"} {
		w := httptest.NewRecorder()
		// Even with a perfectly good Remote-User header, the peer isn't trusted.
		h.ServeHTTP(w, headerReq(remote, "alice", "alice@example.com"))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", remote, w.Code)
		}
	}
}

func TestHeaderAuthIgnoresForwardedFor(t *testing.T) {
	h := newHeaderAuth(t)
	r := headerReq("192.0.2.10:5000", "alice", "alice@example.com")
	r.Header.Set("X-Forwarded-For", "10.42.0.5")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestHeaderAuthTrustedPeer(t *testing.T) {
	h := newHeaderAuth(t)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, headerReq("10.42.0.5:5000", "", ""))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing header: status = %d, want 401", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, headerReq("10.42.0.5:5000", "alice", "alice@example.com"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", w.Code, w.Body)
	}
	first := strings.Fields(w.Body.String())
	if len(first) < 2 || first[1] != "alice@example.com" {
		t.Fatalf("body = %q", w.Body)
	}

	// IPv4-mapped IPv6 from a trusted range counts as trusted, the user keeps
	// the same ID, and a changed email is picked up.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, headerReq("[::ffff:10.42.9.9]:5000", "alice", "alice@new.example.com"))
	if w.Code != http.StatusOK {
		t.Fatalf("mapped address: status = %d", w.Code)
	}
	second := strings.Fields(w.Body.String())
	if second[0] != first[0] || second[1] != "alice@new.example.com" {
		t.Errorf("got %q after %q", second, first)
	}
}
