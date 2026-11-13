// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/store"
)

var t0 = time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC)

// newSource makes a migrated database and an attachments directory with a
// couple of files, including a half-written upload that must be skipped.
func newSource(t *testing.T) (Source, string) {
	t.Helper()
	data := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(data, "chrysalis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	att := filepath.Join(data, "attachments")
	writeFile(t, filepath.Join(att, "01", "01ABC"), "hello")
	writeFile(t, filepath.Join(att, "01", "01ABC.thumb"), "thumb")
	writeFile(t, filepath.Join(att, "01", ".tmp-123"), "partial")
	return Source{DB: st.DB, AttachmentsDir: att, Version: "test"}, data
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o640); err != nil {
		t.Fatal(err)
	}
}

func names(bs []Backup) []string {
	var out []string
	for _, b := range bs {
		out = append(out, b.Name)
	}
	return out
}

func TestCreate(t *testing.T) {
	src, _ := newSource(t)
	dir := filepath.Join(t.TempDir(), "backups")
	b, err := Create(context.Background(), src, dir, t0.Add(1500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "chrysalis-20261112T030001Z" || !b.Time.Equal(t0.Add(time.Second)) {
		t.Fatalf("backup = %+v", b)
	}
	entries, _ := os.ReadDir(b.Path)
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if want := []string{"attachments.tar.gz", "chrysalis.db", "manifest.json"}; !slices.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	if err := IntegrityCheck(context.Background(), filepath.Join(b.Path, "chrysalis.db")); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(b.Path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Attachments != 2 || m.Version != "test" || m.CreatedAt != "2026-11-12T03:00:01Z" || len(m.SHA256) != 2 {
		t.Fatalf("manifest = %+v", m)
	}
	if files := tarFiles(t, filepath.Join(b.Path, "attachments.tar.gz")); !slices.Equal(files, []string{"01/", "01/01ABC", "01/01ABC.thumb"}) {
		t.Fatalf("archive = %v", files)
	}

	// Nothing else is left in the backup dir, and List finds it.
	all, _ := os.ReadDir(dir)
	if len(all) != 1 {
		t.Fatalf("backup dir has %d entries, want 1", len(all))
	}
	listed, err := List(dir)
	if err != nil || !slices.Equal(names(listed), []string{b.Name}) {
		t.Fatalf("List = %v, %v", listed, err)
	}

	// The same second again is refused rather than overwritten.
	if _, err := Create(context.Background(), src, dir, t0.Add(time.Second)); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate err = %v", err)
	}
}

func tarFiles(t *testing.T, p string) []string {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var out []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, h.Name)
	}
}

func TestCreateSkipsSymlinks(t *testing.T) {
	src, _ := newSource(t)
	outside := filepath.Join(t.TempDir(), "secret")
	writeFile(t, outside, "not an attachment")
	if err := os.Symlink(outside, filepath.Join(src.AttachmentsDir, "01", "01LINK")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(src.AttachmentsDir, "zz")); err != nil {
		t.Fatal(err)
	}
	b, err := Create(context.Background(), src, t.TempDir(), t0)
	if err != nil {
		t.Fatal(err)
	}
	if files := tarFiles(t, filepath.Join(b.Path, "attachments.tar.gz")); !slices.Equal(files, []string{"01/", "01/01ABC", "01/01ABC.thumb"}) {
		t.Fatalf("archive = %v", files)
	}
}

func TestCreateWithoutAttachmentsDir(t *testing.T) {
	src, data := newSource(t)
	src.AttachmentsDir = filepath.Join(data, "missing")
	b, err := Create(context.Background(), src, t.TempDir(), t0)
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := readManifest(b.Path); m.Attachments != 0 {
		t.Fatalf("attachments = %d", m.Attachments)
	}
}

func TestCreateFailureLeavesNoBackup(t *testing.T) {
	src, _ := newSource(t)
	src.DB.Close() // VACUUM INTO will fail
	dir := t.TempDir()
	if _, err := Create(context.Background(), src, dir, t0); err == nil {
		t.Fatal("want an error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
}

func TestListIgnoresIncomplete(t *testing.T) {
	dir := t.TempDir()
	fake(t, dir, t0)
	fake(t, dir, t0.Add(time.Hour))
	if err := os.Mkdir(filepath.Join(dir, "chrysalis-20261112T050000Z"), 0o750); err != nil { // no manifest
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".tmp-1", "manifest.json"), "{}")
	writeFile(t, filepath.Join(dir, "notes.txt"), "x")
	writeFile(t, filepath.Join(dir, "chrysalis-garbage", "manifest.json"), "{}")
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"chrysalis-20261112T040000Z", "chrysalis-20261112T030000Z"}; !slices.Equal(names(got), want) {
		t.Fatalf("List = %v, want %v", names(got), want)
	}
	if got, err := List(filepath.Join(dir, "nope")); err != nil || got != nil {
		t.Fatalf("missing dir: %v, %v", got, err)
	}
}

