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
	"github.com/runlevel-six/chrysalis/internal/config"
	"github.com/runlevel-six/chrysalis/internal/service"
	"github.com/runlevel-six/chrysalis/internal/storage"
	"github.com/runlevel-six/chrysalis/internal/store"
)

type server struct {
	cfg         *config.Config
	store       *store.Store
	boards      *service.Boards
	attachments *service.Attachments
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
// files holds attachment content.
func NewHandler(cfg *config.Config, st *store.Store, files storage.Store, authn auth.Authenticator, static fs.FS, log *slog.Logger) http.Handler {
	boards := service.NewBoards(st, time.Now)
	s := &server{cfg: cfg, store: st, boards: boards, attachments: service.NewAttachments(boards, files), log: log}

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/me", s.me)
	protected.HandleFunc("GET /api/v1/boards", s.listBoards)
	protected.HandleFunc("POST /api/v1/boards", s.createBoard)
	protected.HandleFunc("GET /api/v1/boards/{id}", s.getBoard)
	s.registerContent(protected)
	s.registerAttachments(protected)
	protected.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		auth.WriteError(w, http.StatusNotFound, "not found")
	})
	protected.Handle("/", spaHandler(static))

	public := http.NewServeMux()
	public.HandleFunc("GET /healthz", s.healthz)
	public.HandleFunc("GET /readyz", s.readyz)
	public.Handle("GET /assets/", assetsHandler(static))
	authn.Register(public)
	public.Handle("/", authn.Middleware(protected))

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

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	writeJSON(w, http.StatusOK, u)
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
