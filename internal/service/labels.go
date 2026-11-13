// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/sse"
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
	if err == nil {
		s.emit(ctx, sse.Event{BoardID: out.BoardID, Kind: "label", EntityID: out.ID})
	}
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
	if err == nil {
		s.emit(ctx, sse.Event{BoardID: out.BoardID, Kind: "label", EntityID: out.ID})
	}
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
	if err == nil {
		s.emit(ctx, sse.Event{BoardID: out.BoardID, Kind: "label", EntityID: out.ID})
	}
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
	Name       *string        `json:"name"`
	Icon       *string        `json:"icon"`
	Position   *string        `json:"position"` // among the user's boards
	Archived   *bool          `json:"archived"` // owner only
	Settings   *BoardSettings `json:"settings"`
	Background *Background    `json:"background"`
}

// Gradients are the background presets (§3.5); the client maps each id to
// its colors, so presets can be refined without touching stored boards.
var Gradients = []string{"meadow", "dawn", "sky", "milkweed", "dusk"}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// checkBackground validates a board background: "" (the theme's surface),
// a #rrggbb color, a gradient preset, or one of this board's own images.
func checkBackground(ctx context.Context, q *db.Queries, boardID string, bg Background) error {
	switch bg.Type {
	case "color":
		if bg.Value != "" && !hexColor.MatchString(bg.Value) {
			return &ValidationError{"Background colors look like #a1b2c3."}
		}
	case "gradient":
		if !slices.Contains(Gradients, bg.Value) {
			return &ValidationError{"Pick one of the background gradients."}
		}
	case "image":
		a, err := q.GetAttachment(ctx, bg.Value)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && (a.BoardID != boardID || a.Kind != "image" || a.DeletedAt.Valid)) {
			return &ValidationError{"That image isn't one of this board's."}
		}
		return err
	default:
		return &ValidationError{"background type must be color, gradient or image"}
	}
	return nil
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
		icon, position, archivedAt := b.Board.Icon, b.Board.Position, b.Board.ArchivedAt
		if p.Icon != nil {
			if icon, err = cleanIcon(*p.Icon); err != nil {
				return err
			}
		}
		if p.Position != nil {
			if err := order.Valid(*p.Position); err != nil {
				return &ValidationError{"invalid position"}
			}
			position = *p.Position
		}
		if p.Archived != nil && *p.Archived != archivedAt.Valid {
			if r != RoleOwner {
				return ErrForbidden
			}
			archivedAt = sql.NullString{}
			if *p.Archived {
				archivedAt = sql.NullString{String: store.FormatTime(s.now()), Valid: true}
			}
		}
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
		bgType, bgValue := b.Board.BackgroundType, b.Board.BackgroundValue
		if p.Background != nil {
			if err := checkBackground(ctx, q, boardID, *p.Background); err != nil {
				return err
			}
			// A replaced background image goes to the trash, so the usual purge
			// removes its file later instead of it being kept forever.
			if bgType == "image" && (p.Background.Type != "image" || p.Background.Value != bgValue) {
				if _, err := q.SetAttachmentDeleted(ctx, db.SetAttachmentDeletedParams{
					ID: bgValue, DeletedAt: sql.NullString{String: store.FormatTime(s.now()), Valid: true},
				}); err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}
			}
			bgType, bgValue = p.Background.Type, p.Background.Value
		}
		row, err := q.UpdateBoard(ctx, db.UpdateBoardParams{
			ID: boardID, Name: name, Icon: icon, Position: position, SettingsJson: string(raw),
			BackgroundType: bgType, BackgroundValue: bgValue, ArchivedAt: archivedAt,
			UpdatedAt: store.FormatTime(s.now()),
		})
		out = boardFromRow(row, string(r))
		return err
	})
	if err == nil {
		s.emit(ctx, sse.Event{BoardID: out.ID, Kind: "board", EntityID: out.ID})
		if p.Name != nil || p.Icon != nil || p.Position != nil || p.Archived != nil {
			s.emit(ctx, sse.Event{UserID: userID, Kind: "boards", EntityID: out.ID})
		}
	}
	return out, err
}
