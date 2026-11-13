// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/runlevel-six/chrysalis/internal/store"
)

// ErrTargetNotEmpty means a restore would overwrite existing data and wasn't
// forced.
var ErrTargetNotEmpty = errors.New("the data directory already has a database or attachments")

// Target is where a restore writes: the paths from the server's
// configuration.
type Target struct {
	DBPath         string
	AttachmentsDir string
}

// Restored describes a finished restore.
type Restored struct {
	Attachments int
	// MovedAside is where the data that was there before went, when the
	// restore was forced. Empty if nothing was moved.
	MovedAside string
}

// Restore replaces the database and attachments at t with the backup in
// dir. The server must not be running.
//
// It checks the backup's checksums and runs PRAGMA integrity_check on the
// database before touching anything. If t already has data, Restore returns
// ErrTargetNotEmpty unless force is set; then the old database and
// attachments are moved into a pre-restore-<time> directory beside them
// rather than deleted. Finally it runs any migrations the backup needs to
// match this version.
func Restore(ctx context.Context, dir string, t Target, force bool, now time.Time) (Restored, error) {
	var r Restored
	m, err := readManifest(dir)
	if err != nil {
		return r, fmt.Errorf("restore: %s is not a readable backup: %w", dir, err)
	}
	for _, name := range []string{dbFile, attachmentsFile} {
		sum, err := hashFile(filepath.Join(dir, name))
		if err != nil {
			return r, fmt.Errorf("restore: %w", err)
		}
		if sum != m.SHA256[name] {
			return r, fmt.Errorf("restore: %s doesn't match its checksum; the backup is damaged", name)
		}
	}

	existing, err := existingData(t)
	if err != nil {
		return r, fmt.Errorf("restore: %w", err)
	}
	if len(existing) > 0 && !force {
		return r, ErrTargetNotEmpty
	}

	// Stage everything next to the target (same filesystem, so the final
	// moves are renames), and check it, before changing anything.
	dataDir := filepath.Dir(t.DBPath)
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return r, fmt.Errorf("restore: %w", err)
	}
	stage, err := os.MkdirTemp(dataDir, ".restore-")
	if err != nil {
		return r, fmt.Errorf("restore: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	stagedDB := filepath.Join(stage, dbFile)
	if err := copyFile(filepath.Join(dir, dbFile), stagedDB); err != nil {
		return r, fmt.Errorf("restore: copy database: %w", err)
	}
	if err := IntegrityCheck(ctx, stagedDB); err != nil {
		return r, fmt.Errorf("restore: %w", err)
	}
	stagedFiles := filepath.Join(stage, "attachments")
	n, err := extractAttachments(ctx, filepath.Join(dir, attachmentsFile), stagedFiles)
	if err != nil {
		return r, fmt.Errorf("restore: unpack attachments: %w", err)
	}
	r.Attachments = n

	if len(existing) > 0 {
		aside := filepath.Join(dataDir, "pre-restore-"+now.UTC().Format(nameTimeFormat))
		if err := os.Mkdir(aside, 0o750); err != nil {
			return r, fmt.Errorf("restore: %w", err)
		}
		for _, p := range existing {
			if err := os.Rename(p, filepath.Join(aside, filepath.Base(p))); err != nil {
				return r, fmt.Errorf("restore: move %s aside: %w", p, err)
			}
		}
		r.MovedAside = aside
	}
	// An empty attachments directory isn't "existing data", but it's in the
	// way of the rename.
	if err := os.Remove(t.AttachmentsDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return r, fmt.Errorf("restore: %w", err)
	}
	if err := os.Rename(stagedFiles, t.AttachmentsDir); err != nil {
		return r, fmt.Errorf("restore: %w", err)
	}
	if err := os.Rename(stagedDB, t.DBPath); err != nil {
		return r, fmt.Errorf("restore: %w", err)
	}
	if err := syncDir(dataDir); err != nil {
		return r, fmt.Errorf("restore: %w", err)
	}

	// Opening the store applies any migrations newer than the backup.
	st, err := store.Open(ctx, t.DBPath)
	if err != nil {
		return r, fmt.Errorf("restore: %w", err)
	}
	return r, st.Close()
}

// existingData lists what a restore would replace: the database with its
// WAL and shared-memory files, and a non-empty attachments directory.
func existingData(t Target) ([]string, error) {
	var out []string
	for _, p := range []string{t.DBPath, t.DBPath + "-wal", t.DBPath + "-shm"} {
		if _, err := os.Lstat(p); err == nil {
			out = append(out, p)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	entries, err := os.ReadDir(t.AttachmentsDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if len(entries) > 0 {
		out = append(out, t.AttachmentsDir)
	}
	return out, nil
}

// IntegrityCheck runs SQLite's PRAGMA integrity_check on the database at
// path and returns an error unless it reports "ok".
func IntegrityCheck(ctx context.Context, dbPath string) error {
	if _, err := os.Stat(dbPath); err != nil {
		return err
	}
	conn, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	rows, err := conn.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("integrity check: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var problems []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		if s != "ok" {
			problems = append(problems, s)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("integrity check: %w", err)
	}
	if len(problems) > 0 {
		return fmt.Errorf("integrity check failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

// extractAttachments unpacks a backup's attachment archive into dst, which
// must not exist. Only plain files and directories are accepted, and
// os.Root keeps every path inside dst.
func extractAttachments(ctx context.Context, archive, dst string) (int, error) {
	f, err := os.Open(archive) // #nosec G304 -- a file inside a backup
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer func() { _ = gz.Close() }()

	if err := os.Mkdir(dst, 0o750); err != nil {
		return 0, err
	}
	root, err := os.OpenRoot(dst)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()

	tr := tar.NewReader(gz)
	n := 0
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}
		name := path.Clean(h.Name)
		if !fs.ValidPath(name) {
			return 0, fmt.Errorf("unsafe path %q in archive", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o750); err != nil {
				return 0, err
			}
		case tar.TypeReg:
			if d := path.Dir(name); d != "." {
				if err := root.MkdirAll(d, 0o750); err != nil {
					return 0, err
				}
			}
			out, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return 0, err
			}
			if _, err := io.CopyN(out, tr, h.Size); err != nil {
				_ = out.Close()
				return 0, fmt.Errorf("%s: %w", name, err)
			}
			if err := out.Sync(); err != nil {
				_ = out.Close()
				return 0, err
			}
			if err := out.Close(); err != nil {
				return 0, err
			}
			n++
		default:
			return 0, fmt.Errorf("unexpected entry %q in archive", h.Name)
		}
	}
	return n, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- a file inside a backup
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- our staging dir
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
