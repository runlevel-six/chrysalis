// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package service holds the app's business rules. Handlers in internal/api
// decode requests, call a service, and encode the result; services never see
// HTTP.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/sse"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// ErrNotFound means the thing doesn't exist or the user can't see it. The
// two cases look the same on purpose, so IDs of other people's boards can't
// be probed.
var ErrNotFound = errors.New("not found")

// ValidationError is a problem with user input; its message is safe to show.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

const MaxBoardNameLen = 200

type Role string

const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

// Board is a board as the current user sees it.
type Board struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Icon       string        `json:"icon"`
	Background Background    `json:"background"`
	Settings   BoardSettings `json:"settings"`
	Position   string        `json:"position"`
	Archived   bool          `json:"archived"`
	DeletedAt  string        `json:"deletedAt,omitempty"`
	Role       Role          `json:"role"`
	Version    int64         `json:"version"`
	CreatedAt  string        `json:"createdAt"`
	UpdatedAt  string        `json:"updatedAt"`
}

type Background struct {
	Type  string `json:"type"`  // color | gradient | image
	Value string `json:"value"` // "" = theme default
}

func boardFromRow(b db.Board, role string) Board {
	return Board{
		ID: b.ID, Name: b.Name, Icon: b.Icon,
		Background: Background{Type: b.BackgroundType, Value: b.BackgroundValue},
		Settings:   parseSettings(b.SettingsJson),
		Position:   b.Position, Role: Role(role), Version: b.Version,
		Archived: b.ArchivedAt.Valid, DeletedAt: b.DeletedAt.String,
		CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
	}
}

type Boards struct {
	st     *store.Store
	now    func() time.Time
	notify Notifier
}

func NewBoards(st *store.Store, now func() time.Time) *Boards {
	return &Boards{st: st, now: now}
}

// Create makes a new board owned by userID, placed after their other boards.
// Templates are starting columns offered when a board is created (§3.12),
// never forced. Each entry is a column name and whether it's a done column.
var Templates = map[string][]struct {
	Name string
	Done bool
}{
	"blank":  nil,
	"simple": {{"To do", false}, {"Doing", false}, {"Done", true}},
	// The app's namesake: a card's life from first idea to finished.
	"metamorphosis": {{"Egg", false}, {"Caterpillar", false}, {"Chrysalis", false}, {"Imago", true}},
}

