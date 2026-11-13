// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/service"
)

func TestPrefs(t *testing.T) {
	m := newMultiUser(t, testStatic)
	p := decode[service.Prefs](t, m.as("alice", "GET", "/api/v1/me/prefs", ""))
	if p != service.DefaultPrefs {
		t.Fatalf("defaults = %+v", p)
	}
	if p.HighlightOverdue || !p.CompletionFlutter || p.Mode != "system" || p.Motion != "system" {
		t.Errorf("calm defaults wrong: %+v", p)
	}

	w := m.as("alice", "PATCH", "/api/v1/me/prefs", `{"theme":"butterfly-garden","mode":"dark","density":"compact","motion":"reduce","completionFlutter":false}`)
	p = decode[service.Prefs](t, w)
	if w.Code != http.StatusOK || p.Theme != "butterfly-garden" || p.Mode != "dark" || p.Density != "compact" || p.Motion != "reduce" || p.CompletionFlutter {
		t.Fatalf("update: %d %+v", w.Code, p)
	}
	// A partial update keeps the rest.
	p = decode[service.Prefs](t, m.as("alice", "PATCH", "/api/v1/me/prefs", `{"highlightOverdue":true}`))
	if !p.HighlightOverdue || p.Theme != "butterfly-garden" || p.Density != "compact" {
		t.Fatalf("partial: %+v", p)
	}
	// /me carries them for the first paint.
	me := decode[struct{ Prefs service.Prefs }](t, m.as("alice", "GET", "/api/v1/me", ""))
	if me.Prefs != p {
		t.Errorf("/me prefs = %+v", me.Prefs)
	}
	for _, body := range []string{`{"theme":"neon"}`, `{"mode":"sepia"}`, `{"density":"tiny"}`, `{"motion":"max"}`} {
		if w := m.as("alice", "PATCH", "/api/v1/me/prefs", body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
	// Bob's are his own.
	if bp := decode[service.Prefs](t, m.as("bob", "GET", "/api/v1/me/prefs", "")); bp != service.DefaultPrefs {
		t.Errorf("bob's prefs = %+v", bp)
	}
	if w := m.as("alice", "GET", "/api/v1/branding", ""); w.Code != http.StatusOK {
		t.Errorf("branding: %d", w.Code)
	}
	if w := m.as("", "GET", "/branding/icon", ""); w.Code != http.StatusNotFound {
		t.Errorf("no icon configured: %d", w.Code)
	}
}
