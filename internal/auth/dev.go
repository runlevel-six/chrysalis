// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"log/slog"
	"net/http"
)

// DevAuth signs every request in as one fixed user. It exists for local
// development and tests, and config refuses it unless CHRYSALIS_DEV_MODE=true.
type DevAuth struct {
	users *Users
	log   *slog.Logger
}

var devIdentity = Identity{Subject: "dev", Email: "dev@example.com", Name: "Dev User"}

func NewDevAuth(users *Users, log *slog.Logger) *DevAuth {
	return &DevAuth{users: users, log: log}
}

func (d *DevAuth) Register(*http.ServeMux) {}

func (d *DevAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := d.users.Resolve(r.Context(), devIdentity)
		if err != nil {
			d.log.Error("resolve dev user", "err", err)
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}
