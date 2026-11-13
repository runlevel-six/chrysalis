// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/sse"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// Custom fields (docs/DESIGN.md §4, Phase 2) are defined per board: text,
// number, date, single choice or checkbox. Each card can have a value for
// each. A field can show on collapsed cards as a small chip; that's off by
// default, like everything else on collapsed cards (§3.2).

// CustomField is one field definition.
type CustomField struct {
	ID              string        `json:"id"`
	BoardID         string        `json:"boardId"`
	Name            string        `json:"name"`
	Type            string        `json:"type"` // text | number | date | select | checkbox
	Options         []FieldOption `json:"options"`
	ShowOnCollapsed bool          `json:"showOnCollapsed"`
	Position        string        `json:"position"`
	Version         int64         `json:"version"`
}

// FieldOption is one choice of a select field.
type FieldOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"` // a label palette key, or ""
}

const (
	MaxFieldNameLen   = 60
	MaxFieldsPerBoard = 30
	MaxFieldOptions   = 50
	MaxFieldTextLen   = 500
)

var fieldTypes = []string{"text", "number", "date", "select", "checkbox"}

func fieldFromRow(r db.CustomField) CustomField {
	f := CustomField{ID: r.ID, BoardID: r.BoardID, Name: r.Name, Type: r.Type, Options: []FieldOption{},
		ShowOnCollapsed: r.ShowOnCollapsed != 0, Position: r.Position, Version: r.Version}
	_ = json.Unmarshal([]byte(r.OptionsJson), &f.Options)
	if f.Options == nil {
		f.Options = []FieldOption{}
	}
	return f
}

// cleanOptions validates a select field's choices, giving new ones ids.
func cleanOptions(opts []FieldOption) ([]FieldOption, error) {
	if len(opts) > MaxFieldOptions {
		return nil, &ValidationError{"That's more choices than a field can hold (up to 50)."}
	}
	out := make([]FieldOption, 0, len(opts))
	seen := map[string]bool{}
	for _, o := range opts {
		name := strings.TrimSpace(o.Name)
		if name == "" {
			return nil, &ValidationError{"Give each choice a name."}
		}
		if utf8.RuneCountInString(name) > MaxFieldNameLen {
			return nil, &ValidationError{"That choice's name is too long (up to 60 characters)."}
		}
		if o.Color != "" && !slices.Contains(LabelColors, o.Color) {
			return nil, &ValidationError{"That color isn't one of the palette's."}
		}
		id := o.ID
		if id == "" || len(id) > 32 || seen[id] {
			id = strings.ToLower(ulid.Make().String())
		}
		seen[id] = true
		out = append(out, FieldOption{ID: id, Name: name, Color: o.Color})
	}
	return out, nil
}

// Fields lists a board's live field definitions in order.
func listFields(ctx context.Context, q *db.Queries, boardID string) ([]CustomField, error) {
	rows, err := q.ListFields(ctx, boardID)
	if err != nil {
		return nil, err
	}
	out := make([]CustomField, 0, len(rows))
	for _, r := range rows {
		out = append(out, fieldFromRow(r))
	}
	return out, nil
}

// CreateField adds a field to a board, after its others.
func (s *Boards) CreateField(ctx context.Context, userID, boardID, name, typ string, options []FieldOption) (CustomField, error) {
	name, err := cleanName(name, "field", MaxFieldNameLen)
	if err != nil {
		return CustomField{}, err
	}
	if !slices.Contains(fieldTypes, typ) {
		return CustomField{}, &ValidationError{"type must be text, number, date, select or checkbox"}
	}
	if typ != "select" {
		options = nil
	}
	opts, err := cleanOptions(options)
	if err != nil {
		return CustomField{}, err
	}
	var out CustomField
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := role(ctx, q, userID, boardID, true); err != nil {
			return err
		}
		if n, err := q.CountFields(ctx, boardID); err != nil {
			return err
		} else if n >= MaxFieldsPerBoard {
			return &ValidationError{"A board can have up to 30 fields."}
		}
		last, err := q.LastFieldPosition(ctx, boardID)
		if errors.Is(err, sql.ErrNoRows) {
			last = ""
		} else if err != nil {
			return err
		}
		pos := order.First()
		if last != "" {
			if pos, err = order.KeyBetween(last, ""); err != nil {
				return err
			}
		}
		raw, _ := json.Marshal(opts)
		now := store.FormatTime(s.now())
		row, err := q.CreateField(ctx, db.CreateFieldParams{
			ID: ulid.Make().String(), BoardID: boardID, Name: name, Type: typ, OptionsJson: string(raw),
			Position: pos, CreatedAt: now, UpdatedAt: now,
		})
		out = fieldFromRow(row)
		return err
	})
	if err == nil {
		s.emit(ctx, sse.Event{BoardID: boardID, Kind: "field", EntityID: out.ID})
	}
	return out, err
}