// Create makes a new board owned by userID, placed after their other
// boards, with the columns of the named template ("" means blank).
func (s *Boards) Create(ctx context.Context, userID, name string, template ...string) (Board, error) {
	tmpl := ""
	if len(template) > 0 {
		tmpl = template[0]
	}
	if tmpl == "" {
		tmpl = "blank"
	}
	cols, ok := Templates[tmpl]
	if !ok {
		return Board{}, &ValidationError{"template must be blank, simple or metamorphosis"}
	}
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return Board{}, &ValidationError{"Give the board a name."}
	case utf8.RuneCountInString(name) > MaxBoardNameLen:
		return Board{}, &ValidationError{fmt.Sprintf("Board names can be up to %d characters.", MaxBoardNameLen)}
	}

	// The store opens transactions with BEGIN IMMEDIATE, so reading the last
	// position and inserting after it can't interleave with another create.
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return Board{}, err
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	q := s.st.WithTx(tx)

	last, err := q.LastBoardPosition(ctx, userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Board{}, err
	}
	pos, err := order.KeyBetween(last, "")
	if err != nil {
		return Board{}, fmt.Errorf("board position: %w", err)
	}

	now := store.FormatTime(s.now())
	row, err := q.CreateBoard(ctx, db.CreateBoardParams{
		ID: ulid.Make().String(), OwnerID: userID, Name: name, Position: pos,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return Board{}, err
	}
	if err := q.AddBoardMember(ctx, db.AddBoardMemberParams{BoardID: row.ID, UserID: userID, Role: string(RoleOwner)}); err != nil {
		return Board{}, err
	}
	colPos := ""
	for _, c := range cols {
		if colPos, err = order.KeyBetween(colPos, ""); err != nil {
			return Board{}, err
		}
		col, err := q.CreateColumn(ctx, db.CreateColumnParams{
			ID: ulid.Make().String(), BoardID: row.ID, Name: c.Name, Position: colPos, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return Board{}, err
		}
		if c.Done {
			if _, err := q.UpdateColumn(ctx, db.UpdateColumnParams{
				ID: col.ID, Name: col.Name, Color: col.Color, ColorMode: col.ColorMode, SoftLimit: col.SoftLimit,
				IsDoneColumn: 1, Position: col.Position, UpdatedAt: now,
			}); err != nil {
				return Board{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return Board{}, err
	}
	// The person's board list changed on their other devices.
	s.emit(ctx, sse.Event{UserID: userID, Kind: "boards", EntityID: row.ID})
	return boardFromRow(row, string(RoleOwner)), nil
}

// List returns the boards userID is a member of, in their saved order.
func (s *Boards) List(ctx context.Context, userID string) ([]Board, error) {
	rows, err := s.st.ListBoardsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(rows))
	for _, r := range rows {
		out = append(out, boardFromRow(r.Board, r.Role))
	}
	return out, nil
}

// Get returns one board if userID is a member of it.
func (s *Boards) Get(ctx context.Context, userID, boardID string) (Board, error) {
	r, err := s.st.GetBoardForUser(ctx, db.GetBoardForUserParams{UserID: userID, ID: boardID})
	if errors.Is(err, sql.ErrNoRows) {
		return Board{}, ErrNotFound
	}
	if err != nil {
		return Board{}, err
	}
	return boardFromRow(r.Board, r.Role), nil
}

// ListArchived returns the user's archived boards, most recently archived first.
func (s *Boards) ListArchived(ctx context.Context, userID string) ([]Board, error) {
	rows, err := s.st.ListArchivedBoardsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(rows))
	for _, r := range rows {
		out = append(out, boardFromRow(r.Board, r.Role))
	}
	return out, nil
}

// ListTrashed returns boards the user deleted that can still be restored.
func (s *Boards) ListTrashed(ctx context.Context, userID string) ([]Board, error) {
	rows, err := s.st.ListTrashedBoardsForOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(rows))
	for _, r := range rows {
		out = append(out, boardFromRow(r.Board, r.Role))
	}
	return out, nil
}

// SetBoardTrashed moves a board to the trash, or restores it. Only its owner
// can. Everything on it stays as it was, and comes back with it.
func (s *Boards) SetBoardTrashed(ctx context.Context, userID, boardID string, trashed bool) (Board, error) {
	var out Board
	err := s.tx(ctx, func(q *db.Queries) error {
		r, err := q.GetBoardForUserIncludingTrashed(ctx, db.GetBoardForUserIncludingTrashedParams{UserID: userID, ID: boardID})
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if Role(r.Role) != RoleOwner {
			return ErrForbidden
		}
		if r.Board.DeletedAt.Valid == trashed {
			out = boardFromRow(r.Board, r.Role)
			return nil
		}
		now := store.FormatTime(s.now())
		deleted := sql.NullString{}
		if trashed {
			deleted = sql.NullString{String: now, Valid: true}
		}
		row, err := q.SetBoardDeleted(ctx, db.SetBoardDeletedParams{ID: boardID, DeletedAt: deleted, UpdatedAt: now})
		out = boardFromRow(row, r.Role)
		return err
	})
	if err == nil {
		s.emit(ctx, sse.Event{BoardID: boardID, Kind: "board", EntityID: boardID})
		s.emit(ctx, sse.Event{UserID: userID, Kind: "boards", EntityID: boardID})
	}
	return out, err
}

// MaxBoardIconLen bounds a board's icon: one emoji, which can be a sequence
// of several code points (a family, a flag, a skin tone).
const MaxBoardIconLen = 16

func cleanIcon(icon string) (string, error) {
	icon = strings.TrimSpace(icon)
	if utf8.RuneCountInString(icon) > MaxBoardIconLen || len(icon) > 64 {
		return "", &ValidationError{"Use a single emoji for the board's icon."}
	}
	for _, r := range icon {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", &ValidationError{"Use a single emoji for the board's icon."}
		}
	}
	return icon, nil
}
