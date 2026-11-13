// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newBackup(t *testing.T) Backup {
	t.Helper()
	src, _ := newSource(t)
	b, err := Create(context.Background(), src, t.TempDir(), t0)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func targetIn(dir string) Target {
	return Target{DBPath: filepath.Join(dir, "chrysalis.db"), AttachmentsDir: filepath.Join(dir, "attachments")}
}

func TestRestoreIntoEmptyDir(t *testing.T) {
	b := newBackup(t)
	data := filepath.Join(t.TempDir(), "data") // doesn't exist yet
	r, err := Restore(context.Background(), b.Path, targetIn(data), false, t0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Attachments != 2 || r.MovedAside != "" {
		t.Fatalf("restored = %+v", r)
	}
	if got, _ := os.ReadFile(filepath.Join(data, "attachments", "01", "01ABC")); string(got) != "hello" {
		t.Fatalf("attachment = %q", got)
	}
	entries, _ := os.ReadDir(data)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".restore-") {
			t.Fatalf("staging dir left behind: %s", e.Name())
		}
	}
}

func TestRestoreIntoEmptyAttachmentsDir(t *testing.T) {
	// The server makes an empty attachments directory on start; that alone
	// isn't data worth protecting.
	b := newBackup(t)
	data := t.TempDir()
	if err := os.Mkdir(filepath.Join(data, "attachments"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(context.Background(), b.Path, targetIn(data), false, t0); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreRefusesExistingData(t *testing.T) {
	b := newBackup(t)
	for _, existing := range []string{"chrysalis.db", "chrysalis.db-wal", "attachments/ab/abc"} {
		t.Run(existing, func(t *testing.T) {
			data := t.TempDir()
			writeFile(t, filepath.Join(data, existing), "old")
			_, err := Restore(context.Background(), b.Path, targetIn(data), false, t0)
			if !errors.Is(err, ErrTargetNotEmpty) {
				t.Fatalf("err = %v, want ErrTargetNotEmpty", err)
			}
			if got, _ := os.ReadFile(filepath.Join(data, existing)); string(got) != "old" {
				t.Fatal("existing data changed")
			}
		})
	}
}

func TestRestoreForceMovesOldDataAside(t *testing.T) {
	b := newBackup(t)
	data := t.TempDir()
	writeFile(t, filepath.Join(data, "chrysalis.db"), "old db")
	writeFile(t, filepath.Join(data, "chrysalis.db-wal"), "old wal")
	writeFile(t, filepath.Join(data, "attachments", "ab", "abc"), "old file")

	r, err := Restore(context.Background(), b.Path, targetIn(data), true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if r.MovedAside != filepath.Join(data, "pre-restore-20261112T030000Z") {
		t.Fatalf("moved aside to %q", r.MovedAside)
	}
	for name, want := range map[string]string{
		"chrysalis.db":       "old db",
		"chrysalis.db-wal":   "old wal",
		"attachments/ab/abc": "old file",
	} {
		if got, _ := os.ReadFile(filepath.Join(r.MovedAside, name)); string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	// The old WAL was moved with its database; if it had stayed, SQLite
	// would have tried to replay it into the restored one.
	if got, _ := os.ReadFile(filepath.Join(data, "chrysalis.db-wal")); string(got) == "old wal" {
		t.Error("old WAL left beside the restored database")
	}
	if err := IntegrityCheck(context.Background(), filepath.Join(data, "chrysalis.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "attachments", "ab", "abc")); !os.IsNotExist(err) {
		t.Error("old attachment still in place")
	}
	if got, _ := os.ReadFile(filepath.Join(data, "attachments", "01", "01ABC")); string(got) != "hello" {
		t.Errorf("restored attachment = %q", got)
	}
}

func TestRestoreRejectsDamagedBackup(t *testing.T) {
	t.Run("checksum", func(t *testing.T) {
		b := newBackup(t)
		f, err := os.OpenFile(filepath.Join(b.Path, "attachments.tar.gz"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("x")
		f.Close()
		_, err = Restore(context.Background(), b.Path, targetIn(t.TempDir()), false, t0)
		if err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("integrity", func(t *testing.T) {
		b := newBackup(t)
		p := filepath.Join(b.Path, "chrysalis.db")
		db, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		// Scribble over the second page onward (the first is the header and
		// schema), leaving a file SQLite opens but finds damaged.
		for i := 4096; i < len(db); i++ {
			db[i] = 0x5A
		}
		if err := os.WriteFile(p, db, 0o640); err != nil {
			t.Fatal(err)
		}
		rehash(t, b.Path, "chrysalis.db")
		data := t.TempDir()
		_, err = Restore(context.Background(), b.Path, targetIn(data), false, t0)
		if err == nil {
			t.Fatal("want an error")
		}
		if _, err := os.Stat(filepath.Join(data, "chrysalis.db")); !os.IsNotExist(err) {
			t.Fatal("damaged database was put in place")
		}
	})
	t.Run("not a backup", func(t *testing.T) {
		if _, err := Restore(context.Background(), t.TempDir(), targetIn(t.TempDir()), false, t0); err == nil {
			t.Fatal("want an error")
		}
	})
}

// rehash updates a backup's manifest after a test changes one of its files.
func rehash(t *testing.T, dir, name string) {
	t.Helper()
	m, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.SHA256[name], err = hashFile(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, manifestFile), data, 0o640); err != nil {
		t.Fatal(err)
	}
}

func TestExtractRejectsUnsafeEntries(t *testing.T) {
	tests := map[string]tar.Header{
		"parent path":   {Typeflag: tar.TypeReg, Name: "../escape", Size: 1},
		"absolute path": {Typeflag: tar.TypeReg, Name: "/etc/escape", Size: 1},
		"symlink":       {Typeflag: tar.TypeSymlink, Name: "link", Linkname: "/etc/passwd"},
	}
	for name, h := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gz)
			if err := tw.WriteHeader(&h); err != nil {
				t.Fatal(err)
			}
			if h.Size > 0 {
				tw.Write([]byte("x"))
			}
			tw.Close()
			gz.Close()
			dir := t.TempDir()
			archive := filepath.Join(dir, "a.tar.gz")
			if err := os.WriteFile(archive, buf.Bytes(), 0o640); err != nil {
				t.Fatal(err)
			}
			if _, err := extractAttachments(context.Background(), archive, filepath.Join(dir, "out")); err == nil {
				t.Fatal("want an error")
			}
			if _, err := os.Stat(filepath.Join(dir, "escape")); !os.IsNotExist(err) {
				t.Fatal("file written outside the destination")
			}
		})
	}
}
