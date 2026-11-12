// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Command chrysalis runs the Chrysalis board server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/runlevel-six/chrysalis/internal/api"
	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/config"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "chrysalis:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] != "serve" {
		return fmt.Errorf("unknown command %q (usage: chrysalis [serve])", os.Args[1])
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("configuration:\n%w", err)
	}
	log := newLogger(cfg.LogLevel)

	// ctx is cancelled on SIGINT/SIGTERM (Kubernetes sends SIGTERM on shutdown).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer st.Close()

	authn := newAuthenticator(cfg, st, log)
	go purgeExpiredSessions(ctx, st, log)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.NewHandler(cfg, st, authn, web.Dist(), log),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", cfg.ListenAddr, "base_url", cfg.BaseURL.String(), "auth_mode", cfg.AuthMode)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}

func newAuthenticator(cfg *config.Config, st *store.Store, log *slog.Logger) auth.Authenticator {
	users := auth.NewUsers(st.Queries, time.Now)
	switch cfg.AuthMode {
	case config.AuthHeader:
		return auth.NewHeaderAuth(cfg.Header, cfg.TrustedProxies, users, log)
	case config.AuthDev:
		log.Warn("!!! DEV AUTH MODE: every request is signed in as a fixed user. Never use this outside local development. !!!")
		return auth.NewDevAuth(users, log)
	default:
		sessions := auth.NewSessions(st.Queries, cfg.SessionTTL, cfg.SecureCookies(), time.Now)
		return auth.NewOIDCAuth(cfg.OIDC, users, sessions, cfg.SecureCookies(), log)
	}
}

func purgeExpiredSessions(ctx context.Context, st *store.Store, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		n, err := st.DeleteExpiredSessions(ctx, store.FormatTime(time.Now()))
		if err != nil && ctx.Err() == nil {
			log.Error("purge expired sessions", "err", err)
		} else if n > 0 {
			log.Info("purged expired sessions", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
