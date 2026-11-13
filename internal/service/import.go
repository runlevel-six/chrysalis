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
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// ImportBoard is a whole board brought in from another app's export
// (docs/DESIGN.md §8), already translated into Chrysalis's terms. Order in
// each slice is the order on the board; positions are generated here.
type ImportBoard struct {
	Name       string         `json:"name"`
	Background *Background    `json:"background,omitempty"` // nil: the theme's default
	Labels     []ImportLabel  `json:"labels"`
	Columns    []ImportColumn `json:"columns"`
}

type ImportLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"` // a LabelColors key
}

type ImportColumn struct {
	Name string `json:"name"`
	// Collapsed starts the column collapsed for the importing user (for
	// lists that were archived at the source).
	Collapsed bool         `json:"collapsed,omitempty"`
	Cards     []ImportCard `json:"cards"`
}

type ImportCard struct {
	Title       string            `json:"title"`
	Description json.RawMessage   `json:"description,omitempty"` // ProseMirror doc; nil for none
	StartAt     *string           `json:"startAt,omitempty"`     // "YYYY-MM-DD"
	DueAt       *string           `json:"dueAt,omitempty"`       // RFC 3339 timestamp
	Completed   bool              `json:"completed,omitempty"`
	Archived    bool              `json:"archived,omitempty"`
	CoverColor  string            `json:"coverColor,omitempty"` // a LabelColors key, or ""
	Labels      []int             `json:"labels,omitempty"`     // indexes into ImportBoard.Labels
	Checklists  []ImportChecklist `json:"checklists,omitempty"`
	Comments    []ImportComment   `json:"comments,omitempty"`
	Links       []ImportLink      `json:"links,omitempty"`
}

type ImportChecklist struct {
	Title string       `json:"title"`
	Items []ImportItem `json:"items"`
}

type ImportItem struct {
	Text string `json:"text"`
	Done bool   `json:"done,omitempty"`
}

type ImportComment struct {
	Body      json.RawMessage `json:"body"`      // ProseMirror doc
	CreatedAt string          `json:"createdAt"` // RFC 3339; the original date
}

type ImportLink struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// ImportCounts is how many of each thing an import created.
type ImportCounts struct {
	Columns        int `json:"columns"`
	Cards          int `json:"cards"`
	Descriptions   int `json:"descriptions"`
	Labels         int `json:"labels"`
	Checklists     int `json:"checklists"`
	ChecklistItems int `json:"checklistItems"`
	Comments       int `json:"comments"`
	Links          int `json:"links"`
}

// Upper bounds for one import, well beyond any real board. They keep a
// hostile or broken file from tying up the server in one long transaction.
const (
	MaxImportColumns    = 1000
	MaxImportCards      = 20000
	MaxImportLabels     = 1000
	MaxImportChecklists = 20000
	MaxImportItems      = 200000
	MaxImportComments   = 50000
	MaxImportLinks      = 50000
)

// Count is how many of each thing importing b creates.
func (b ImportBoard) Count() ImportCounts {
	n := ImportCounts{Columns: len(b.Columns), Labels: len(b.Labels)}
	for _, col := range b.Columns {
		n.Cards += len(col.Cards)
		for _, c := range col.Cards {
			if len(c.Description) > 0 {
				n.Descriptions++
			}
			n.Checklists += len(c.Checklists)
			for _, cl := range c.Checklists {
				n.ChecklistItems += len(cl.Items)
			}
			n.Comments += len(c.Comments)
			n.Links += len(c.Links)
		}
	}
	return n
}

// errImportTooLarge means the import is bigger than one board should be.
var errImportTooLarge = &ValidationError{"This board is larger than Chrysalis can import in one go. Try archiving or removing old cards first, then export it again."}

