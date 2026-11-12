// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

const (
	MaxChecklistTitleLen = 200
	MaxItemTextLen       = 1000
)

type Checklist struct {
	ID       string          `json:"id"`
	CardID   string          `json:"cardId"`
	Title    string          `json:"title"`
	Position string          `json:"position"`
	Items    []ChecklistItem `json:"items"`
	Version  int64           `json:"version"`
}

type ChecklistItem struct {
	ID          string  `json:"id"`
	ChecklistID string  `json:"checklistId"`
	Text        string  `json:"text"`
	DoneAt      *string `json:"doneAt"`
	Position    string  `json:"position"`
	Version     int64   `json:"version"`
}

func checklistFromRow(c db.Checklist) Checklist {
	return Checklist{ID: c.ID, CardID: c.CardID, Title: c.Title, Position: c.Position, Items: []ChecklistItem{}, Version: c.Version}
}

func itemFromRow(i db.ChecklistItem) ChecklistItem {
	return ChecklistItem{
		ID: i.ID, ChecklistID: i.ChecklistID, Text: i.Text, DoneAt: nullStr(i.DoneAt),
		Position: i.Position, Version: i.Version,
	}
}

// liveCard loads a card that isn't trashed and checks the user's role on
// its board.
func liveCard(ctx context.Context, q *db.Queries, userID, cardID string, write bool) (db.Card, error) {
	c, err := q.GetCard(ctx, cardID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && c.DeletedAt.Valid) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	_, err = role(ctx, q, userID, c.BoardID, write)
	return c, err
}

// liveChecklist loads a checklist that isn't trashed, plus its card, with a
// role check.
func liveChecklist(ctx context.Context, q *db.Queries, userID, checklistID string, write bool) (db.Checklist, error) {
	cl, err := q.GetChecklist(ctx, checklistID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && cl.DeletedAt.Valid) {
		return cl, ErrNotFound
	}
	if err != nil {
		return cl, err
	}
	_, err = liveCard(ctx, q, userID, cl.CardID, write)
	return cl, err
}

func cleanText(s, what string, max int, allowEmpty bool) (string, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "" && !allowEmpty:
		return "", &ValidationError{fmt.Sprintf("The %s can't be empty.", what)}
	case utf8.RuneCountInString(s) > max:
		return "", &ValidationError{fmt.Sprintf("That %s is too long (up to %d characters).", what, max)}
	}
	return s, nil
}

// Checklists returns a card's checklists with their items.
func (s *Boards) Checklists(ctx context.Context, userID, cardID string) ([]Checklist, error) {
	if _, err := liveCard(ctx, s.st.Queries, userID, cardID, false); err != nil {
		return nil, err
	}
	lists, err := s.st.ListChecklists(ctx, cardID)
	if err != nil {
		return nil, err
	}
	items, err := s.st.ListChecklistItemsForCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	out := make([]Checklist, 0, len(lists))
	idx := map[string]int{}
	for i, l := range lists {
		idx[l.ID] = i
		out = append(out, checklistFromRow(l))
	}
	for _, it := range items {
		if i, ok := idx[it.ChecklistID]; ok {
			out[i].Items = append(out[i].Items, itemFromRow(it))
		}
	}
	return out, nil
}

func (s *Boards) CreateChecklist(ctx context.Context, userID, cardID, title string) (Checklist, error) {
	title, err := cleanText(title, "checklist title", MaxChecklistTitleLen, true)
	if err != nil {
		return Checklist{}, err
	}
	if title == "" {
		title = "Checklist"
	}
	var out Checklist
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := liveCard(ctx, q, userID, cardID, true); err != nil {
			return err
		}
		pos, err := positionOrAppend(nil, func() (string, error) { return q.LastChecklistPosition(ctx, cardID) })
		if err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		row, err := q.CreateChecklist(ctx, db.CreateChecklistParams{
			ID: ulid.Make().String(), CardID: cardID, Title: title, Position: pos, CreatedAt: now, UpdatedAt: now,
		})
		out = checklistFromRow(row)
		return err
	})
	return out, err
}

type ChecklistPatch struct {
	Title    *string `json:"title"`
	Position *string `json:"position"`
}

func (s *Boards) UpdateChecklist(ctx context.Context, userID, checklistID string, p ChecklistPatch) (Checklist, error) {
	var out Checklist
	err := s.tx(ctx, func(q *db.Queries) error {
		cl, err := liveChecklist(ctx, q, userID, checklistID, true)
		if err != nil {
			return err
		}
		params := db.UpdateChecklistParams{ID: cl.ID, Title: cl.Title, Position: cl.Position, UpdatedAt: store.FormatTime(s.now())}
		if p.Title != nil {
			if params.Title, err = cleanText(*p.Title, "checklist title", MaxChecklistTitleLen, false); err != nil {
				return err
			}
		}
		if p.Position != nil {
			if order.Valid(*p.Position) != nil {
				return &ValidationError{"invalid position"}
			}
			params.Position = *p.Position
		}
		row, err := q.UpdateChecklist(ctx, params)
		out = checklistFromRow(row)
		return err
	})
	return out, err
}

