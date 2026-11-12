// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// LabelColors are the palette keys a label can use. Each theme maps them to
// real colors that meet contrast requirements (docs/DESIGN.md §3.12), so a
// label stays readable whatever theme is active.
var LabelColors = []string{"green", "lime", "yellow", "orange", "red", "pink", "purple", "blue", "sky", "gray"}

const MaxLabelNameLen = 50

type Label struct {
	ID       string `json:"id"`
	BoardID  string `json:"boardId"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Position string `json:"position"`
	Version  int64  `json:"version"`
}

func labelFromRow(l db.Label) Label {
	return Label{ID: l.ID, BoardID: l.BoardID, Name: l.Name, Color: l.Color, Position: l.Position, Version: l.Version}
}

func cleanLabel(name, color string) (string, string, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > MaxLabelNameLen {
		return "", "", &ValidationError{fmt.Sprintf("Label names can be up to %d characters.", MaxLabelNameLen)}
	}
	if !slices.Contains(LabelColors, color) {
		return "", "", &ValidationError{"Pick one of the label colors."}
	}
	return name, color, nil
}

func (s *Boards) CreateLabel(ctx context.Context, userID, boardID, name, color string) (Label, error) {
	name, color, err := cleanLabel(name, color)
	if err != nil {
		return Label{}, err
	}
	var out Label
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := role(ctx, q, userID, boardID, true); err != nil {
			return err
		}
		pos, err := positionOrAppend(nil, func() (string, error) { return q.LastLabelPosition(ctx, boardID) })
		if err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		row, err := q.CreateLabel(ctx, db.CreateLabelParams{
			ID: ulid.Make().String(), BoardID: boardID, Name: name, Color: color, Position: pos,
			CreatedAt: now, UpdatedAt: now,
		})
		out = labelFromRow(row)
		return err
	})
	return out, err
}

type LabelPatch struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func (s *Boards) UpdateLabel(ctx context.Context, userID, labelID string, p LabelPatch) (Label, error) {
	var out Label
	err := s.tx(ctx, func(q *db.Queries) error {
		l, err := q.GetLabel(ctx, labelID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && l.DeletedAt.Valid) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := role(ctx, q, userID, l.BoardID, true); err != nil {
			return err
		}
		name, color := l.Name, l.Color
		if p.Name != nil {
			name = *p.Name
		}
		if p.Color != nil {
			color = *p.Color
		}
		if name, color, err = cleanLabel(name, color); err != nil {
			return err
		}
		row, err := q.UpdateLabel(ctx, db.UpdateLabelParams{
			ID: l.ID, Name: name, Color: color, Position: l.Position, UpdatedAt: store.FormatTime(s.now()),
		})
		out = labelFromRow(row)
		return err
	})
	return out, err
}

// SetLabelTrashed moves a label to the trash (cards stop showing it) or back.
// Card assignments are kept, so restoring puts it back on the same cards.
func (s *Boards) SetLabelTrashed(ctx context.Context, userID, labelID string, trashed bool) (Label, error) {
	var out Label
	err := s.tx(ctx, func(q *db.Queries) error {
		l, err := q.GetLabel(ctx, labelID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := role(ctx, q, userID, l.BoardID, true); err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		del := sql.NullString{}
		if trashed {
			del = sql.NullString{String: now, Valid: true}
		}
		row, err := q.SetLabelDeleted(ctx, db.SetLabelDeletedParams{ID: l.ID, DeletedAt: del, UpdatedAt: now})
		out = labelFromRow(row)
		return err
	})
	return out, err
}

// BoardSettings are per-board display choices (docs/DESIGN.md §3.2, §3.4).
// Everything that adds detail to collapsed cards is off by default.
type BoardSettings struct {
	DefaultCardState string `json:"defaultCardState"` // "collapsed" (default) | "expanded"
	ShowLabels       bool   `json:"showLabels"`       // label dots on collapsed cards
	ShowDueDate      bool   `json:"showDueDate"`      // due date on collapsed cards
	ShowChecklist    bool   `json:"showChecklist"`    // checklist progress on collapsed cards
	HideColumnCount  bool   `json:"hideColumnCount"`  // card count on collapsed columns
}

func parseSettings(raw string) BoardSettings {
	var st BoardSettings
	_ = json.Unmarshal([]byte(raw), &st) // bad or empty JSON means defaults
	if st.DefaultCardState != "expanded" {
		st.DefaultCardState = "collapsed"
	}
	return st
}

type BoardPatch struct {
	Name     *string        `json:"name"`
	Settings *BoardSettings `json:"settings"`
}

func (s *Boards) UpdateBoard(ctx context.Context, userID, boardID string, p BoardPatch) (Board, error) {
	var out Board
	err := s.tx(ctx, func(q *db.Queries) error {
		r, err := role(ctx, q, userID, boardID, true)
		if err != nil {
			return err
		}
		b, err := q.GetBoardForUser(ctx, db.GetBoardForUserParams{UserID: userID, ID: boardID})
		if err != nil {
			return err
		}
		name, settings := b.Board.Name, parseSettings(b.Board.SettingsJson)
		if p.Name != nil {
			if name, err = cleanName(*p.Name, "board", MaxBoardNameLen); err != nil {
				return err
			}
		}
		if p.Settings != nil {
			settings = *p.Settings
			if settings.DefaultCardState != "expanded" && settings.DefaultCardState != "collapsed" {
				return &ValidationError{"defaultCardState must be collapsed or expanded"}
			}
		}
		raw, err := json.Marshal(settings)
		if err != nil {
			return err
		}
		row, err := q.UpdateBoard(ctx, db.UpdateBoardParams{
			ID: boardID, Name: name, SettingsJson: string(raw), UpdatedAt: store.FormatTime(s.now()),
		})
		out = boardFromRow(row, string(r))
		return err
	})
	return out, err
}
