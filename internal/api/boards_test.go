// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/config"
	"github.com/runlevel-six/chrysalis/internal/service"
	"github.com/runlevel-six/chrysalis/internal/storage"
	"github.com/runlevel-six/chrysalis/internal/store"
)

// multiUser is a server in header-auth mode, so tests can act as different
// users by setting Remote-User. httptest requests come from 192.0.2.1.
type multiUser struct {
	t      *testing.T
	dbPath string
	h      http.Handler
	st     *store.Store
	files  storage.Store
	env    map[string]string
}

func newMultiUser(t *testing.T, static fs.FS) *multiUser {
	return newMultiUserWith(t, static, nil)
}

// newMultiUserWith adds or overrides CHRYSALIS_* settings for one test.
func newMultiUserWith(t *testing.T, static fs.FS, env map[string]string) *multiUser {
	m := &multiUser{t: t, dbPath: filepath.Join(t.TempDir(), "test.db"), env: env}
	m.open(static)
	return m
}

func (m *multiUser) open(static fs.FS) {
	settings := map[string]string{
		"CHRYSALIS_BASE_URL":            "https://board.example.home",
		"CHRYSALIS_AUTH_MODE":           "header",
		"CHRYSALIS_TRUSTED_PROXY_CIDRS": "192.0.2.0/24",
	}
	for k, v := range m.env {
		settings[k] = v
	}
	cfg, err := config.Load(func(k string) string { return settings[k] })
	if err != nil {
		m.t.Fatal(err)
	}
	st, err := store.Open(context.Background(), m.dbPath)
	if err != nil {
		m.t.Fatal(err)
	}
	m.t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	authn := auth.NewHeaderAuth(cfg.Header, []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}, auth.NewUsers(st.Queries, time.Now), log)
	m.st = st
	files, err := storage.NewLocal(filepath.Join(filepath.Dir(m.dbPath), "attachments"))
	if err != nil {
		m.t.Fatal(err)
	}
	m.files = files
	m.h = NewHandler(Deps{Config: cfg, Store: st, Files: files, Auth: authn, Static: static, Log: log})
}

// as sends a request as user ("" = no identity header).
func (m *multiUser) as(user, method, path, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if user != "" {
		r.Header.Set("Remote-User", user)
		r.Header.Set("Remote-Email", user+"@example.com")
	}
	w := httptest.NewRecorder()
	m.h.ServeHTTP(w, r)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", w.Body, err)
	}
	return v
}

func (m *multiUser) createBoard(user, name string) service.Board {
	m.t.Helper()
	w := m.as(user, "POST", "/api/v1/boards", `{"name":`+jsonString(name)+`}`)
	if w.Code != http.StatusCreated {
		m.t.Fatalf("create %q as %s: %d %s", name, user, w.Code, w.Body)
	}
	return decode[service.Board](m.t, w)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestCreateAndListBoards(t *testing.T) {
	m := newMultiUser(t, testStatic)

	first := m.createBoard("alice", "  Garden plans  ")
	if first.Name != "Garden plans" || first.Role != service.RoleOwner || first.Version != 1 || first.Position != "a0" {
		t.Errorf("first = %+v", first)
	}
	if first.Background.Type != "color" || first.Background.Value != "" {
		t.Errorf("background = %+v", first.Background)
	}
	second := m.createBoard("alice", "Reading list")
	if second.Position <= first.Position {
		t.Errorf("second position %q not after %q", second.Position, first.Position)
	}

	w := m.as("alice", "GET", "/api/v1/boards", "")
	got := decode[struct{ Boards []service.Board }](t, w).Boards
	if len(got) != 2 || got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("list = %+v", got)
	}

	w = m.as("alice", "GET", "/api/v1/boards/"+first.ID, "")
	if w.Code != http.StatusOK || decode[service.Board](t, w).Name != "Garden plans" {
		t.Fatalf("get = %d %s", w.Code, w.Body)
	}
}

