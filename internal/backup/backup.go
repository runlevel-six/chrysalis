// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package backup makes and restores backups of a Chrysalis data directory
// (docs/DESIGN.md §7.4).
//
// A backup is one directory, named for the time it was made, holding:
//
//   - chrysalis.db: a consistent copy of the database, made with SQLite's
//     VACUUM INTO while the server keeps running;
//   - attachments.tar.gz: the local attachment files;
//   - manifest.json: when and by which version it was made, and a SHA-256
//     checksum of each file.
//
// It's assembled in a hidden temporary directory and renamed into place only
// once every file is written and synced, so a crash never leaves a partial
// backup that looks complete.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// namePrefix and nameTimeFormat make backup directory names, such as
	// chrysalis-20261112T030000Z. They sort by name in time order.
	namePrefix     = "chrysalis-"
	nameTimeFormat = "20060102T150405Z"
	tempPrefix     = ".tmp-"

	dbFile          = "chrysalis.db"
	attachmentsFile = "attachments.tar.gz"
	manifestFile    = "manifest.json"

	// manifestFormat changes only if the layout of a backup does.
	manifestFormat = 1
)

// Backup is one complete backup on disk.
type Backup struct {
	Name string    // directory name, such as chrysalis-20261112T030000Z
	Path string    // full path to the directory
	Time time.Time // when it was made (UTC, to the second)
}

// Source is what to back up.
type Source struct {
	// DB is an open connection to the live database.
	DB *sql.DB
	// AttachmentsDir holds the local attachment files. It may not exist yet.
	AttachmentsDir string
	// Version is the app's version, recorded in the manifest.
	Version string
}

type manifest struct {
	Format      int               `json:"format"`
	CreatedAt   string            `json:"created_at"`
	Version     string            `json:"version"`
	Attachments int               `json:"attachments"`
	SHA256      map[string]string `json:"sha256"`
}

// Create writes one backup of src into a new directory under dir, named for
// now, and returns it.
func Create(ctx context.Context, src Source, dir string, now time.Time) (Backup, error) {
	now = now.UTC().Truncate(time.Second)
	b := Backup{Name: namePrefix + now.Format(nameTimeFormat), Time: now}
	b.Path = filepath.Join(dir, b.Name)

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Backup{}, fmt.Errorf("backup: create %s: %w", dir, err)
	}
	if _, err := os.Stat(b.Path); err == nil {
		return Backup{}, fmt.Errorf("backup: %s already exists", b.Path)
	}
	removeStaleTemps(dir, now)

	tmp, err := os.MkdirTemp(dir, tempPrefix)
	if err != nil {
		return Backup{}, fmt.Errorf("backup: %w", err)
	}
	// Removes everything if anything below fails; a no-op after the rename.
	defer func() { _ = os.RemoveAll(tmp) }()

	m := manifest{Format: manifestFormat, CreatedAt: now.Format(time.RFC3339), Version: src.Version, SHA256: map[string]string{}}

	// The database first, then the attachments. Uploads write their file
	// before the row that points at it, so every attachment row in this
	// snapshot already has its file on disk by the time the files are
	// archived.
	dbPath := filepath.Join(tmp, dbFile)
	if _, err := src.DB.ExecContext(ctx, "VACUUM INTO ?", dbPath); err != nil {
		return Backup{}, fmt.Errorf("backup: copy database: %w", err)
	}
	sum, err := syncAndHash(dbPath)
	if err != nil {
		return Backup{}, fmt.Errorf("backup: %w", err)
	}
	m.SHA256[dbFile] = sum

	n, sum, err := writeAttachments(ctx, src.AttachmentsDir, filepath.Join(tmp, attachmentsFile))
	if err != nil {
		return Backup{}, fmt.Errorf("backup: archive attachments: %w", err)
	}
	m.Attachments, m.SHA256[attachmentsFile] = n, sum

	if err := writeManifest(filepath.Join(tmp, manifestFile), m); err != nil {
		return Backup{}, fmt.Errorf("backup: %w", err)
	}
	if err := syncDir(tmp); err != nil {
		return Backup{}, fmt.Errorf("backup: %w", err)
	}
	if err := os.Rename(tmp, b.Path); err != nil {
		return Backup{}, fmt.Errorf("backup: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return Backup{}, fmt.Errorf("backup: %w", err)
	}
	return b, nil
}

