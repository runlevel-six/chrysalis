// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package store opens the SQLite database, applies migrations, and exposes
// the sqlc-generated queries in the db subpackage.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/runlevel-six/chrysalis/internal/store/db"

	// Registers the pure-Go "sqlite" driver with database/sql. The blank
	// import (_) runs the package's init() for its side effect only.
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// TimeFormat is how every timestamp is stored: fixed-width UTC RFC 3339 with
// milliseconds. Fixed width means comparing the strings in SQL gives the same
// answer as comparing the times.
const TimeFormat = "2006-01-02T15:04:05.000Z"

// FormatTime converts t to the stored string form.
func FormatTime(t time.Time) string { return t.UTC().Format(TimeFormat) }

// ParseTime is the inverse of FormatTime.
func ParseTime(s string) (time.Time, error) { return time.Parse(TimeFormat, s) }

// Store bundles the connection pool with the generated queries.
type Store struct {
	DB          *sql.DB
	*db.Queries // embedded: s.GetUser(...) works directly on a Store
}

// Open opens (creating if needed) the database at path and runs all pending
// migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	// modernc.org/sqlite applies each _pragma on every new connection.
	// _txlock=immediate makes BEGIN take the write lock up front, so two
	// concurrent write transactions queue on busy_timeout instead of one
	// failing with SQLITE_BUSY partway through.
	q := url.Values{}
	for _, p := range []string{
		"journal_mode(WAL)",
		"busy_timeout(5000)",
		"foreign_keys(1)",
		"synchronous(NORMAL)",
	} {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	dsn := "file:" + path + "?" + q.Encode()

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite allows one writer at a time; a small pool is plenty and keeps
	// lock contention low.
	sqlDB.SetMaxOpenConns(4)

	if err := migrate(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return &Store{DB: sqlDB, Queries: db.New(sqlDB)}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func newProvider(sqlDB *sql.DB) (*goose.Provider, error) {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectSQLite3, sqlDB, sub)
}

func migrate(ctx context.Context, sqlDB *sql.DB) error {
	p, err := newProvider(sqlDB)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// Ready reports whether the database is reachable and fully migrated.
func (s *Store) Ready(ctx context.Context) error {
	if err := s.DB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	p, err := newProvider(s.DB)
	if err != nil {
		return err
	}
	pending, err := p.HasPending(ctx)
	if err != nil {
		return fmt.Errorf("check migrations: %w", err)
	}
	if pending {
		return errors.New("migrations pending")
	}
	return nil
}