func TestCreateBoardValidation(t *testing.T) {
	m := newMultiUser(t, testStatic)
	for _, body := range []string{
		`{"name":""}`,
		`{"name":"   "}`,
		`{}`,
		`{"name":` + jsonString(strings.Repeat("é", service.MaxBoardNameLen+1)) + `}`,
		`not json`,
	} {
		w := m.as("alice", "POST", "/api/v1/boards", body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %.40q: status %d, want 400", body, w.Code)
		}
	}
	// Exactly at the limit (counted in characters, not bytes) is fine.
	m.createBoard("alice", strings.Repeat("é", service.MaxBoardNameLen))
}

// docs/DESIGN.md §9: user B can't read user A's boards via any endpoint.
func TestBoardsAreIsolatedBetweenUsers(t *testing.T) {
	m := newMultiUser(t, testStatic)
	alices := m.createBoard("alice", "Alice's board")

	if w := m.as("bob", "GET", "/api/v1/boards/"+alices.ID, ""); w.Code != http.StatusNotFound {
		t.Errorf("bob GET alice's board: %d", w.Code)
	}
	w := m.as("bob", "GET", "/api/v1/boards", "")
	if got := decode[struct{ Boards []service.Board }](t, w).Boards; len(got) != 0 {
		t.Errorf("bob sees %+v", got)
	}
	// Bob's own boards are positioned independently of Alice's.
	if b := m.createBoard("bob", "Bob's board"); b.Position != "a0" {
		t.Errorf("bob's first position = %q", b.Position)
	}
	if w := m.as("bob", "GET", "/api/v1/boards/does-not-exist", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown id: %d", w.Code)
	}
	// Unauthenticated requests never reach the handlers.
	for _, p := range []string{"/api/v1/boards", "/api/v1/boards/" + alices.ID} {
		if w := m.as("", "GET", p, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous GET %s: %d", p, w.Code)
		}
	}
	if w := m.as("", "POST", "/api/v1/boards", `{"name":"x"}`); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous POST: %d", w.Code)
	}
}

// The Phase 0 exit check, server side: a board survives a restart.
func TestBoardsPersistAcrossRestart(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Garden plans")
	m.st.Close()
	m.open(testStatic)
	w := m.as("alice", "GET", "/api/v1/boards/"+b.ID, "")
	if w.Code != http.StatusOK || decode[service.Board](t, w).Name != "Garden plans" {
		t.Fatalf("after restart: %d %s", w.Code, w.Body)
	}
}

func TestSPAServing(t *testing.T) {
	m := newMultiUser(t, testStatic)

	for _, p := range []string{"/", "/b/01ABC", "/index.html", "/some/deep/link"} {
		w := m.as("alice", "GET", p, "")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<title>app</title>") || w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("GET %s: %d %q cache=%q", p, w.Code, w.Body, w.Header().Get("Cache-Control"))
		}
	}
	if w := m.as("alice", "GET", "/favicon.svg", ""); w.Body.String() != "<svg/>" {
		t.Errorf("favicon: %d %q", w.Code, w.Body)
	}

	// Assets are public and immutable; pages need sign-in.
	w := m.as("", "GET", "/assets/index-abc.js", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %d cache=%q", w.Code, w.Header().Get("Cache-Control"))
	}
	if w := m.as("", "GET", "/", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous page: %d", w.Code)
	}
	for _, p := range []string{"/assets/", "/assets/sub/", "/assets/missing.js"} {
		if w := m.as("", "GET", p, ""); w.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", p, w.Code)
		}
	}
}

func TestSPANotBuilt(t *testing.T) {
	m := newMultiUser(t, fstest.MapFS{".gitkeep": {}})
	w := m.as("alice", "GET", "/", "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "make web") {
		t.Errorf("unbuilt: %d %q", w.Code, w.Body)
	}
}
