// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"flag"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/store"
)

func TestParseArgs(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	force := fs.Bool("force", false, "")
	pos, err := parseArgs(fs, []string{"one", "--force", "two"})
	if err != nil || !*force || !slices.Equal(pos, []string{"one", "two"}) {
		t.Fatalf("pos = %v, force = %v, err = %v", pos, *force, err)
	}
}

func setEnv(t *testing.T, dataDir, backupDir string) {
	t.Setenv("CHRYSALIS_BASE_URL", "http://localhost:8080")
	t.Setenv("CHRYSALIS_AUTH_MODE", "dev")
	t.Setenv("CHRYSALIS_DEV_MODE", "true")
	t.Setenv("CHRYSALIS_DATA_DIR", dataDir)
	t.Setenv("CHRYSALIS_BACKUP_DIR", backupDir)
}

func admin(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := runAdmin(context.Background(), args, &out, &errOut)
	return out.String() + errOut.String(), err
}

func TestAdminBackupAndRestore(t *testing.T) {
	src, dst, backups := t.TempDir(), t.TempDir(), t.TempDir()
	setEnv(t, src, backups)

	if _, err := admin(t, "backup"); err == nil || !strings.Contains(err.Error(), "no database") {
		t.Fatalf("backup with no database: %v", err)
	}
	st, err := store.Open(context.Background(), filepath.Join(src, "chrysalis.db"))
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	out, err := admin(t, "backup")
	if err != nil || !strings.Contains(out, "backed up to "+backups) {
		t.Fatalf("backup: %q, %v", out, err)
	}
	out, err = admin(t, "list-backups")
	if err != nil || !strings.HasPrefix(out, "chrysalis-") {
		t.Fatalf("list-backups: %q, %v", out, err)
	}
	name := strings.TrimSpace(out)

	// Restoring over the database it came from needs --force.
	if _, err := admin(t, "restore", "latest"); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("restore without --force: %v", err)
	}
	out, err = admin(t, "restore", "latest", "--force")
	if err != nil || !strings.Contains(out, "moved the previous data to") {
		t.Fatalf("restore --force: %q, %v", out, err)
	}

	setEnv(t, dst, backups)
	out, err = admin(t, "restore", name)
	if err != nil || !strings.Contains(out, "restored ") {
		t.Fatalf("restore by name: %q, %v", out, err)
	}
	if _, err := admin(t, "restore", "chrysalis-19990101T000000Z"); err == nil {
		t.Fatal("restore of a missing backup succeeded")
	}
}

func TestAdminUsage(t *testing.T) {
	out, err := admin(t)
	if err == nil || !strings.Contains(out, "restore <backup> [--force]") {
		t.Fatalf("usage: %q, %v", out, err)
	}
	if _, err := admin(t, "nope"); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unknown: %v", err)
	}
}
