// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package api wires the HTTP routes: health checks, auth routes, and the
// authenticated /api/v1 JSON API.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/branding"
	"github.com/runlevel-six/chrysalis/internal/config"
	"github.com/runlevel-six/chrysalis/internal/service"
	"github.com/runlevel-six/chrysalis/internal/sse"
	"github.com/runlevel-six/chrysalis/internal/storage"
	"github.com/runlevel-six/chrysalis/internal/store"
)

type server struct {
	cfg         *config.Config
	store       *store.Store
	boards      *service.Boards
	attachments *service.Attachments
	prefs       *service.PrefsService
	centering   *service.Centering
	branding    branding.Branding
	hub         *sse.Hub
	log         *slog.Logger
}

// NewHandler returns the app's root handler.
//
// Routing is two-layered: the public mux holds health checks and the auth
// mode's own routes; everything else falls through "/" into the auth
// middleware and then the protected mux. A new handler added to protected is
// therefore authenticated by default.
//
// static is the built web app (index.html and assets/).
// Deps is everything the HTTP layer needs.
type Deps struct {
	Config   *config.Config
	Store    *store.Store
	Files    storage.Store // attachment content
	Auth     auth.Authenticator
	Static   fs.FS // the built web app
	Branding branding.Branding
	// Hub carries live-sync notices; nil gets a private one (tests).
	Hub *sse.Hub
	// Boards is the content service, shared with background jobs such as
	// reminders; nil makes one (tests).
	Boards *service.Boards
	Log    *slog.Logger
}

func NewHandler(d Deps) http.Handler {
	cfg, st, authn, static, log := d.Config, d.Store, d.Auth, d.Static, d.Log
	boards := d.Boards
	if boards == nil {
		boards = service.NewBoards(st, time.Now)
	}
	hub := d.Hub
	if hub == nil {
		hub = sse.NewHub()
	}
	boards.SetNotifier(hub.Publish)
	s := &server{
		hub: hub,
		cfg: cfg, store: st, boards: boards, attachments: service.NewAttachments(boards, d.Files),
		prefs: service.NewPrefs(st, boards, d.Branding.Has), branding: d.Branding, log: log,
		centering: service.NewCentering(boards),
	}

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/me", s.me)
	protected.HandleFunc("GET /api/v1/boards", s.listBoards)
	protected.HandleFunc("POST /api/v1/boards", s.createBoard)
	protected.HandleFunc("GET /api/v1/boards/{id}", s.getBoard)
	s.registerContent(protected)
	s.registerReminders(protected)
	s.registerAttachments(protected)
	s.registerImports(protected)
	s.registerCentering(protected)
	protected.HandleFunc("GET /api/v1/me/prefs", s.getPrefs)
	protected.HandleFunc("PATCH /api/v1/me/prefs", s.updatePrefs)
	protected.HandleFunc("GET /api/v1/branding", s.getBranding)
	protected.HandleFunc("GET /api/v1/events", s.events)
	protected.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		auth.WriteError(w, http.StatusNotFound, "not found")
	})
	protected.Handle("/", spaHandler(static))

	public := http.NewServeMux()
	public.HandleFunc("GET /healthz", s.healthz)
	public.HandleFunc("GET /readyz", s.readyz)
	public.Handle("GET /assets/", assetsHandler(static))
	// The deployment's icon is public, like the built-in favicon, so
	// browsers can fetch it for tabs and home-screen shortcuts.
	public.HandleFunc("GET /branding/icon", s.brandingIcon)
	public.HandleFunc("GET /manifest.webmanifest", s.manifest)
	authn.Register(public)
	public.Handle("/", authn.Middleware(withOrigin(protected)))

	// CrossOriginProtection (Go 1.25+) rejects state-changing requests
	// (POST, PUT, DELETE, ...) that a browser marks as cross-site, using
	// Sec-Fetch-Site with an Origin-header fallback. That's our CSRF defense,
	// alongside SameSite=Lax cookies (docs/DESIGN.md §6.2).
	cop := http.NewCrossOriginProtection()
	if err := cop.AddTrustedOrigin(cfg.BaseURL.Scheme + "://" + cfg.BaseURL.Host); err != nil {
		panic(err) // BaseURL was validated by config.Load
	}

	return s.recoverer(s.logRequests(securityHeaders(cop.Handler(public))))
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "ok")
}