// fake makes an empty but complete-looking backup for retention tests.
func fake(t *testing.T, dir string, at time.Time) {
	t.Helper()
	writeFile(t, filepath.Join(dir, namePrefix+at.UTC().Format(nameTimeFormat), manifestFile), "{}")
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	// One backup a day for 60 days, ending Thursday 2026-11-12, plus two
	// extra the last day (the newest of the day is the one kept).
	for i := range 60 {
		fake(t, dir, t0.AddDate(0, 0, -i))
	}
	fake(t, dir, t0.Add(-2*time.Hour))
	fake(t, dir, t0.Add(5*time.Hour))
	removed, err := Prune(dir, 7, 4)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := List(dir)
	want := []string{
		// 7 daily: Nov 12 (newest of that day) back to Nov 6.
		"chrysalis-20261112T080000Z",
		"chrysalis-20261111T030000Z",
		"chrysalis-20261110T030000Z",
		"chrysalis-20261109T030000Z",
		"chrysalis-20261108T030000Z", // Sunday: newest of week 45
		"chrysalis-20261107T030000Z",
		"chrysalis-20261106T030000Z",
		// Weekly: weeks 46 (Nov 12) and 45 (Nov 8) are already kept;
		// weeks 44 and 43 end on Sundays Nov 1 and Oct 25.
		"chrysalis-20261101T030000Z",
		"chrysalis-20261025T030000Z",
	}
	if !slices.Equal(names(left), want) {
		t.Fatalf("kept %v\nwant %v", names(left), want)
	}
	if len(removed) != 62-len(want) {
		t.Fatalf("removed %d, want %d", len(removed), 62-len(want))
	}

	// Pruning again changes nothing.
	if removed, err := Prune(dir, 7, 4); err != nil || len(removed) != 0 {
		t.Fatalf("second prune removed %v, %v", names(removed), err)
	}
}

func TestRetain(t *testing.T) {
	mk := func(ts ...string) []Backup {
		var out []Backup
		for _, s := range ts {
			tm, _ := time.Parse(time.RFC3339, s)
			out = append(out, Backup{Name: s, Time: tm})
		}
		return out
	}
	tests := []struct {
		name          string
		all           []Backup
		daily, weekly int
		want          []string
	}{
		{"none", nil, 7, 4, nil},
		{"one", mk("2026-11-12T03:00:00Z"), 1, 0, []string{"2026-11-12T03:00:00Z"}},
		{
			"gaps count days that have backups, not calendar days",
			mk("2026-11-12T03:00:00Z", "2026-11-01T03:00:00Z", "2026-10-01T03:00:00Z", "2026-09-01T03:00:00Z"),
			2, 0,
			[]string{"2026-11-12T03:00:00Z", "2026-11-01T03:00:00Z"},
		},
		{
			"newest is always kept",
			mk("2026-11-12T03:00:00Z", "2026-11-11T03:00:00Z"),
			0, 0,
			[]string{"2026-11-12T03:00:00Z"},
		},
		{
			"weekly reaches past daily",
			mk("2026-11-12T03:00:00Z", "2026-11-11T03:00:00Z", "2026-11-04T03:00:00Z", "2026-10-28T03:00:00Z"),
			1, 3,
			[]string{"2026-11-12T03:00:00Z", "2026-11-04T03:00:00Z", "2026-10-28T03:00:00Z"},
		},
		{
			"ISO week spans a year boundary",
			mk("2027-01-02T03:00:00Z", "2026-12-28T03:00:00Z", "2026-12-27T03:00:00Z"),
			1, 2,
			// Dec 28 2026 to Jan 3 2027 is 2026-W53; Dec 27 is in W52.
			[]string{"2027-01-02T03:00:00Z", "2026-12-27T03:00:00Z"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keep := retain(tt.all, tt.daily, tt.weekly)
			var got []string
			for _, b := range tt.all {
				if keep[b.Name] {
					got = append(got, b.Name)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("kept %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDue(t *testing.T) {
	dir := t.TempDir()
	if due, err := Due(dir, t0); err != nil || !due {
		t.Fatalf("empty: due = %v, %v", due, err)
	}
	fake(t, dir, t0)
	if due, _ := Due(dir, t0.Add(21*time.Hour)); !due {
		t.Fatal("want due on the next UTC day")
	}
	if due, _ := Due(dir, t0.Add(20*time.Hour+59*time.Minute)); due {
		t.Fatal("want not due later the same UTC day")
	}
}

func TestRemoveStaleTemps(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, ".tmp-old")
	young := filepath.Join(dir, ".tmp-young")
	for _, p := range []string{old, young} {
		if err := os.Mkdir(p, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(old, t0.Add(-48*time.Hour), t0.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(young, t0.Add(-time.Hour), t0.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	removeStaleTemps(dir, t0)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("stale temp dir kept")
	}
	if _, err := os.Stat(young); err != nil {
		t.Error("recent temp dir removed")
	}
}
