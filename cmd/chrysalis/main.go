// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Command chrysalis runs the Chrysalis board server.
//
//	chrysalis [serve]          run the server (the default)
//	chrysalis admin <command>  maintenance tasks such as backups; see admin.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/runlevel-six/chrysalis/internal/api"
	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/backup"
	"github.com/runlevel-six/chrysalis/internal/branding"
	"github.com/runlevel-six/chrysalis/internal/config"
	"github.com/runlevel-six/chrysalis/internal/service"
	"github.com/runlevel-six/chrysalis/internal/sse"
	"github.com/runlevel-six/chrysalis/internal/storage"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "chrysalis:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// ctx is cancelled on SIGINT/SIGTERM (Kubernetes sends SIGTERM on shutdown).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch {
	case len(args) == 0 || (args[0] == "serve" && len(args) == 1):
		return serve(ctx)
	case args[0] == "admin":
		return runAdmin(ctx, args[1:], os.Stdout, os.Stderr)
	default:
		return fmt.Errorf("unknown command %q (usage: chrysalis [serve] | chrysalis admin <command>)", strings.Join(args, " "))
	}
}

func serve(ctx context.Context) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("configuration:\n%w", err)
	}
	cfg.Version = version
	log := newLogger(cfg.LogLevel)

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	files, err := storage.NewLocal(cfg.AttachmentsDir())
	if err != nil {
		return err
	}

	brand, err := branding.Load(cfg.BrandingDir, log)
	if err != nil {
		return err
	}

	// Live-sync notices. Closed first on shutdown, so open event streams end
	// and don't hold the server open.
	hub := sse.NewHub()

	authn := newAuthenticator(cfg, st, log)
	go purgeExpiredSessions(ctx, st, log)
	go purgeTrash(ctx, st, files, cfg.TrashRetention, log)
	// Reminders (§3.9) are checked every half minute.
	boards := service.NewBoards(st, time.Now)
	if cfg.Backup.Schedule == "daily" {
		go dailyBackups(ctx, st, cfg, log)
	}

	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: api.NewHandler(api.Deps{
			Config: cfg, Store: st, Files: files, Auth: authn, Static: web.Dist(), Branding: brand, Hub: hub, Boards: boards, Log: log,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// Reminders (§3.9) are checked every half minute, once the handler has
	// connected the service to live sync.
	go fireReminders(ctx, boards, log)
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "version", version, "addr", cfg.ListenAddr, "base_url", cfg.BaseURL.String(), "auth_mode", cfg.AuthMode)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	hub.Close()
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

// purgeTrash permanently deletes items that have been in the trash longer
// than the retention period, hourly.
func purgeTrash(ctx context.Context, st *store.Store, files storage.Store, retention time.Duration, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		cutoff := time.Now().Add(-retention)
		// Attachment files first: once their cards are gone, nothing would
		// point at them any more.
		if n, err := service.PurgeFiles(ctx, st, files, cutoff); err != nil && ctx.Err() == nil {
			log.Error("purge attachment files", "err", err)
		} else if n > 0 {
			log.Info("purged attachment files", "count", n)
		}
		// Notifications are a short-lived list, kept as long as the trash.
		if _, err := service.PurgeNotifications(ctx, st, cutoff); err != nil && ctx.Err() == nil {
			log.Error("purge notifications", "err", err)
		}
		n, err := service.PurgeTrash(ctx, st, cutoff)
		if err != nil && ctx.Err() == nil {
			log.Error("purge trash", "err", err)
		} else if n > 0 {
			log.Info("purged trash", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// dailyBackups backs up once per UTC day while the server runs
// (CHRYSALIS_BACKUP_SCHEDULE=daily). It checks hourly whether today's backup
// exists yet, so a restart neither skips a day nor makes a second one.
func dailyBackups(ctx context.Context, st *store.Store, cfg *config.Config, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		due, err := backup.Due(cfg.Backup.Dir, time.Now())
		if err != nil {
			log.Error("check backups", "err", err)
		} else if due {
			b, removed, err := backup.Run(ctx, backup.Source{
				DB: st.DB, AttachmentsDir: cfg.AttachmentsDir(), Version: cfg.Version,
			}, backupPolicy(cfg), time.Now())
			if err != nil && ctx.Err() == nil {
				log.Error("backup", "err", err)
			}
			if b.Path != "" {
				log.Info("backed up", "path", b.Path, "pruned", len(removed))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// fireReminders turns due reminders into notifications every 30 seconds.
func fireReminders(ctx context.Context, boards *service.Boards, log *slog.Logger) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		if n, err := boards.FireReminders(ctx); err != nil && ctx.Err() == nil {
			log.Error("fire reminders", "err", err)
		} else if n > 0 {
			log.Info("reminders went off", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
