// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

func newImportStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	u, err := st.UpsertUser(context.Background(), db.UpsertUserParams{
		ID: "01JALICE000000000000000000", OidcSub: "alice", Email: "alice@example.com", DisplayName: "Alice",
		CreatedAt: "2026-01-01T00:00:00.000Z", LastSeenAt: "2026-01-01T00:00:00.000Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, u.ID
}

func sampleImport() ImportBoard {
	due, start := "2026-03-15T12:00:00-05:00", "2026-03-01"
	doc := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`)
	return ImportBoard{
		Name:       "Garden",
		Background: &Background{Type: "gradient", Value: "meadow"},
		Labels:     []ImportLabel{{Name: "Veg", Color: "green"}, {Name: "", Color: "gray"}},
		Columns: []ImportColumn{
			{Name: "Old", Collapsed: true, Cards: []ImportCard{{Title: "Garlic", Archived: true}}},
			{Name: "Now", Cards: []ImportCard{
				{Title: "Tomatoes", Description: doc, StartAt: &start, DueAt: &due, CoverColor: "red", Labels: []int{0, 1, 0},
					Checklists: []ImportChecklist{{Title: "Steps", Items: []ImportItem{{Text: "Soak", Done: true}, {Text: "Sow"}}}},
					Comments:   []ImportComment{{Body: doc, CreatedAt: "2025-12-01T10:00:00Z"}, {Body: doc, CreatedAt: "garbage"}},
					Links:      []ImportLink{{URL: "https://example.com/a", Title: ""}}},
				{Title: "Basil", Completed: true},
			}},
		},
	}
}

func TestImportWritesEverything(t *testing.T) {
	ctx := context.Background()
	st, alice := newImportStore(t)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	s := NewBoards(st, func() time.Time { return now })

	b, n, err := s.Import(ctx, alice, sampleImport())
	if err != nil {
		t.Fatal(err)
	}
	if n != (ImportCounts{Columns: 2, Cards: 3, Descriptions: 1, Labels: 2, Checklists: 1, ChecklistItems: 2, Comments: 2, Links: 1}) {
		t.Errorf("counts = %+v", n)
	}
	if b.Background != (Background{Type: "gradient", Value: "meadow"}) || b.Role != RoleOwner {
		t.Errorf("board = %+v", b)
	}
	d, err := s.Detail(ctx, alice, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Columns) != 2 || d.Columns[0].Name != "Old" || !d.View.Columns[d.Columns[0].ID] {
		t.Errorf("columns = %+v view %+v", d.Columns, d.View.Columns)
	}
	if len(d.Cards) != 2 || d.Cards[0].Title != "Tomatoes" || d.Cards[1].Title != "Basil" || d.Cards[0].Position >= d.Cards[1].Position {
		t.Fatalf("cards = %+v", d.Cards)
	}
	tom := d.Cards[0]
	if *tom.DueAt != "2026-03-15T17:00:00.000Z" || len(tom.LabelIDs) != 2 || tom.Cover.Value != "red" || tom.ChecklistDone != 1 {
		t.Errorf("tomatoes = %+v", tom)
	}
	if d.Cards[1].CompletedAt == nil || *d.Cards[1].CompletedAt != store.FormatTime(now) {
		t.Errorf("basil = %+v", d.Cards[1])
	}
	cs, err := s.Comments(ctx, alice, tom.ID)
	if err != nil || len(cs) != 2 {
		t.Fatalf("comments = %+v, %v", cs, err)
	}
	// The original date is kept; one that can't be read becomes the import time.
	if cs[0].CreatedAt != "2025-12-01T10:00:00.000Z" || cs[1].CreatedAt != store.FormatTime(now) || cs[0].AuthorName != "Alice" {
		t.Errorf("comments = %+v", cs)
	}
}

// A failed import leaves nothing behind.
func TestImportIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	st, alice := newImportStore(t)
	s := NewBoards(st, time.Now)

	bad := []func(*ImportBoard){
		func(b *ImportBoard) { b.Columns[1].Cards[1].CoverColor = "chartreuse" },
		func(b *ImportBoard) { b.Columns[1].Cards[0].Links[0].URL = "javascript:alert(1)" },
		func(b *ImportBoard) { b.Columns[1].Cards[0].Labels = []int{7} },
		func(b *ImportBoard) { b.Columns[1].Cards[0].Description = json.RawMessage(`{"type":"nope"}`) },
		func(b *ImportBoard) { b.Columns[1].Cards[0].Checklists[0].Items[1].Text = " " },
		func(b *ImportBoard) { b.Labels[0].Color = "teal" },
		func(b *ImportBoard) { b.Columns[1].Name = "" },
		func(b *ImportBoard) { b.Background = &Background{Type: "image", Value: "x"} },
		func(b *ImportBoard) { b.Background = &Background{Type: "color", Value: "red"} },
		func(b *ImportBoard) { s := "2026-04-01"; b.Columns[1].Cards[0].StartAt = &s },
		func(b *ImportBoard) { b.Columns = make([]ImportColumn, MaxImportColumns+1) },
	}
	for i, breakIt := range bad {
		b := sampleImport()
		breakIt(&b)
		_, _, err := s.Import(ctx, alice, b)
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Errorf("case %d: err = %v, want a validation error", i, err)
		}
	}
	for _, table := range []string{"boards", "board_members", "columns", "cards", "labels", "card_labels", "checklists", "checklist_items", "comments", "attachments", "card_descriptions", "user_column_view"} {
		var count int
		if err := st.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s has %d rows after failed imports", table, count)
		}
	}
}