// FieldPatch changes a field; nil leaves things as they are. The type can't
// change (values of one type don't mean anything as another).
type FieldPatch struct {
	Name            *string        `json:"name"`
	Options         *[]FieldOption `json:"options"`
	ShowOnCollapsed *bool          `json:"showOnCollapsed"`
	Position        *string        `json:"position"`
}

func liveField(ctx context.Context, q *db.Queries, userID, fieldID string, write bool) (db.CustomField, error) {
	f, err := q.GetField(ctx, fieldID)
	if errors.Is(err, sql.ErrNoRows) {
		return f, ErrNotFound
	}
	if err != nil {
		return f, err
	}
	if _, err := role(ctx, q, userID, f.BoardID, write); err != nil {
		return f, err
	}
	return f, nil
}

func (s *Boards) UpdateField(ctx context.Context, userID, fieldID string, p FieldPatch) (CustomField, error) {
	var out CustomField
	err := s.tx(ctx, func(q *db.Queries) error {
		f, err := liveField(ctx, q, userID, fieldID, true)
		if err != nil {
			return err
		}
		if f.DeletedAt.Valid {
			return ErrNotFound
		}
		params := db.UpdateFieldParams{ID: f.ID, Name: f.Name, OptionsJson: f.OptionsJson, ShowOnCollapsed: f.ShowOnCollapsed,
			Position: f.Position, UpdatedAt: store.FormatTime(s.now())}
		if p.Name != nil {
			if params.Name, err = cleanName(*p.Name, "field", MaxFieldNameLen); err != nil {
				return err
			}
		}
		if p.Options != nil {
			if f.Type != "select" {
				return &ValidationError{"Only choice fields have choices."}
			}
			opts, err := cleanOptions(*p.Options)
			if err != nil {
				return err
			}
			raw, _ := json.Marshal(opts)
			params.OptionsJson = string(raw)
			// A value whose choice was removed is cleared.
			keep := map[string]bool{}
			for _, o := range opts {
				keep[o.ID] = true
			}
			vals, err := q.ListFieldValues(ctx, f.ID)
			if err != nil {
				return err
			}
			for _, v := range vals {
				var id string
				if json.Unmarshal([]byte(v.ValueJson), &id) != nil || !keep[id] {
					if err := q.DeleteFieldValue(ctx, db.DeleteFieldValueParams{CardID: v.CardID, FieldID: f.ID}); err != nil {
						return err
					}
				}
			}
		}
		if p.ShowOnCollapsed != nil {
			params.ShowOnCollapsed = 0
			if *p.ShowOnCollapsed {
				params.ShowOnCollapsed = 1
			}
		}
		if p.Position != nil {
			if err := order.Valid(*p.Position); err != nil {
				return &ValidationError{"invalid position"}
			}
			params.Position = *p.Position
		}
		row, err := q.UpdateField(ctx, params)
		out = fieldFromRow(row)
		return err
	})
	if err == nil {
		s.emit(ctx, sse.Event{BoardID: out.BoardID, Kind: "field", EntityID: out.ID})
	}
	return out, err
}

