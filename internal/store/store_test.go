// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenMigratesAndIsReady(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "chrysalis.db")
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Ready(ctx); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	var mode string
	if err := st.DB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q (%v), want wal", mode, err)
	}
	var fk int
	if err := st.DB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d (%v), want 1", fk, err)
	}
	st.Close()

	// Reopening an already-migrated database is a no-op.
	st, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	st.Close()
}

func TestTimeFormatSortsLikeTime(t *testing.T) {
	a := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	b := a.Add(1500 * time.Millisecond)
	c := a.Add(10 * time.Hour)
	if FormatTime(a) >= FormatTime(b) || FormatTime(b) >= FormatTime(c) {
		t.Errorf("not ordered: %s %s %s", FormatTime(a), FormatTime(b), FormatTime(c))
	}
	// Non-UTC input is normalized.
	loc := time.FixedZone("UTC-5", -5*3600)
	if got := FormatTime(a.In(loc)); got != "2026-01-02T03:04:05.000Z" {
		t.Errorf("FormatTime = %s", got)
	}
	back, err := ParseTime(FormatTime(b))
	if err != nil || !back.Equal(b) {
		t.Errorf("round trip = %v, %v", back, err)
	}
}
