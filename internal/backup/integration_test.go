// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package backup_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/backup"
	"github.com/runlevel-six/chrysalis/internal/service"
	"github.com/runlevel-six/chrysalis/internal/storage"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// instance is a data directory opened the way the server opens it.
type instance struct {
	dir         string
	st          *store.Store
	boards      *service.Boards
	attachments *service.Attachments
}

func open(t *testing.T, dir string) *instance {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(dir, "chrysalis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	files, err := storage.NewLocal(filepath.Join(dir, "attachments"))
	if err != nil {
		t.Fatal(err)
	}
	boards := service.NewBoards(st, time.Now)
	return &instance{dir: dir, st: st, boards: boards, attachments: service.NewAttachments(boards, files)}
}

var tables = []string{"users", "boards", "board_members", "columns", "cards", "attachments"}

func (in *instance) counts(t *testing.T) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range tables {
		var n int
		if err := in.st.DB.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[table] = n
	}
	return out
}

func (in *instance) download(t *testing.T, userID, attachmentID string) []byte {
	t.Helper()
	_, f, err := in.attachments.Open(context.Background(), userID, attachmentID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestBackupAndRestore is the tested restore that docs/DESIGN.md §7.4
// requires: fill an instance, back it up while it's open, restore into a
// fresh data directory, and check everything came back.
func TestBackupAndRestore(t *testing.T) {
	ctx := context.Background()
	src := open(t, t.TempDir())

	users := auth.NewUsers(src.st.Queries, time.Now)
	alice, err := users.Resolve(ctx, auth.Identity{Subject: "alice", Email: "alice@example.com", Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := users.Resolve(ctx, auth.Identity{Subject: "bob", Email: "bob@example.com", Name: "Bob"})
	if err != nil {
		t.Fatal(err)
	}
	board, err := src.boards.Create(ctx, alice.ID, "Household")
	if err != nil {
		t.Fatal(err)
	}
	if err := src.st.AddBoardMember(ctx, db.AddBoardMemberParams{BoardID: board.ID, UserID: bob.ID, Role: "editor"}); err != nil {
		t.Fatal(err)
	}
	col, err := src.boards.CreateColumn(ctx, alice.ID, board.ID, "Groceries", nil)
	if err != nil {
		t.Fatal(err)
	}
	var cardID string
	for _, title := range []string{"Milk", "Bread", "Apples"} {
		c, err := src.boards.CreateCard(ctx, bob.ID, col.ID, title, nil)
		if err != nil {
			t.Fatal(err)
		}
		cardID = c.ID
	}
	content := bytes.Repeat([]byte("synthetic attachment bytes\n"), 1000)
	att, err := src.attachments.Upload(ctx, alice.ID, cardID, "list.txt", content)
	if err != nil {
		t.Fatal(err)
	}
	want := src.counts(t)

	dir := filepath.Join(t.TempDir(), "backups")
	b, _, err := backup.Run(ctx, backup.Source{
		DB: src.st.DB, AttachmentsDir: filepath.Join(src.dir, "attachments"), Version: "test",
	}, backup.Policy{Dir: dir, KeepDaily: 7, KeepWeekly: 4}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// A change after the backup isn't in it.
	if _, err := src.boards.CreateCard(ctx, alice.ID, col.ID, "After the backup", nil); err != nil {
		t.Fatal(err)
	}

	fresh := t.TempDir()
	r, err := backup.Restore(ctx, b.Path, backup.Target{
		DBPath: filepath.Join(fresh, "chrysalis.db"), AttachmentsDir: filepath.Join(fresh, "attachments"),
	}, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Attachments != 1 {
		t.Errorf("restored %d attachment files, want 1", r.Attachments)
	}

	dst := open(t, fresh)
	if err := dst.st.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	got := dst.counts(t)
	for _, table := range tables {
		if got[table] != want[table] {
			t.Errorf("%s: %d rows, want %d", table, got[table], want[table])
		}
	}
	if want["cards"] != 3 || want["users"] != 2 {
		t.Errorf("unexpected source counts %v", want)
	}
	detail, err := dst.boards.Detail(ctx, bob.ID, board.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Columns) != 1 || detail.Columns[0].Name != "Groceries" {
		t.Errorf("columns = %+v", detail.Columns)
	}
	if data := dst.download(t, bob.ID, att.ID); !bytes.Equal(data, content) {
		t.Errorf("attachment: got %d bytes, want %d identical bytes", len(data), len(content))
	}
}
