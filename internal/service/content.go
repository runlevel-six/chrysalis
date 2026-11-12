// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"database/sql"
	"encoding/json"
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

// ErrForbidden means the user can see the board but not change it (viewers).
var ErrForbidden = errors.New("forbidden")

const (
	MaxColumnNameLen = 100
	MaxCardTitleLen  = 500
	maxViewBatch     = 1000
)

type Column struct {
	ID        string  `json:"id"`
	BoardID   string  `json:"boardId"`
	Name      string  `json:"name"`
	Color     string  `json:"color"`
	ColorMode string  `json:"colorMode"`
	SoftLimit *int64  `json:"softLimit"`
	IsDone    bool    `json:"isDone"`
	Position  string  `json:"position"`
	DeletedAt *string `json:"deletedAt,omitempty"`
	Version   int64   `json:"version"`
	UpdatedAt string  `json:"updatedAt"`
}

type Card struct {
	ID          string   `json:"id"`
	BoardID     string   `json:"boardId"`
	ColumnID    string   `json:"columnId"`
	Title       string   `json:"title"`
	Position    string   `json:"position"`
	LabelIDs    []string `json:"labelIds"`
	StartAt     *string  `json:"startAt"` // "YYYY-MM-DD"
	DueAt       *string  `json:"dueAt"`   // "YYYY-MM-DD", or a UTC timestamp when DueHasTime
	DueHasTime  bool     `json:"dueHasTime"`
	CompletedAt *string  `json:"completedAt"`
	ArchivedAt  *string  `json:"archivedAt"`
	DeletedAt   *string  `json:"deletedAt,omitempty"`
	Version     int64    `json:"version"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

// BoardView is the signed-in user's own view of a board (docs/DESIGN.md
// §3.3). Cards and columns without an entry use the defaults.
type BoardView struct {
	DefaultCardState string          `json:"defaultCardState"` // "collapsed" | "expanded"
	Cards            map[string]bool `json:"cards"`            // card id -> expanded
	Columns          map[string]bool `json:"columns"`          // column id -> collapsed
}

// BoardDetail is everything the board page needs, in one response.
type BoardDetail struct {
	Board   Board     `json:"board"`
	Columns []Column  `json:"columns"`
	Cards   []Card    `json:"cards"`
	Labels  []Label   `json:"labels"`
	View    BoardView `json:"view"`
}

type Trash struct {
	Columns []Column `json:"columns"`
	Cards   []Card   `json:"cards"`
}

func nullStr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

func columnFromRow(c db.Column) Column {
	out := Column{
		ID: c.ID, BoardID: c.BoardID, Name: c.Name, Color: c.Color, ColorMode: c.ColorMode,
		IsDone: c.IsDoneColumn != 0, Position: c.Position, DeletedAt: nullStr(c.DeletedAt),
		Version: c.Version, UpdatedAt: c.UpdatedAt,
	}
	if c.SoftLimit.Valid {
		out.SoftLimit = &c.SoftLimit.Int64
	}
	return out
}

func cardFromRow(c db.Card) Card {
	return Card{
		ID: c.ID, BoardID: c.BoardID, ColumnID: c.ColumnID, Title: c.Title, Position: c.Position,
		LabelIDs: []string{}, StartAt: nullStr(c.StartAt), DueAt: nullStr(c.DueAt), DueHasTime: c.DueHasTime != 0,
		CompletedAt: nullStr(c.CompletedAt), ArchivedAt: nullStr(c.ArchivedAt), DeletedAt: nullStr(c.DeletedAt),
		Version: c.Version, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// withLabels fills in the card's live label IDs.
func withLabels(ctx context.Context, q *db.Queries, c Card) (Card, error) {
	ids, err := q.ListLabelIDsForCard(ctx, c.ID)
	if err != nil {
		return c, err
	}
	if ids != nil {
		c.LabelIDs = ids
	}
	return c, nil
}

// Nullable distinguishes a JSON field that's absent (leave it alone) from one
// that's explicitly null (clear it). encoding/json only calls UnmarshalJSON
// for fields present in the input, so Set records presence.
type Nullable[T any] struct {
	Set   bool
	Value *T
}

func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

const dateOnly = "2006-01-02"

// cleanDate validates a date-only value ("YYYY-MM-DD").
func cleanDate(v *string, what string) (sql.NullString, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	if _, err := time.Parse(dateOnly, *v); err != nil {
		return sql.NullString{}, &ValidationError{fmt.Sprintf("The %s date must look like 2026-05-31.", what)}
	}
	return sql.NullString{String: *v, Valid: true}, nil
}

// cleanDue validates a due value: a date, or with a time, any RFC 3339
// timestamp (stored normalized to UTC).
func cleanDue(v *string, hasTime bool) (sql.NullString, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	if !hasTime {
		return cleanDate(v, "due")
	}
	t, err := time.Parse(time.RFC3339, *v)
	if err != nil {
		return sql.NullString{}, &ValidationError{"The due time must be a full timestamp, like 2026-05-31T17:00:00-05:00."}
	}
	return sql.NullString{String: store.FormatTime(t), Valid: true}, nil
}

// tx runs fn in one write transaction. BEGIN IMMEDIATE (set in the store's
// DSN) means fn's reads and writes can't interleave with another writer.
func (s *Boards) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	t, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = t.Rollback() }()
	if err := fn(s.st.WithTx(t)); err != nil {
		return err
	}
	return t.Commit()
}

// role returns the user's role on a board, or ErrNotFound. With write set,
// viewers get ErrForbidden.
func role(ctx context.Context, q *db.Queries, userID, boardID string, write bool) (Role, error) {
	r, err := q.GetBoardRole(ctx, db.GetBoardRoleParams{UserID: userID, BoardID: boardID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if write && Role(r) == RoleViewer {
		return "", ErrForbidden
	}
	return Role(r), nil
}

func cleanName(name, what string, max int) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", &ValidationError{fmt.Sprintf("Give the %s a name.", what)}
	case utf8.RuneCountInString(name) > max:
		return "", &ValidationError{fmt.Sprintf("That %s name is too long (up to %d characters).", what, max)}
	}
	return name, nil
}

func cleanTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	switch {
	case title == "":
		return "", &ValidationError{"Give the card a title."}
	case utf8.RuneCountInString(title) > MaxCardTitleLen:
		return "", &ValidationError{fmt.Sprintf("Card titles can be up to %d characters.", MaxCardTitleLen)}
	}
	return title, nil
}

// positionOrAppend returns the client's position if it sent one (validated),
// otherwise a key after last.
func positionOrAppend(pos *string, last func() (string, error)) (string, error) {
	if pos != nil {
		if order.Valid(*pos) != nil {
			return "", &ValidationError{"invalid position"}
		}
		return *pos, nil
	}
	l, err := last()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	return order.KeyBetween(l, "")
}

// Detail returns a board with its live columns and cards and the user's view.
func (s *Boards) Detail(ctx context.Context, userID, boardID string) (BoardDetail, error) {
	var out BoardDetail
	b, err := s.Get(ctx, userID, boardID)
	if err != nil {
		return out, err
	}
	out.Board = b

	cols, err := s.st.ListColumns(ctx, boardID)
	if err != nil {
		return out, err
	}
	out.Columns = make([]Column, 0, len(cols))
	for _, c := range cols {
		out.Columns = append(out.Columns, columnFromRow(c))
	}
	cards, err := s.st.ListCards(ctx, boardID)
	if err != nil {
		return out, err
	}
	labelsByCard := map[string][]string{}
	links, err := s.st.ListCardLabelsForBoard(ctx, boardID)
	if err != nil {
		return out, err
	}
	for _, l := range links {
		labelsByCard[l.CardID] = append(labelsByCard[l.CardID], l.LabelID)
	}
	out.Cards = make([]Card, 0, len(cards))
	for _, c := range cards {
		card := cardFromRow(c)
		if ids := labelsByCard[c.ID]; ids != nil {
			card.LabelIDs = ids
		}
		out.Cards = append(out.Cards, card)
	}
	labels, err := s.st.ListLabels(ctx, boardID)
	if err != nil {
		return out, err
	}
	out.Labels = make([]Label, 0, len(labels))
	for _, l := range labels {
		out.Labels = append(out.Labels, labelFromRow(l))
	}

	out.View = BoardView{DefaultCardState: b.Settings.DefaultCardState, Cards: map[string]bool{}, Columns: map[string]bool{}}
	cv, err := s.st.ListCardViews(ctx, db.ListCardViewsParams{UserID: userID, BoardID: boardID})
	if err != nil {
		return out, err
	}
	for _, v := range cv {
		out.View.Cards[v.CardID] = v.Expanded != 0
	}
	colv, err := s.st.ListColumnViews(ctx, db.ListColumnViewsParams{UserID: userID, BoardID: boardID})
	if err != nil {
		return out, err
	}
	for _, v := range colv {
		out.View.Columns[v.ColumnID] = v.Collapsed != 0
	}
	return out, nil
}

// CreateColumn adds a column at position (or the end).
func (s *Boards) CreateColumn(ctx context.Context, userID, boardID, name string, pos *string) (Column, error) {
	name, err := cleanName(name, "column", MaxColumnNameLen)
	if err != nil {
		return Column{}, err
	}
	var out Column
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := role(ctx, q, userID, boardID, true); err != nil {
			return err
		}
		p, err := positionOrAppend(pos, func() (string, error) { return q.LastColumnPosition(ctx, boardID) })
		if err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		row, err := q.CreateColumn(ctx, db.CreateColumnParams{
			ID: ulid.Make().String(), BoardID: boardID, Name: name, Position: p, CreatedAt: now, UpdatedAt: now,
		})
		out = columnFromRow(row)
		return err
	})
	return out, err
}

// ColumnPatch holds the fields to change; nil means leave alone. SoftLimit 0
// clears the limit.
type ColumnPatch struct {
	Name      *string `json:"name"`
	Color     *string `json:"color"`
	ColorMode *string `json:"colorMode"`
	SoftLimit *int64  `json:"softLimit"`
	IsDone    *bool   `json:"isDone"`
	Position  *string `json:"position"`
}

// UpdateColumn applies p. Concurrent edits are last-write-wins (§5.5).
func (s *Boards) UpdateColumn(ctx context.Context, userID, columnID string, p ColumnPatch) (Column, error) {
	var out Column
	err := s.tx(ctx, func(q *db.Queries) error {
		c, err := q.GetColumn(ctx, columnID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && c.DeletedAt.Valid) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := role(ctx, q, userID, c.BoardID, true); err != nil {
			return err
		}
		params := db.UpdateColumnParams{
			ID: c.ID, Name: c.Name, Color: c.Color, ColorMode: c.ColorMode, SoftLimit: c.SoftLimit,
			IsDoneColumn: c.IsDoneColumn, Position: c.Position, UpdatedAt: store.FormatTime(s.now()),
		}
		if p.Name != nil {
			if params.Name, err = cleanName(*p.Name, "column", MaxColumnNameLen); err != nil {
				return err
			}
		}
		if p.Color != nil {
			params.Color = strings.TrimSpace(*p.Color)
		}
		if p.ColorMode != nil {
			if *p.ColorMode != "header" && *p.ColorMode != "full" {
				return &ValidationError{"colorMode must be header or full"}
			}
			params.ColorMode = *p.ColorMode
		}
		if p.SoftLimit != nil {
			switch {
			case *p.SoftLimit < 0:
				return &ValidationError{"softLimit can't be negative"}
			case *p.SoftLimit == 0:
				params.SoftLimit = sql.NullInt64{}
			default:
				params.SoftLimit = sql.NullInt64{Int64: *p.SoftLimit, Valid: true}
			}
		}
		if p.IsDone != nil {
			params.IsDoneColumn = 0
			if *p.IsDone {
				params.IsDoneColumn = 1
			}
		}
		if p.Position != nil {
			if order.Valid(*p.Position) != nil {
				return &ValidationError{"invalid position"}
			}
			params.Position = *p.Position
		}
		row, err := q.UpdateColumn(ctx, params)
		out = columnFromRow(row)
		return err
	})
	return out, err
}

// SetColumnTrashed moves a column (and with it, its cards) to the trash or
// back out.
func (s *Boards) SetColumnTrashed(ctx context.Context, userID, columnID string, trashed bool) (Column, error) {
	var out Column
	err := s.tx(ctx, func(q *db.Queries) error {
		c, err := q.GetColumn(ctx, columnID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := role(ctx, q, userID, c.BoardID, true); err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		del := sql.NullString{}
		if trashed {
			del = sql.NullString{String: now, Valid: true}
		}
		row, err := q.SetColumnDeleted(ctx, db.SetColumnDeletedParams{ID: c.ID, DeletedAt: del, UpdatedAt: now})
		out = columnFromRow(row)
		return err
	})
	return out, err
}

// CreateCard adds a card to a column, at position or the bottom (§3.6: new
// cards go to the bottom).
func (s *Boards) CreateCard(ctx context.Context, userID, columnID, title string, pos *string) (Card, error) {
	title, err := cleanTitle(title)
	if err != nil {
		return Card{}, err
	}
	var out Card
	err = s.tx(ctx, func(q *db.Queries) error {
		col, err := q.GetColumn(ctx, columnID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && col.DeletedAt.Valid) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := role(ctx, q, userID, col.BoardID, true); err != nil {
			return err
		}
		p, err := positionOrAppend(pos, func() (string, error) { return q.LastCardPosition(ctx, columnID) })
		if err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		row, err := q.CreateCard(ctx, db.CreateCardParams{
			ID: ulid.Make().String(), BoardID: col.BoardID, ColumnID: col.ID, Title: title,
			Position: p, CreatedAt: now, UpdatedAt: now,
		})
		out = cardFromRow(row)
		return err
	})
	return out, err
}

// CardPatch holds the fields to change; nil means leave alone. Changing
// ColumnID moves the card, possibly to another board the user can edit.
type CardPatch struct {
	Title      *string          `json:"title"`
	ColumnID   *string          `json:"columnId"`
	Position   *string          `json:"position"`
	Completed  *bool            `json:"completed"`
	Archived   *bool            `json:"archived"`
	LabelIDs   *[]string        `json:"labelIds"` // the full set; replaces the card's labels
	StartAt    Nullable[string] `json:"startAt"`  // null clears
	DueAt      Nullable[string] `json:"dueAt"`    // null clears
	DueHasTime *bool            `json:"dueHasTime"`
}

func (s *Boards) UpdateCard(ctx context.Context, userID, cardID string, p CardPatch) (Card, error) {
	var out Card
	err := s.tx(ctx, func(q *db.Queries) error {
		c, err := q.GetCard(ctx, cardID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && c.DeletedAt.Valid) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := role(ctx, q, userID, c.BoardID, true); err != nil {
			return err
		}
		// Start from the current row so fields the patch doesn't mention keep
		// their values.
		params := db.UpdateCardParams{
			ID: c.ID, BoardID: c.BoardID, ColumnID: c.ColumnID, Title: c.Title, Position: c.Position,
			CompletedAt: c.CompletedAt, StartAt: c.StartAt, DueAt: c.DueAt, DueHasTime: c.DueHasTime,
			ArchivedAt: c.ArchivedAt, UpdatedAt: store.FormatTime(s.now()),
		}
		if p.Title != nil {
			if params.Title, err = cleanTitle(*p.Title); err != nil {
				return err
			}
		}
		if p.ColumnID != nil && *p.ColumnID != c.ColumnID {
			target, err := q.GetColumn(ctx, *p.ColumnID)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && target.DeletedAt.Valid) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			// Moving to another board needs edit rights there too.
			if _, err := role(ctx, q, userID, target.BoardID, true); err != nil {
				return err
			}
			params.ColumnID, params.BoardID = target.ID, target.BoardID
			if p.Position == nil {
				last, err := q.LastCardPosition(ctx, target.ID)
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
		if p.Completed != nil {
			switch {
			case *p.Completed && !c.CompletedAt.Valid:
				params.CompletedAt = sql.NullString{String: params.UpdatedAt, Valid: true}
			case !*p.Completed:
				params.CompletedAt = sql.NullString{}
			}
		}
		if p.Archived != nil {
			switch {
			case *p.Archived && !c.ArchivedAt.Valid:
				params.ArchivedAt = sql.NullString{String: params.UpdatedAt, Valid: true}
			case !*p.Archived:
				params.ArchivedAt = sql.NullString{}
			}
		}
		if p.StartAt.Set {
			if params.StartAt, err = cleanDate(p.StartAt.Value, "start"); err != nil {
				return err
			}
		}
		if p.DueHasTime != nil {
			params.DueHasTime = 0
			if *p.DueHasTime {
				params.DueHasTime = 1
			}
		}
		if p.DueAt.Set {
			if params.DueAt, err = cleanDue(p.DueAt.Value, params.DueHasTime != 0); err != nil {
				return err
			}
		} else if p.DueHasTime != nil && params.DueAt.Valid {
			return &ValidationError{"Send dueAt along with dueHasTime."}
		}
		if !params.DueAt.Valid {
			params.DueHasTime = 0
		}
		if params.StartAt.Valid && params.DueAt.Valid && params.StartAt.String > params.DueAt.String[:len(dateOnly)] {
			return &ValidationError{"The start date is after the due date."}
		}

		row, err := q.UpdateCard(ctx, params)
		if err != nil {
			return err
		}

		movedBoards := row.BoardID != c.BoardID
		if movedBoards && p.LabelIDs == nil {
			// Labels belong to a board; they don't travel with the card.
			if err := q.ClearCardLabels(ctx, c.ID); err != nil {
				return err
			}
		}
		if p.LabelIDs != nil {
			if err := setCardLabels(ctx, q, c.ID, row.BoardID, *p.LabelIDs); err != nil {
				return err
			}
		}
		out, err = withLabels(ctx, q, cardFromRow(row))
		return err
	})
	return out, err
}

// setCardLabels replaces a card's labels. Every ID must be a live label on
// the card's board.
func setCardLabels(ctx context.Context, q *db.Queries, cardID, boardID string, ids []string) error {
	if len(ids) > len(LabelColors)*10 {
		return &ValidationError{"too many labels"}
	}
	for _, id := range ids {
		l, err := q.GetLabel(ctx, id)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && (l.BoardID != boardID || l.DeletedAt.Valid)) {
			return &ValidationError{"That label isn't on this board."}
		}
		if err != nil {
			return err
		}
	}
	if err := q.ClearCardLabels(ctx, cardID); err != nil {
		return err
	}
	for _, id := range ids {
		if err := q.AddCardLabel(ctx, db.AddCardLabelParams{CardID: cardID, LabelID: id}); err != nil {
			return err
		}
	}
	return nil
}

// Archived lists a board's archived cards, most recent first.
func (s *Boards) Archived(ctx context.Context, userID, boardID string) ([]Card, error) {
	if _, err := role(ctx, s.st.Queries, userID, boardID, false); err != nil {
		return nil, err
	}
	rows, err := s.st.ListArchivedCards(ctx, boardID)
	if err != nil {
		return nil, err
	}
	out := make([]Card, 0, len(rows))
	for _, r := range rows {
		c, err := withLabels(ctx, s.st.Queries, cardFromRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// SetCardTrashed moves a card to the trash or back out. Restoring a card
// whose column is itself trashed restores it into that column, which stays
// hidden until the column is restored too.
func (s *Boards) SetCardTrashed(ctx context.Context, userID, cardID string, trashed bool) (Card, error) {
	var out Card
	err := s.tx(ctx, func(q *db.Queries) error {
		c, err := q.GetCard(ctx, cardID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := role(ctx, q, userID, c.BoardID, true); err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		del := sql.NullString{}
		if trashed {
			del = sql.NullString{String: now, Valid: true}
		}
		row, err := q.SetCardDeleted(ctx, db.SetCardDeletedParams{ID: c.ID, DeletedAt: del, UpdatedAt: now})
		if err != nil {
			return err
		}
		out, err = withLabels(ctx, q, cardFromRow(row))
		return err
	})
	return out, err
}

// Trash lists a board's trashed columns and individually trashed cards.
func (s *Boards) Trash(ctx context.Context, userID, boardID string) (Trash, error) {
	out := Trash{Columns: []Column{}, Cards: []Card{}}
	if _, err := role(ctx, s.st.Queries, userID, boardID, false); err != nil {
		return out, err
	}
	cols, err := s.st.ListTrashedColumns(ctx, boardID)
	if err != nil {
		return out, err
	}
	for _, c := range cols {
		out.Columns = append(out.Columns, columnFromRow(c))
	}
	cards, err := s.st.ListTrashedCards(ctx, boardID)
	if err != nil {
		return out, err
	}
	for _, c := range cards {
		out.Cards = append(out.Cards, cardFromRow(c))
	}
	return out, nil
}

// SetView saves the user's expanded/collapsed state for cards and columns on
// one board. Viewers may do this too: view state isn't content. IDs that
// aren't on this board are ignored.
func (s *Boards) SetView(ctx context.Context, userID, boardID string, cards, columns map[string]bool) error {
	if len(cards)+len(columns) > maxViewBatch {
		return &ValidationError{"too many view changes in one request"}
	}
	b2i := func(b bool) int64 {
		if b {
			return 1
		}
		return 0
	}
	return s.tx(ctx, func(q *db.Queries) error {
		if _, err := role(ctx, q, userID, boardID, false); err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		for id, expanded := range cards {
			c, err := q.GetCard(ctx, id)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && c.BoardID != boardID) {
				continue
			}
			if err != nil {
				return err
			}
			if err := q.UpsertCardView(ctx, db.UpsertCardViewParams{UserID: userID, CardID: id, Expanded: b2i(expanded), UpdatedAt: now}); err != nil {
				return err
			}
		}
		for id, collapsed := range columns {
			c, err := q.GetColumn(ctx, id)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && c.BoardID != boardID) {
				continue
			}
			if err != nil {
				return err
			}
			if err := q.UpsertColumnView(ctx, db.UpsertColumnViewParams{UserID: userID, ColumnID: id, Collapsed: b2i(collapsed), UpdatedAt: now}); err != nil {
				return err
			}
		}
		return nil
	})
}

// PurgeTrash permanently deletes cards and columns trashed before cutoff.
func PurgeTrash(ctx context.Context, st *store.Store, cutoff time.Time) (int64, error) {
	c := sql.NullString{String: store.FormatTime(cutoff), Valid: true}
	n1, err := st.PurgeTrashedCards(ctx, c)
	if err != nil {
		return 0, err
	}
	n2, err := st.PurgeTrashedColumns(ctx, c)
	if err != nil {
		return 0, err
	}
	n3, err := st.PurgeTrashedLabels(ctx, c)
	return n1 + n2 + n3, err
}
