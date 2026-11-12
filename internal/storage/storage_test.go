// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, "01ABCDEF.thumb", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	f, err := s.Open(ctx, "01ABCDEF.thumb")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "hello" {
		t.Fatalf("read %q", b)
	}
	// No temp files left behind.
	entries, _ := os.ReadDir(filepath.Join(dir, "01"))
	if len(entries) != 1 {
		t.Errorf("files in shard: %v", entries)
	}
	if err := s.Delete(ctx, "01ABCDEF.thumb"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, "01ABCDEF.thumb"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
	if err := s.Delete(ctx, "01ABCDEF.thumb"); err != nil {
		t.Errorf("deleting twice should be fine: %v", err)
	}
}

func TestLocalRejectsBadKeys(t *testing.T) {
	ctx := context.Background()
	s, _ := NewLocal(t.TempDir())
	for _, key := range []string{"", "a", "../etc/passwd", "01/../../x", ".hidden", "with space", "slash/inside"} {
		if err := s.Put(ctx, key, []byte("x")); err == nil {
			t.Errorf("Put(%q) succeeded", key)
		}
		if _, err := s.Open(ctx, key); err == nil {
			t.Errorf("Open(%q) succeeded", key)
		}
	}
}