// SetFieldTrashed moves a field (and so its values) to the trash, or back.
func (s *Boards) SetFieldTrashed(ctx context.Context, userID, fieldID string, trashed bool) (CustomField, error) {
	var out CustomField
	err := s.tx(ctx, func(q *db.Queries) error {
		f, err := liveField(ctx, q, userID, fieldID, true)
		if err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		deleted := sql.NullString{}
		if trashed {
			deleted = sql.NullString{String: now, Valid: true}
		}
		row, err := q.SetFieldDeleted(ctx, db.SetFieldDeletedParams{DeletedAt: deleted, UpdatedAt: now, ID: f.ID})
		out = fieldFromRow(row)
		return err
	})
	if err == nil {
		s.emit(ctx, sse.Event{BoardID: out.BoardID, Kind: "field", EntityID: out.ID})
	}
	return out, err
}

// cleanValue checks a value against its field's type. null (or an empty
// text) means "no value".
func cleanValue(f db.CustomField, raw json.RawMessage) (string, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", false, nil
	}
	bad := func(msg string) (string, bool, error) { return "", false, &ValidationError{msg} }
	switch f.Type {
	case "text":
		var v string
		if json.Unmarshal(raw, &v) != nil {
			return bad("This field takes text.")
		}
		v = strings.TrimSpace(v)
		if v == "" {
			return "", false, nil
		}
		if utf8.RuneCountInString(v) > MaxFieldTextLen {
			return bad("That's longer than a field holds (up to 500 characters).")
		}
		b, _ := json.Marshal(v)
		return string(b), true, nil
	case "number":
		var v float64
		if json.Unmarshal(raw, &v) != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e15 {
			return bad("This field takes a number.")
		}
		b, _ := json.Marshal(v)
		return string(b), true, nil
	case "date":
		var v string
		if json.Unmarshal(raw, &v) != nil {
			return bad("This field takes a date.")
		}
		if _, err := time.Parse(dateOnly, v); err != nil {
			return bad("This field takes a date.")
		}
		b, _ := json.Marshal(v)
		return string(b), true, nil
	case "checkbox":
		var v bool
		if json.Unmarshal(raw, &v) != nil {
			return bad("This field is ticked or not.")
		}
		if !v {
			return "", false, nil // unticked is the same as no value
		}
		return "true", true, nil
	case "select":
		var v string
		if json.Unmarshal(raw, &v) != nil {
			return bad("Choose one of the field's choices.")
		}
		var opts []FieldOption
		_ = json.Unmarshal([]byte(f.OptionsJson), &opts)
		for _, o := range opts {
			if o.ID == v {
				b, _ := json.Marshal(v)
				return string(b), true, nil
			}
		}
		return bad("Choose one of the field's choices.")
	}
	return bad("unknown field type")
}

// SetCardField sets (or clears, with null) a card's value for a field.
func (s *Boards) SetCardField(ctx context.Context, userID, cardID, fieldID string, value json.RawMessage) (Card, error) {
	var out Card
	err := s.tx(ctx, func(q *db.Queries) error {
		c, err := liveCard(ctx, q, userID, cardID, true)
		if err != nil {
			return err
		}
		f, err := q.GetField(ctx, fieldID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && (f.DeletedAt.Valid || f.BoardID != c.BoardID)) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		v, set, err := cleanValue(f, value)
		if err != nil {
			return err
		}
		if set {
			err = q.UpsertFieldValue(ctx, db.UpsertFieldValueParams{CardID: c.ID, FieldID: f.ID, ValueJson: v, UpdatedAt: store.FormatTime(s.now())})
		} else {
			err = q.DeleteFieldValue(ctx, db.DeleteFieldValueParams{CardID: c.ID, FieldID: f.ID})
		}
		if err != nil {
			return err
		}
		out, err = withLabels(ctx, q, cardFromRow(c))
		return err
	})
	if err == nil {
		s.emit(ctx, sse.Event{BoardID: out.BoardID, Kind: "card", EntityID: out.ID, CardID: out.ID})
	}
	return out, err
}