// Import creates a new board owned by userID from b, all in one
// transaction: if anything fails, nothing is left behind. It never merges
// into an existing board. Everything is validated with the same rules as
// everyday edits.
func (s *Boards) Import(ctx context.Context, userID string, b ImportBoard) (Board, ImportCounts, error) {
	n := b.Count()
	if n.Columns > MaxImportColumns || n.Cards > MaxImportCards || n.Labels > MaxImportLabels ||
		n.Checklists > MaxImportChecklists || n.ChecklistItems > MaxImportItems ||
		n.Comments > MaxImportComments || n.Links > MaxImportLinks {
		return Board{}, ImportCounts{}, errImportTooLarge
	}
	name, err := cleanName(b.Name, "board", MaxBoardNameLen)
	if err != nil {
		return Board{}, ImportCounts{}, err
	}
	if b.Background != nil && b.Background.Type == "image" {
		return Board{}, ImportCounts{}, &ValidationError{"An imported board can't start with a background image."}
	}

	var out Board
	err = s.tx(ctx, func(q *db.Queries) error {
		last, err := q.LastBoardPosition(ctx, userID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		pos, err := order.KeyBetween(last, "")
		if err != nil {
			return err
		}
		nowT := s.now()
		now := store.FormatTime(nowT)
		row, err := q.CreateBoard(ctx, db.CreateBoardParams{
			ID: ulid.Make().String(), OwnerID: userID, Name: name, Position: pos, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := q.AddBoardMember(ctx, db.AddBoardMemberParams{BoardID: row.ID, UserID: userID, Role: string(RoleOwner)}); err != nil {
			return err
		}
		if bg := b.Background; bg != nil {
			if err := checkBackground(ctx, q, row.ID, *bg); err != nil {
				return err
			}
			if err := q.SetBoardBackground(ctx, db.SetBoardBackgroundParams{ID: row.ID, BackgroundType: bg.Type, BackgroundValue: bg.Value}); err != nil {
				return err
			}
			row.BackgroundType, row.BackgroundValue = bg.Type, bg.Value
		}
		im := importer{q: q, ctx: ctx, userID: userID, boardID: row.ID, now: now, nowT: nowT}
		if err := im.labels(b.Labels); err != nil {
			return err
		}
		colPos := ""
		for _, col := range b.Columns {
			if colPos, err = order.KeyBetween(colPos, ""); err != nil {
				return err
			}
			if err := im.column(col, colPos); err != nil {
				return err
			}
		}
		out = boardFromRow(row, string(RoleOwner))
		return nil
	})
	if err != nil {
		return Board{}, ImportCounts{}, err
	}
	return out, n, nil
}

// importer writes one import inside its transaction.
type importer struct {
	q        *db.Queries
	ctx      context.Context
	userID   string
	boardID  string
	now      string
	nowT     time.Time
	labelIDs []string
}

func (im *importer) labels(labels []ImportLabel) error {
	pos := ""
	for _, l := range labels {
		name, color, err := cleanLabel(l.Name, l.Color)
		if err != nil {
			return err
		}
		if pos, err = order.KeyBetween(pos, ""); err != nil {
			return err
		}
		id := ulid.Make().String()
		if err := im.q.ImportLabel(im.ctx, db.ImportLabelParams{
			ID: id, BoardID: im.boardID, Name: name, Color: color, Position: pos, CreatedAt: im.now, UpdatedAt: im.now,
		}); err != nil {
			return err
		}
		im.labelIDs = append(im.labelIDs, id)
	}
	return nil
}

func (im *importer) column(col ImportColumn, pos string) error {
	name, err := cleanName(col.Name, "column", MaxColumnNameLen)
	if err != nil {
		return err
	}
	id := ulid.Make().String()
	if err := im.q.ImportColumn(im.ctx, db.ImportColumnParams{
		ID: id, BoardID: im.boardID, Name: name, Position: pos, CreatedAt: im.now, UpdatedAt: im.now,
	}); err != nil {
		return err
	}
	if col.Collapsed {
		if err := im.q.UpsertColumnView(im.ctx, db.UpsertColumnViewParams{UserID: im.userID, ColumnID: id, Collapsed: 1, UpdatedAt: im.now}); err != nil {
			return err
		}
	}
	cardPos := ""
	for _, c := range col.Cards {
		if cardPos, err = order.KeyBetween(cardPos, ""); err != nil {
			return err
		}
		if err := im.card(id, c, cardPos); err != nil {
			return fmt.Errorf("card %q: %w", c.Title, err)
		}
	}
	return nil
}

func (im *importer) card(columnID string, c ImportCard, pos string) error {
	title, err := cleanTitle(c.Title)
	if err != nil {
		return err
	}
	p := db.ImportCardParams{
		ID: ulid.Make().String(), BoardID: im.boardID, ColumnID: columnID, Title: title, Position: pos,
		CoverType: "none", CreatedAt: im.now, UpdatedAt: im.now,
	}
	if p.StartAt, err = cleanDate(c.StartAt, "start"); err != nil {
		return err
	}
	if p.DueAt, err = cleanDue(c.DueAt, true); err != nil {
		return err
	}
	if p.DueAt.Valid {
		p.DueHasTime = 1
		if p.StartAt.Valid && p.StartAt.String > p.DueAt.String[:len(dateOnly)] {
			return &ValidationError{"The start date is after the due date."}
		}
	}
	if c.Completed {
		p.CompletedAt = sql.NullString{String: im.now, Valid: true}
	}
	if c.Archived {
		p.ArchivedAt = sql.NullString{String: im.now, Valid: true}
	}
	if c.CoverColor != "" {
		if !slices.Contains(LabelColors, c.CoverColor) {
			return &ValidationError{"Pick one of the cover colors."}
		}
		p.CoverType, p.CoverValue = "color", c.CoverColor
	}
	if err := im.q.ImportCard(im.ctx, p); err != nil {
		return err
	}

	seen := map[int]bool{}
	for _, li := range c.Labels {
		if li < 0 || li >= len(im.labelIDs) {
			return &ValidationError{"That label isn't on this board."}
		}
		if seen[li] {
			continue
		}
		seen[li] = true
		if err := im.q.AddCardLabel(im.ctx, db.AddCardLabelParams{CardID: p.ID, LabelID: im.labelIDs[li]}); err != nil {
			return err
		}
	}

	if len(c.Description) > 0 {
		doc, err := cleanDoc(c.Description)
		if err != nil {
			return err
		}
		if doc != "" {
			if _, err := im.q.UpsertCardDescription(im.ctx, db.UpsertCardDescriptionParams{
				CardID: p.ID, DocJson: doc, Version: 1, UpdatedAt: im.now, UpdatedBy: im.userID,
			}); err != nil {
				return err
			}
		}
	}

	clPos := ""
	for _, cl := range c.Checklists {
		title, err := cleanText(cl.Title, "checklist title", MaxChecklistTitleLen, false)
		if err != nil {
			return err
		}
		if clPos, err = order.KeyBetween(clPos, ""); err != nil {
			return err
		}
		clID := ulid.Make().String()
		if err := im.q.ImportChecklist(im.ctx, db.ImportChecklistParams{
			ID: clID, CardID: p.ID, Title: title, Position: clPos, CreatedAt: im.now, UpdatedAt: im.now,
		}); err != nil {
			return err
		}
		itemPos := ""
		for _, it := range cl.Items {
			text, err := cleanText(it.Text, "item", MaxItemTextLen, false)
			if err != nil {
				return err
			}
			if itemPos, err = order.KeyBetween(itemPos, ""); err != nil {
				return err
			}
			done := sql.NullString{}
			if it.Done {
				done = sql.NullString{String: im.now, Valid: true}
			}
			if err := im.q.ImportChecklistItem(im.ctx, db.ImportChecklistItemParams{
				ID: ulid.Make().String(), ChecklistID: clID, Text: text, DoneAt: done, Position: itemPos,
				CreatedAt: im.now, UpdatedAt: im.now,
			}); err != nil {
				return err
			}
		}
	}

	for _, cm := range c.Comments {
		body, err := cleanDoc(cm.Body)
		if err != nil {
			return err
		}
		if body == "" {
			return &ValidationError{"A comment can't be empty."}
		}
		at := im.now
		if t, err := time.Parse(time.RFC3339, cm.CreatedAt); err == nil && !t.After(im.nowT) {
			at = store.FormatTime(t)
		}
		if err := im.q.CreateComment(im.ctx, db.CreateCommentParams{
			ID: ulid.Make().String(), CardID: p.ID, AuthorID: im.userID, BodyJson: body, CreatedAt: at, UpdatedAt: at,
		}); err != nil {
			return err
		}
	}

	for _, l := range c.Links {
		u, err := CleanLinkURL(l.URL)
		if err != nil {
			return err
		}
		name := cleanFilename(l.Title)
		if strings.TrimSpace(l.Title) == "" {
			name = u.Host
		}
		if _, err := im.q.CreateAttachment(im.ctx, db.CreateAttachmentParams{
			ID: ulid.Make().String(), BoardID: im.boardID, CardID: sql.NullString{String: p.ID, Valid: true},
			UploaderID: im.userID, Kind: "link", Filename: name, Url: u.String(), CreatedAt: im.now,
		}); err != nil {
			return err
		}
	}
	return nil
}