// List returns the complete backups in dir, newest first. Temporary
// directories and anything else that isn't a backup are ignored. A missing
// dir has no backups.
func List(dir string) ([]Backup, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("backup: list %s: %w", dir, err)
	}
	var out []Backup
	for _, e := range entries {
		t, ok := parseName(e.Name())
		if !ok || !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(p, manifestFile)); err != nil {
			continue
		}
		out = append(out, Backup{Name: e.Name(), Path: p, Time: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

func parseName(name string) (time.Time, bool) {
	ts, ok := strings.CutPrefix(name, namePrefix)
	if !ok {
		return time.Time{}, false
	}
	t, err := time.Parse(nameTimeFormat, ts)
	return t, err == nil
}

// Prune deletes the backups in dir that the retention policy doesn't keep,
// and returns the ones it deleted.
//
// The policy: the newest backup of each of the keepDaily most recent days
// that have a backup, plus the newest backup of each of the keepWeekly most
// recent ISO weeks that have one. Days and weeks are in UTC. The newest
// backup is always kept.
func Prune(dir string, keepDaily, keepWeekly int) ([]Backup, error) {
	all, err := List(dir)
	if err != nil {
		return nil, err
	}
	keep := retain(all, keepDaily, keepWeekly)
	var removed []Backup
	var errs []error
	for _, b := range all {
		if keep[b.Name] {
			continue
		}
		if err := os.RemoveAll(b.Path); err != nil {
			errs = append(errs, err)
			continue
		}
		removed = append(removed, b)
	}
	return removed, errors.Join(errs...)
}

// retain picks which of all (newest first) to keep.
func retain(all []Backup, keepDaily, keepWeekly int) map[string]bool {
	keep := map[string]bool{}
	if len(all) > 0 {
		keep[all[0].Name] = true
	}
	bucket := func(n int, key func(time.Time) string) {
		seen := map[string]bool{}
		for _, b := range all {
			k := key(b.Time)
			if seen[k] {
				continue
			}
			if len(seen) == n {
				return
			}
			seen[k] = true
			keep[b.Name] = true // the first seen in a bucket is its newest
		}
	}
	bucket(keepDaily, func(t time.Time) string { return t.UTC().Format(time.DateOnly) })
	bucket(keepWeekly, func(t time.Time) string {
		y, w := t.UTC().ISOWeek()
		return fmt.Sprintf("%d-W%02d", y, w)
	})
	return keep
}

// removeStaleTemps clears temporary directories left by backups that crashed
// more than a day ago. Younger ones may belong to a backup still running in
// another process.
func removeStaleTemps(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), tempPrefix) {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > 24*time.Hour {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

// writeAttachments archives every file under src into a gzipped tar at dst,
// returning the number of files and the archive's SHA-256.
func writeAttachments(ctx context.Context, src, dst string) (int, string, error) {
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- dst is inside our own temp dir
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(f, h))
	tw := tar.NewWriter(gz)

	n, err := archiveDir(ctx, src, tw)
	if err != nil {
		return 0, "", err
	}
	if err := tw.Close(); err != nil {
		return 0, "", err
	}
	if err := gz.Close(); err != nil {
		return 0, "", err
	}
	if err := f.Sync(); err != nil {
		return 0, "", err
	}
	if err := f.Close(); err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// archiveDir writes the attachment files under src to tw. It reads through
// os.Root, so a symlink can't lead it outside src.
func archiveDir(ctx context.Context, src string, tw *tar.Writer) (int, error) {
	root, err := os.OpenRoot(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil // no attachments yet
	}
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()

	n := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		switch {
		case name == ".":
			return nil
		case strings.HasPrefix(d.Name(), tempPrefix):
			// A half-written upload (internal/storage writes .tmp-* files
			// and renames them).
			return nil
		case d.IsDir():
			return tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: name + "/", Mode: 0o750})
		case !d.Type().IsRegular():
			return nil // the storage backend never makes anything else
		}
		in, err := root.Open(name)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		info, err := in.Stat()
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg, Name: name, Mode: 0o600, Size: info.Size(), ModTime: info.ModTime(),
		}); err != nil {
			return err
		}
		if _, err := io.CopyN(tw, in, info.Size()); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		n++
		return nil
	})
	return n, err
}

func writeManifest(path string, m manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- inside our own temp dir
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func readManifest(dir string) (manifest, error) {
	var m manifest
	data, err := os.ReadFile(filepath.Join(dir, manifestFile)) // #nosec G304 -- a backup the operator named
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("%s: %w", manifestFile, err)
	}
	if m.Format != manifestFormat {
		return m, fmt.Errorf("%s: unsupported format %d", manifestFile, m.Format)
	}
	return m, nil
}

// syncAndHash flushes path to disk and returns its SHA-256.
func syncAndHash(path string) (string, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0) // #nosec G304 -- inside our own temp dir
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if err := f.Sync(); err != nil {
		return "", err
	}
	return hashReader(f)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- a file inside a backup
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	return hashReader(f)
}

func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// syncDir flushes a directory's entries (new files, renames) to disk.
func syncDir(path string) error {
	d, err := os.Open(path) // #nosec G304 -- our own directories
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
