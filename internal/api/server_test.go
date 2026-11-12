// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/config"
	"github.com/runlevel-six/chrysalis/internal/store"
)

func newTestServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	cfg, err := config.Load(func(k string) string {
		return map[string]string{
			"CHRYSALIS_BASE_URL":  "http://localhost:8080",
			"CHRYSALIS_AUTH_MODE": "dev",
			"CHRYSALIS_DEV_MODE":  "true",
		}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	authn := auth.NewDevAuth(auth.NewUsers(st.Queries, time.Now), log)
	return NewHandler(cfg, st, authn, testStatic, log), st
}

var testStatic = fstest.MapFS{
	"index.html":          {Data: []byte("<!doctype html><title>app</title>")},
	"assets/index-abc.js": {Data: []byte("console.log(1)")},
	"favicon.svg":         {Data: []byte("<svg/>")},
	"assets/sub/.keep":    {Data: nil},
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

func TestHealthAndReady(t *testing.T) {
	h, st := newTestServer(t)
	if w := get(h, "/healthz"); w.Code != http.StatusOK {
		t.Errorf("healthz = %d", w.Code)
	}
	if w := get(h, "/readyz"); w.Code != http.StatusOK {
		t.Errorf("readyz = %d", w.Code)
	}
	st.Close()
	if w := get(h, "/readyz"); w.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz with closed db = %d, want 503", w.Code)
	}
	// Liveness doesn't depend on the database.
	if w := get(h, "/healthz"); w.Code != http.StatusOK {
		t.Errorf("healthz with closed db = %d", w.Code)
	}
}

func TestMeAndSecurityHeaders(t *testing.T) {
	h, _ := newTestServer(t)
	w := get(h, "/api/v1/me")
	if w.Code != http.StatusOK {
		t.Fatalf("me = %d", w.Code)
	}
	var u auth.User
	if err := json.Unmarshal(w.Body.Bytes(), &u); err != nil || u.ID == "" || u.Email != "dev@example.com" {
		t.Fatalf("me body = %s (%v)", w.Body, err)
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP = %q", csp)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}
	if w := get(h, "/api/v1/nope"); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"error"`) {
		t.Errorf("unknown api path = %d %s", w.Code, w.Body)
	}
}

func TestCrossSiteMutationsRejected(t *testing.T) {
	h, _ := newTestServer(t)
	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"cross-site fetch", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, http.StatusForbidden},
		{"foreign origin, no fetch metadata", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"same origin", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://localhost:8080"}, http.StatusNotFound},
		{"non-browser client", nil, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/nothing-here", nil)
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			// 404 means the request got past CSRF protection to routing.
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}