func (s *server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := s.store.Ready(ctx); err != nil {
		s.log.Warn("not ready", "err", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, "not ready")
		return
	}
	fmt.Fprintln(w, "ready")
}

// me returns the signed-in user, plus the board they last had open so the
// app can bring them straight back to it (§3.3).
func (s *server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	last, err := s.boards.LastOpenedBoard(r.Context(), u.ID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	prefs, err := s.prefs.Get(r.Context(), u.ID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		auth.User
		LastBoardID string        `json:"lastBoardId,omitempty"`
		Version     string        `json:"version"`
		Prefs       service.Prefs `json:"prefs"`
		// Shown instead of "Chrysalis" when the deployment's branding sets one.
		DisplayName string `json:"appName,omitempty"`
	}{u, last, s.cfg.Version, prefs, s.branding.DisplayName})
}

func (s *server) getPrefs(w http.ResponseWriter, r *http.Request) {
	p, err := s.prefs.Get(r.Context(), userID(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) updatePrefs(w http.ResponseWriter, r *http.Request) {
	var patch service.PrefsPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	p, err := s.prefs.Update(r.Context(), userID(r), patch)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// getBranding lists the deployment's extra themes (the built-ins ship in
// the web app itself) and its display name.
func (s *server) getBranding(w http.ResponseWriter, r *http.Request) {
	themes := s.branding.Themes
	if themes == nil {
		themes = []branding.Theme{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"displayName": s.branding.DisplayName, "themes": themes, "hasIcon": s.branding.Icon != nil})
}

func (s *server) brandingIcon(w http.ResponseWriter, r *http.Request) {
	if s.branding.Icon == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", s.branding.IconMIME)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// An SVG icon gets no chance to run script if opened directly.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(s.branding.Icon)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Strict CSP: everything is bundled and served from our own origin.
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status code a handler wrote, for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the real writer (needed later
// for flushing server-sent events).
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		level := slog.LevelInfo
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			level = slog.LevelDebug // probes would otherwise drown the log
		}
		s.log.Log(r.Context(), level, "request",
			"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(), "remote", r.RemoteAddr)
	})
}

func (s *server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) { // deliberate abort; let net/http handle it
					panic(v)
				}
				s.log.Error("panic", "value", v, "path", r.URL.Path, "stack", strings.TrimSpace(string(debug.Stack())))
				auth.WriteError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// manifest makes the app installable (docs/DESIGN.md §3.7), with the
// deployment's own name and icon when branding sets them.
func (s *server) manifest(w http.ResponseWriter, r *http.Request) {
	name := s.branding.DisplayName
	if name == "" {
		name = "Chrysalis"
	}
	icons := []map[string]string{
		{"src": "/icon-192.png", "sizes": "192x192", "type": "image/png"},
		{"src": "/icon-512.png", "sizes": "512x512", "type": "image/png"},
	}
	if s.branding.Icon != nil {
		sizes := "512x512"
		if s.branding.IconMIME == "image/svg+xml" {
			sizes = "any"
		}
		icons = []map[string]string{{"src": "/branding/icon", "sizes": sizes, "type": s.branding.IconMIME}}
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":             name,
		"short_name":       name,
		"description":      "A calm board for the things you're working on.",
		"start_url":        "/",
		"scope":            "/",
		"display":          "standalone",
		"background_color": "#fcf7f3",
		"theme_color":      "#fcf7f3",
		"icons":            icons,
	})
}
