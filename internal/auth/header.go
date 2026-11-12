// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"log/slog"
	"net/http"
	"net/netip"

	"github.com/runlevel-six/chrysalis/internal/config"
)

// HeaderAuth trusts identity headers set by a forward-auth proxy, but only on
// connections that come directly from a trusted proxy address. Anyone else
// could set Remote-User themselves, so their requests are rejected outright.
type HeaderAuth struct {
	names   config.Header
	trusted []netip.Prefix
	users   *Users
	log     *slog.Logger
}

func NewHeaderAuth(names config.Header, trusted []netip.Prefix, users *Users, log *slog.Logger) *HeaderAuth {
	return &HeaderAuth{names: names, trusted: trusted, users: users, log: log}
}

func (h *HeaderAuth) Register(*http.ServeMux) {}

// isTrusted checks the TCP peer address. It deliberately ignores
// X-Forwarded-For: the proxy doing auth is the thing connecting to us.
func (h *HeaderAuth) isTrusted(remoteAddr string) bool {
	ap, err := netip.ParseAddrPort(remoteAddr)
	if err != nil {
		return false
	}
	addr := ap.Addr().Unmap() // treat ::ffff:10.0.0.1 as 10.0.0.1
	for _, p := range h.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func (h *HeaderAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.isTrusted(r.RemoteAddr) {
			h.log.Warn("rejected request from untrusted address in header auth mode", "remote", r.RemoteAddr, "path", r.URL.Path)
			WriteError(w, http.StatusForbidden, "forbidden")
			return
		}
		sub := r.Header.Get(h.names.User)
		if sub == "" {
			unauthenticated(w, r, "")
			return
		}
		u, err := h.users.Resolve(r.Context(), Identity{
			Subject: sub,
			Email:   r.Header.Get(h.names.Email),
			Name:    r.Header.Get(h.names.Name),
		})
		if err != nil {
			h.log.Error("resolve user", "err", err)
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}