func (s *Boards) SetChecklistTrashed(ctx context.Context, userID, checklistID string, trashed bool) (Checklist, error) {
	var out Checklist
	err := s.tx(ctx, func(q *db.Queries) error {
		cl, err := q.GetChecklist(ctx, checklistID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := liveCard(ctx, q, userID, cl.CardID, true); err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		del := sql.NullString{}
		if trashed {
			del = sql.NullString{String: now, Valid: true}
		}
		row, err := q.SetChecklistDeleted(ctx, db.SetChecklistDeletedParams{ID: cl.ID, DeletedAt: del, UpdatedAt: now})
		out = checklistFromRow(row)
		return err
	})
	return out, err
}

func (s *Boards) CreateChecklistItem(ctx context.Context, userID, checklistID, text string, pos *string) (ChecklistItem, error) {
	text, err := cleanText(text, "checklist item", MaxItemTextLen, false)
	if err != nil {
		return ChecklistItem{}, err
	}
	var out ChecklistItem
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := liveChecklist(ctx, q, userID, checklistID, true); err != nil {
			return err
		}
		p, err := positionOrAppend(pos, func() (string, error) { return q.LastChecklistItemPosition(ctx, checklistID) })
		if err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		row, err := q.CreateChecklistItem(ctx, db.CreateChecklistItemParams{
			ID: ulid.Make().String(), ChecklistID: checklistID, Text: text, Position: p, CreatedAt: now, UpdatedAt: now,
		})
		out = itemFromRow(row)
		return err
	})
	return out, err
}

type ChecklistItemPatch struct {
	Text        *string `json:"text"`
	Done        *bool   `json:"done"`
	Position    *string `json:"position"`
	ChecklistID *string `json:"checklistId"` // another checklist on the same card
}

func (s *Boards) UpdateChecklistItem(ctx context.Context, userID, itemID string, p ChecklistItemPatch) (ChecklistItem, error) {
	var out ChecklistItem
	err := s.tx(ctx, func(q *db.Queries) error {
		it, err := q.GetChecklistItem(ctx, itemID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && it.DeletedAt.Valid) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		cl, err := liveChecklist(ctx, q, userID, it.ChecklistID, true)
		if err != nil {
			return err
		}
		params := db.UpdateChecklistItemParams{
			ID: it.ID, ChecklistID: it.ChecklistID, Text: it.Text, DoneAt: it.DoneAt, Position: it.Position,
			UpdatedAt: store.FormatTime(s.now()),
		}
		if p.Text != nil {
			if params.Text, err = cleanText(*p.Text, "checklist item", MaxItemTextLen, false); err != nil {
				return err
			}
		}
		if p.Done != nil {
			switch {
			case *p.Done && !it.DoneAt.Valid:
				params.DoneAt = sql.NullString{String: params.UpdatedAt, Valid: true}
			case !*p.Done:
				params.DoneAt = sql.NullString{}
			}
		}
		if p.ChecklistID != nil && *p.ChecklistID != it.ChecklistID {
			target, err := liveChecklist(ctx, q, userID, *p.ChecklistID, true)
			if err != nil {
				return err
			}
			if target.CardID != cl.CardID {
				return &ValidationError{"Items can only move between checklists on the same card."}
			}
			params.ChecklistID = target.ID
			if p.Position == nil {
				last, err := q.LastChecklistItemPosition(ctx, target.ID)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				if params.Position, err = order.KeyBetween(last, ""); err != nil {
					return err
				}
			}
		}
		if p.Position != nil {
			if order.Valid(*p.Position) != nil {
				return &ValidationError{"invalid position"}
			}
			params.Position = *p.Position
		}
		row, err := q.UpdateChecklistItem(ctx, params)
		out = itemFromRow(row)
		return err
	})
	return out, err
}

func (s *Boards) SetChecklistItemTrashed(ctx context.Context, userID, itemID string, trashed bool) (ChecklistItem, error) {
	var out ChecklistItem
	err := s.tx(ctx, func(q *db.Queries) error {
		it, err := q.GetChecklistItem(ctx, itemID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		cl, err := q.GetChecklist(ctx, it.ChecklistID)
		if err != nil {
			return err
		}
		if _, err := liveCard(ctx, q, userID, cl.CardID, true); err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		del := sql.NullString{}
		if trashed {
			del = sql.NullString{String: now, Valid: true}
		}
		row, err := q.SetChecklistItemDeleted(ctx, db.SetChecklistItemDeletedParams{ID: it.ID, DeletedAt: del, UpdatedAt: now})
		out = itemFromRow(row)
		return err
	})
	return out, err
}
