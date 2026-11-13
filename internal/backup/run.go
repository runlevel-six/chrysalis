// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"time"
)

// Policy is where backups go and how many to keep.
type Policy struct {
	Dir                   string
	KeepDaily, KeepWeekly int
}

// Run makes one backup and then prunes old ones. It returns the new backup
// and the ones pruned.
func Run(ctx context.Context, src Source, p Policy, now time.Time) (Backup, []Backup, error) {
	b, err := Create(ctx, src, p.Dir, now)
	if err != nil {
		return Backup{}, nil, err
	}
	removed, err := Prune(p.Dir, p.KeepDaily, p.KeepWeekly)
	if err != nil {
		return b, removed, fmt.Errorf("backup: prune: %w", err)
	}
	return b, removed, nil
}

// Due reports whether a daily schedule should make a backup now: there's no
// backup in dir yet from the current UTC day.
func Due(dir string, now time.Time) (bool, error) {
	all, err := List(dir)
	if err != nil {
		return false, err
	}
	if len(all) == 0 {
		return true, nil
	}
	return all[0].Time.UTC().Format(time.DateOnly) != now.UTC().Format(time.DateOnly), nil
}

// OpenSource opens an existing database for backing up from outside the
// server (the admin command). Unlike store.Open it never creates the file or
// runs migrations: a backup should copy exactly what's there.
func OpenSource(dbPath string) (*sql.DB, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("backup: no database to back up: %w", err)
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	return sql.Open("sqlite", "file:"+dbPath+"?"+q.Encode())
}
