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
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/order"
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
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Icon       string     `json:"icon"`
	Background Background `json:"background"`
	Position   string     `json:"position"`
	Role       Role       `json:"role"`
	Version    int64      `json:"version"`
	CreatedAt  string     `json:"createdAt"`
	UpdatedAt  string     `json:"updatedAt"`
}

type Background struct {
	Type  string `json:"type"`  // color | gradient | image
	Value string `json:"value"` // "" = theme default
}

func boardFromRow(b db.Board, role string) Board {
	return Board{
		ID: b.ID, Name: b.Name, Icon: b.Icon,
		Background: Background{Type: b.BackgroundType, Value: b.BackgroundValue},
		Position:   b.Position, Role: Role(role), Version: b.Version,
		CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
	}
}

type Boards struct {
	st  *store.Store
	now func() time.Time
}

func NewBoards(st *store.Store, now func() time.Time) *Boards {
	return &Boards{st: st, now: now}
}

// Create makes a new board owned by userID, placed after their other boards.
func (s *Boards) Create(ctx context.Context, userID, name string) (Board, error) {
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
	defer tx.Rollback() // no-op after Commit
	q := s.st.Queries.WithTx(tx)

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
	if err := tx.Commit(); err != nil {
		return Board{}, err
	}
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
